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
	"testing"
	"time"
)

func TestRelayAudio_Creation(t *testing.T) {
	// Note: This test may require PortAudio to be installed
	// In CI environments, you might want to skip this test
	if testing.Short() {
		t.Skip("Skipping PortAudio tests in short mode")
	}

	audio, err := NewRelayAudio()
	if err != nil {
		t.Skipf("Failed to create RelayAudio (PortAudio may not be available): %v", err)
	}

	if audio == nil {
		t.Fatal("NewRelayAudio returned nil without error")
	}

	// Verify default configuration
	if audio.sampleRate != 22050.0 {
		t.Errorf("Expected sample rate 22050, got %f", audio.sampleRate)
	}

	if audio.channels != 1 {
		t.Errorf("Expected 1 channel, got %d", audio.channels)
	}

	if audio.framesPerBuffer != 1024 {
		t.Errorf("Expected frames per buffer 1024, got %d", audio.framesPerBuffer)
	}

	if !audio.wakeWordEnabled {
		t.Error("Expected wake word detection to be enabled by default")
	}

	// Test shutdown
	audio.Shutdown()
}

func TestCalculateEnergy(t *testing.T) {
	audio := &RelayAudio{}

	tests := []struct {
		name     string
		input    []float32
		expected float64
		epsilon  float64
	}{
		{
			name:     "empty buffer",
			input:    []float32{},
			expected: 0.0,
			epsilon:  0.000001,
		},
		{
			name:     "zero samples",
			input:    []float32{0.0, 0.0, 0.0, 0.0},
			expected: 0.0,
			epsilon:  0.000001,
		},
		{
			name:     "single positive sample",
			input:    []float32{1.0},
			expected: 1.0,
			epsilon:  0.000001,
		},
		{
			name:     "single negative sample",
			input:    []float32{-1.0},
			expected: 1.0,
			epsilon:  0.000001,
		},
		{
			name:     "mixed samples",
			input:    []float32{0.5, -0.5, 0.3, -0.3},
			expected: 0.412, // sqrt((0.25 + 0.25 + 0.09 + 0.09) / 4) = sqrt(0.68/4) = sqrt(0.17) ≈ 0.412
			epsilon:  0.01,
		},
		{
			name:     "realistic audio samples",
			input:    []float32{0.1, -0.2, 0.15, -0.1, 0.05},
			expected: 0.13, // Approximate RMS
			epsilon:  0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := audio.calculateEnergy(tt.input)

			if abs64(result-tt.expected) > tt.epsilon {
				t.Errorf("calculateEnergy() = %f, want %f (±%f)", result, tt.expected, tt.epsilon)
			}
		})
	}
}

func TestWakeWordDetection(t *testing.T) {
	audio := &RelayAudio{
		wakeWordPattern:   []float64{0.1, 0.3, 0.8, 0.6, 0.2},
		wakeWordThreshold: 0.7,
	}

	tests := []struct {
		name            string
		audioBuffer     []float32
		expectedConfidence float64
		expectDetection bool
	}{
		{
			name:            "empty buffer",
			audioBuffer:     []float32{},
			expectedConfidence: 0.0,
			expectDetection: false,
		},
		{
			name:            "too short buffer",
			audioBuffer:     make([]float32, 50), // Less than len(pattern) * 100
			expectedConfidence: 0.0,
			expectDetection: false,
		},
		{
			name:            "no pattern match",
			audioBuffer:     make([]float32, 1000), // All zeros
			expectedConfidence: 0.0,
			expectDetection: false,
		},
		{
			name:            "perfect pattern match",
			audioBuffer:     createPatternMatchingBuffer([]float64{0.1, 0.3, 0.8, 0.6, 0.2}, 200),
			expectedConfidence: 0.8, // Should be high confidence
			expectDetection: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := audio.detectWakeWord(tt.audioBuffer)

			if tt.expectDetection {
				if confidence < audio.wakeWordThreshold {
					t.Errorf("Expected wake word detection (confidence >= %f), got %f",
						audio.wakeWordThreshold, confidence)
				}
			} else {
				if confidence >= audio.wakeWordThreshold {
					t.Errorf("Expected no wake word detection (confidence < %f), got %f",
						audio.wakeWordThreshold, confidence)
				}
			}

			// Check confidence is in valid range
			if confidence < 0.0 || confidence > 1.0 {
				t.Errorf("Confidence %f is outside valid range [0.0, 1.0]", confidence)
			}
		})
	}
}

