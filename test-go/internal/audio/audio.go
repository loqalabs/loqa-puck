/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package audio

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/gordonklaus/portaudio"
)

// RelayAudio handles audio capture and playback for the test relay
type RelayAudio struct {
	sampleRate      float64
	framesPerBuffer int
	channels        int
	inputStream     *portaudio.Stream
	outputStream    *portaudio.Stream
	isRecording     bool
	isPlaying       bool

	// VAD settings
	energyThreshold float64
	preBufferSize   int

	// Wake word detection
	wakeWordEnabled   bool
	wakeWordThreshold float64
	wakeWordPattern   []float64
}

// NewRelayAudio creates a new relay audio interface
func NewRelayAudio() (*RelayAudio, error) {
	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %w", err)
	}

	return &RelayAudio{
		sampleRate:        16000.0, // 16kHz for speech
		framesPerBuffer:   1024,
		channels:          1, // Mono
		energyThreshold:   0.01,
		preBufferSize:     10, // ~200ms pre-buffer
		wakeWordEnabled:   true,
		wakeWordThreshold: 0.7,
		wakeWordPattern:   generateWakeWordPattern(), // Simple "hey loqa" pattern
	}, nil
}

// AudioChunk represents a chunk of audio data
type AudioChunk struct {
	Data          []float32
	SampleRate    int32
	Channels      int32
	Timestamp     int64
	IsWakeWord    bool
	IsEndOfSpeech bool
}

// StartRecording begins audio capture with voice activity detection
func (pa *RelayAudio) StartRecording(audioChan chan<- AudioChunk) error {
	if pa.isRecording {
		return fmt.Errorf("already recording")
	}

	// Create input buffer
	inputBuffer := make([]float32, pa.framesPerBuffer*pa.channels)

	// Pre-recording circular buffer
	preBuffer := make([][]float32, pa.preBufferSize)
	preBufferIndex := 0

	// Open input stream
	inputStream, err := portaudio.OpenDefaultStream(
		pa.channels, // input channels
		0,           // output channels
		pa.sampleRate,
		pa.framesPerBuffer,
		inputBuffer,
	)
	if err != nil {
		return fmt.Errorf("failed to open input stream: %w", err)
	}

	pa.inputStream = inputStream
	pa.isRecording = true

	if err := pa.inputStream.Start(); err != nil {
		return fmt.Errorf("failed to start input stream: %w", err)
	}

	log.Println("🎤 Relay: Started audio recording")

	// Recording loop
	go func() {
		defer func() {
			if err := pa.inputStream.Stop(); err != nil {
				log.Printf("⚠️ Failed to stop input stream: %v", err)
			}
			if err := pa.inputStream.Close(); err != nil {
				log.Printf("⚠️ Failed to close input stream: %v", err)
			}
			pa.isRecording = false
			log.Println("🎤 Relay: Stopped audio recording")
		}()

		voiceDetected := false
		lastVoiceTime := time.Now()
		recordingStart := time.Time{}
		var audioBuffer []float32
		var wakeWordBuffer []float32
		wakeWordDetected := false

		for pa.isRecording {
			if err := pa.inputStream.Read(); err != nil {
				log.Printf("❌ Error reading audio: %v", err)
				return
			}

			// Make a copy for pre-buffer
			bufferCopy := make([]float32, len(inputBuffer))
			copy(bufferCopy, inputBuffer)

			hasVoice := pa.calculateEnergy(inputBuffer) > pa.energyThreshold

			// Wake word detection
			if pa.wakeWordEnabled && hasVoice {
				wakeWordBuffer = append(wakeWordBuffer, inputBuffer...)

				// Keep wake word buffer at manageable size (2 seconds)
				maxWakeWordSamples := int(pa.sampleRate * 2.0)
				if len(wakeWordBuffer) > maxWakeWordSamples {
					excess := len(wakeWordBuffer) - maxWakeWordSamples
					wakeWordBuffer = wakeWordBuffer[excess:]
				}

				// Check for wake word pattern
				if !wakeWordDetected && len(wakeWordBuffer) > len(pa.wakeWordPattern)*100 {
					confidence := pa.detectWakeWord(wakeWordBuffer)
					if confidence > pa.wakeWordThreshold {
						wakeWordDetected = true
						log.Printf("🎯 Relay: Wake word detected! (confidence: %.2f)", confidence)
					}
				}
			}

			if hasVoice && (wakeWordDetected || !pa.wakeWordEnabled) {
				if !voiceDetected {
					// Voice detected after wake word - start recording
					voiceDetected = true
					recordingStart = time.Now()
					log.Println("🎤 Relay: Voice detected! Starting transmission...")

					// Include pre-buffered audio
					for i := 0; i < pa.preBufferSize; i++ {
						idx := (preBufferIndex + i) % pa.preBufferSize
						if preBuffer[idx] != nil {
							audioBuffer = append(audioBuffer, preBuffer[idx]...)
						}
					}
				}
				lastVoiceTime = time.Now()
				audioBuffer = append(audioBuffer, inputBuffer...)

			} else if voiceDetected {
				// Check for end of speech
				timeSinceLast := time.Since(lastVoiceTime)
				if timeSinceLast > 2*time.Second {
					// End of speech detected
					log.Printf("🎤 Relay: End of speech detected - sending %.1fs of audio\n",
						time.Since(recordingStart).Seconds())

					// Send the complete audio buffer
					channels := pa.channels
					if channels < 0 || channels > 255 {
						log.Printf("⚠️ Invalid channel count: %d, using 1", channels)
						channels = 1
					}
					// #nosec G115 - channels is bounds-checked above
					channelsInt32 := int32(channels)
					chunk := AudioChunk{
						Data:          audioBuffer,
						SampleRate:    int32(pa.sampleRate),
						Channels:      channelsInt32,
						Timestamp:     time.Now().UnixNano(),
						IsWakeWord:    wakeWordDetected,
						IsEndOfSpeech: true,
					}

					select {
					case audioChan <- chunk:
						// Successfully sent
					default:
						log.Println("⚠️  Audio channel full, dropping chunk")
					}

					// Reset for next utterance
					voiceDetected = false
					wakeWordDetected = false
					audioBuffer = nil
					wakeWordBuffer = nil
				} else {
					// Still within silence timeout
					audioBuffer = append(audioBuffer, inputBuffer...)
				}
			} else {
				// No voice - store in pre-buffer
				preBuffer[preBufferIndex] = bufferCopy
				preBufferIndex = (preBufferIndex + 1) % pa.preBufferSize
			}

			// Prevent busy waiting
			time.Sleep(10 * time.Millisecond)
		}
	}()

	return nil
}

// StopRecording stops audio capture
func (pa *RelayAudio) StopRecording() {
	pa.isRecording = false
}

// PlayAudio plays audio data through speakers
func (pa *RelayAudio) PlayAudio(audioData []float32) error {
	if pa.isPlaying {
		return fmt.Errorf("already playing audio")
	}

	// Create output buffer
	outputBuffer := audioData

	// Open output stream
	outputStream, err := portaudio.OpenDefaultStream(
		0,           // input channels
		pa.channels, // output channels
		pa.sampleRate,
		pa.framesPerBuffer,
		outputBuffer[:pa.framesPerBuffer],
	)
	if err != nil {
		return fmt.Errorf("failed to open output stream: %w", err)
	}

	pa.outputStream = outputStream
	pa.isPlaying = true

	log.Printf("🔊 Relay: Playing %d samples of audio\n", len(audioData))

	if err := pa.outputStream.Start(); err != nil {
		return fmt.Errorf("failed to start output stream: %w", err)
	}

	// Play audio in chunks
	go func() {
		defer func() {
			if err := pa.outputStream.Stop(); err != nil {
				log.Printf("⚠️ Failed to stop output stream: %v", err)
			}
			if err := pa.outputStream.Close(); err != nil {
				log.Printf("⚠️ Failed to close output stream: %v", err)
			}
			pa.isPlaying = false
			log.Println("🔊 Relay: Finished playing audio")
		}()

		samplesPlayed := 0
		chunkSize := pa.framesPerBuffer * pa.channels

		for samplesPlayed < len(audioData) {
			remainingSamples := len(audioData) - samplesPlayed
			currentChunkSize := min(chunkSize, remainingSamples)

			// Copy chunk to output buffer
			chunk := outputBuffer[samplesPlayed : samplesPlayed+currentChunkSize]
			copy(outputBuffer[:currentChunkSize], chunk)

			if err := pa.outputStream.Write(); err != nil {
				log.Printf("❌ Error writing audio: %v", err)
				return
			}

			samplesPlayed += currentChunkSize
		}
	}()

	return nil
}