func TestWakeWordConfiguration(t *testing.T) {
	audio := &RelayAudio{
		wakeWordEnabled:   false,
		wakeWordThreshold: 0.5,
	}

	// Test enabling wake word
	audio.EnableWakeWord(true)
	if !audio.wakeWordEnabled {
		t.Error("EnableWakeWord(true) failed to enable wake word detection")
	}

	// Test disabling wake word
	audio.EnableWakeWord(false)
	if audio.wakeWordEnabled {
		t.Error("EnableWakeWord(false) failed to disable wake word detection")
	}

	// Test threshold setting
	testThresholds := []struct {
		input    float64
		expected float64
	}{
		{0.5, 0.5},   // Normal value
		{-0.1, 0.0},  // Below min, should clamp to 0
		{1.5, 1.0},   // Above max, should clamp to 1
		{0.0, 0.0},   // Minimum valid value
		{1.0, 1.0},   // Maximum valid value
	}

	for _, tt := range testThresholds {
		audio.SetWakeWordThreshold(tt.input)
		if audio.wakeWordThreshold != tt.expected {
			t.Errorf("SetWakeWordThreshold(%f): got %f, want %f",
				tt.input, audio.wakeWordThreshold, tt.expected)
		}
	}
}

func TestAudioChunkStruct(t *testing.T) {
	// Test AudioChunk creation and fields
	chunk := AudioChunk{
		Data:          []float32{0.1, 0.2, 0.3},
		SampleRate:    16000,
		Channels:      1,
		Timestamp:     time.Now().UnixNano(),
		IsWakeWord:    true,
		IsEndOfSpeech: false,
	}

	if len(chunk.Data) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(chunk.Data))
	}

	if chunk.SampleRate != 16000 {
		t.Errorf("Expected sample rate 16000, got %d", chunk.SampleRate)
	}

	if chunk.Channels != 1 {
		t.Errorf("Expected 1 channel, got %d", chunk.Channels)
	}

	if !chunk.IsWakeWord {
		t.Error("Expected IsWakeWord to be true")
	}

	if chunk.IsEndOfSpeech {
		t.Error("Expected IsEndOfSpeech to be false")
	}
}

func TestPlayAudio_ErrorConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PortAudio tests in short mode")
	}

	audio, err := NewRelayAudio()
	if err != nil {
		t.Skipf("Failed to create RelayAudio: %v", err)
	}
	defer audio.Shutdown()

	// Test playing audio while already playing
	audioData := []float32{0.1, 0.2, 0.3, 0.4}

	// Start first playback (this may fail if no audio device)
	err1 := audio.PlayAudio(audioData)
	if err1 != nil {
		t.Logf("First PlayAudio failed (may be expected in test environment): %v", err1)
		return // Skip rest of test if audio device unavailable
	}

	// Try to start second playback while first is running
	err2 := audio.PlayAudio(audioData)
	if err2 == nil {
		t.Error("Expected error when playing audio while already playing")
	}

	// Wait for first playback to complete
	time.Sleep(100 * time.Millisecond)
}

// Helper functions

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// createPatternMatchingBuffer creates an audio buffer that matches the given energy pattern
func createPatternMatchingBuffer(pattern []float64, samplesPerChunk int) []float32 {
	totalSamples := len(pattern) * samplesPerChunk
	buffer := make([]float32, totalSamples)

	for i, energy := range pattern {
		start := i * samplesPerChunk
		end := start + samplesPerChunk

		// Create samples with the target energy level
		amplitude := float32(energy)
		for j := start; j < end && j < totalSamples; j++ {
			// Simplified - just use half the energy as amplitude
			buffer[j] = amplitude * float32(0.5)
		}
	}

	return buffer
}