// calculateEnergy calculates the RMS energy of an audio buffer
func (pa *RelayAudio) calculateEnergy(buffer []float32) float64 {
	if len(buffer) == 0 {
		return 0
	}

	var sum float64
	for _, sample := range buffer {
		sum += float64(sample * sample)
	}

	return math.Sqrt(sum / float64(len(buffer)))
}

// Shutdown cleans up audio resources
func (pa *RelayAudio) Shutdown() {
	pa.StopRecording()
	if pa.outputStream != nil && pa.isPlaying {
		if err := pa.outputStream.Stop(); err != nil {
			log.Printf("⚠️ Failed to stop output stream during shutdown: %v", err)
		}
		if err := pa.outputStream.Close(); err != nil {
			log.Printf("⚠️ Failed to close output stream during shutdown: %v", err)
		}
	}
	if err := portaudio.Terminate(); err != nil {
		log.Printf("⚠️ Failed to terminate PortAudio: %v", err)
	}
	log.Println("🔌 Relay: Audio system shutdown")
}

// generateWakeWordPattern creates a simple frequency pattern for "hey loqa"
func generateWakeWordPattern() []float64 {
	// Simple frequency envelope pattern approximating "hey loqa"
	// This is a very basic pattern - in production you'd use proper ML models
	return []float64{
		0.1, 0.3, 0.8, 0.6, 0.2, // "hey" - rising then falling
		0.1, 0.1, 0.1, // pause
		0.2, 0.5, 0.4, 0.7, 0.3, // "lo" - moderate energy
		0.6, 0.8, 0.5, 0.2, // "qa" - peak then drop
	}
}

// detectWakeWord performs basic pattern matching for wake word detection
func (pa *RelayAudio) detectWakeWord(audioBuffer []float32) float64 {
	if len(audioBuffer) == 0 || len(pa.wakeWordPattern) == 0 {
		return 0.0
	}

	// Calculate energy envelope of the audio
	chunkSize := len(audioBuffer) / len(pa.wakeWordPattern)
	if chunkSize < 100 { // Need minimum samples per chunk
		return 0.0
	}

	var audioEnvelope []float64
	for i := 0; i < len(pa.wakeWordPattern); i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(audioBuffer) {
			end = len(audioBuffer)
		}

		chunk := audioBuffer[start:end]
		energy := pa.calculateEnergy(chunk)
		audioEnvelope = append(audioEnvelope, energy)
	}

	// Normalize audio envelope
	maxEnergy := 0.0
	for _, energy := range audioEnvelope {
		if energy > maxEnergy {
			maxEnergy = energy
		}
	}

	if maxEnergy == 0 {
		return 0.0
	}

	for i := range audioEnvelope {
		audioEnvelope[i] /= maxEnergy
	}

	// Calculate correlation with wake word pattern
	correlation := 0.0
	for i := 0; i < len(pa.wakeWordPattern) && i < len(audioEnvelope); i++ {
		correlation += pa.wakeWordPattern[i] * audioEnvelope[i]
	}

	// Normalize correlation
	patternEnergy := 0.0
	for _, val := range pa.wakeWordPattern {
		patternEnergy += val * val
	}

	if patternEnergy == 0 {
		return 0.0
	}

	confidence := correlation / math.Sqrt(patternEnergy)
	return math.Max(0.0, math.Min(1.0, confidence))
}

// EnableWakeWord enables or disables wake word detection
func (pa *RelayAudio) EnableWakeWord(enabled bool) {
	pa.wakeWordEnabled = enabled
	if enabled {
		log.Println("🎯 Relay: Wake word detection enabled")
	} else {
		log.Println("🎯 Relay: Wake word detection disabled")
	}
}

// SetWakeWordThreshold sets the confidence threshold for wake word detection
func (pa *RelayAudio) SetWakeWordThreshold(threshold float64) {
	pa.wakeWordThreshold = math.Max(0.0, math.Min(1.0, threshold))
	log.Printf("🎯 Relay: Wake word threshold set to %.2f", pa.wakeWordThreshold)
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
