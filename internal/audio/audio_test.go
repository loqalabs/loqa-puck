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
	"math"
	"os"
	"testing"
	"time"
)

func TestPuckAudio_Creation(t *testing.T) {
	// Use mock backend for hardware-independent testing
	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}

	if audio == nil {
		t.Fatal("NewPuckAudio returned nil without error")
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
	audio := &PuckAudio{}

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
	audio := &PuckAudio{
		wakeWordPattern:   []float64{0.1, 0.3, 0.8, 0.6, 0.2},
		wakeWordThreshold: 0.7,
	}

	tests := []struct {
		name               string
		audioBuffer        []float32
		expectedConfidence float64
		expectDetection    bool
	}{
		{
			name:               "empty buffer",
			audioBuffer:        []float32{},
			expectedConfidence: 0.0,
			expectDetection:    false,
		},
		{
			name:               "too short buffer",
			audioBuffer:        make([]float32, 50), // Less than len(pattern) * 100
			expectedConfidence: 0.0,
			expectDetection:    false,
		},
		{
			name:               "no pattern match",
			audioBuffer:        make([]float32, 1000), // All zeros
			expectedConfidence: 0.0,
			expectDetection:    false,
		},
		{
			name:               "perfect pattern match",
			audioBuffer:        createPatternMatchingBuffer([]float64{0.1, 0.3, 0.8, 0.6, 0.2}, 200),
			expectedConfidence: 0.8, // Should be high confidence
			expectDetection:    true,
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
	audio := &PuckAudio{
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
		{0.5, 0.5},  // Normal value
		{-0.1, 0.0}, // Below min, should clamp to 0
		{1.5, 1.0},  // Above max, should clamp to 1
		{0.0, 0.0},  // Minimum valid value
		{1.0, 1.0},  // Maximum valid value
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

	// Skip in CI environments where audio hardware is not available
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}
	defer audio.Shutdown()

	// Test playing audio while already playing
	// Create longer audio data to ensure playback takes some time
	audioData := make([]float32, 8000) // About 0.5 seconds at 16kHz
	for i := range audioData {
		audioData[i] = float32(i) / float32(len(audioData)) * 0.1 // Gentle ramp
	}

	// Start first playback (this may fail if no audio device)
	err1 := audio.PlayAudio(audioData)
	if err1 != nil {
		t.Skipf("PlayAudio failed (may be expected in test environment): %v", err1)
	}

	// Give first playback a moment to start
	time.Sleep(10 * time.Millisecond)

	// Try to start second playback while first is running
	err2 := audio.PlayAudio(audioData)
	if err2 == nil {
		t.Error("Expected error when playing audio while already playing")
	}

	// Wait for first playback to complete
	time.Sleep(600 * time.Millisecond)
}

// Helper functions

// isCIEnvironment detects if we're running in a CI environment
func isCIEnvironment() bool {
	ciEnvVars := []string{
		"CI", // Generic CI indicator
		"CONTINUOUS_INTEGRATION",
		"GITHUB_ACTIONS",   // GitHub Actions
		"GITLAB_CI",        // GitLab CI
		"JENKINS_URL",      // Jenkins
		"TRAVIS",           // Travis CI
		"CIRCLECI",         // CircleCI
		"BUILDKITE",        // Buildkite
		"TEAMCITY_VERSION", // TeamCity
	}

	for _, envVar := range ciEnvVars {
		if os.Getenv(envVar) != "" {
			return true
		}
	}

	return false
}

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

// Test recording lifecycle management
func TestStartRecording_Lifecycle(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}
	defer audio.Shutdown()

	// Test multiple start/stop cycles
	audioChan := make(chan AudioChunk, 10)

	// Test that we can't start recording twice
	err = audio.StartRecording(audioChan)
	if err != nil {
		t.Skipf("StartRecording failed (audio device may not be available): %v", err)
	}

	// Try to start again - should return error
	err = audio.StartRecording(audioChan)
	if err == nil {
		t.Error("Expected error when starting recording twice")
	}

	// Stop recording
	audio.StopRecording()

	// Should be able to start again after stopping
	err = audio.StartRecording(audioChan)
	if err != nil {
		t.Errorf("Failed to restart recording after stop: %v", err)
	}

	audio.StopRecording()
	close(audioChan)
}

// Test concurrent operations
func TestConcurrentOperations(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}
	defer audio.Shutdown()

	// Test concurrent recording and playback
	audioChan := make(chan AudioChunk, 10)
	defer close(audioChan)

	// Start recording
	err = audio.StartRecording(audioChan)
	if err != nil {
		t.Skipf("StartRecording failed (audio device may not be available): %v", err)
	}

	// Play audio while recording
	testData := make([]float32, 4000)
	for i := range testData {
		testData[i] = float32(0.1 * math.Sin(2*math.Pi*880*float64(i)/22050))
	}

	err = audio.PlayAudio(testData)
	if err != nil {
		t.Errorf("PlayAudio failed during recording: %v", err)
	}

	// Stop recording after a short time
	time.Sleep(100 * time.Millisecond)
	audio.StopRecording()
}

// Test deprecated NewRelayAudio function
func TestNewRelayAudio_Deprecated(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	audio, err := NewRelayAudio()
	if err != nil {
		t.Skipf("Failed to create RelayAudio (PortAudio may not be available): %v", err)
	}
	defer audio.Shutdown()

	// Should work the same as NewPuckAudio
	if audio == nil {
		t.Fatal("NewRelayAudio returned nil without error")
	}

	// Verify it has the same default configuration
	if audio.sampleRate != 22050.0 {
		t.Errorf("Expected sample rate 22050, got %f", audio.sampleRate)
	}
}

// Test audio playback worker error conditions
func TestAudioPlaybackWorker(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}
	defer audio.Shutdown()

	// Queue multiple audio chunks for playback
	testData1 := make([]float32, 1000)
	testData2 := make([]float32, 1000)
	testData3 := make([]float32, 1000)

	// Generate different frequencies for each chunk
	for i := range testData1 {
		testData1[i] = float32(0.1 * math.Sin(2*math.Pi*440*float64(i)/22050))
		testData2[i] = float32(0.1 * math.Sin(2*math.Pi*660*float64(i)/22050))
		testData3[i] = float32(0.1 * math.Sin(2*math.Pi*880*float64(i)/22050))
	}

	// Queue audio for playback
	err = audio.PlayAudio(testData1)
	if err != nil {
		t.Errorf("PlayAudio failed for chunk 1: %v", err)
	}

	err = audio.PlayAudio(testData2)
	if err != nil {
		t.Errorf("PlayAudio failed for chunk 2: %v", err)
	}

	err = audio.PlayAudio(testData3)
	if err != nil {
		t.Errorf("PlayAudio failed for chunk 3: %v", err)
	}

	// Wait for playback to complete
	time.Sleep(200 * time.Millisecond)
}

// Test min function
func TestMinFunction(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"a smaller", 5, 10, 5},
		{"b smaller", 10, 5, 5},
		{"equal", 7, 7, 7},
		{"negative numbers", -3, -1, -3},
		{"zero", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Test generateWakeWordPattern function
func TestGenerateWakeWordPattern(t *testing.T) {
	pattern := generateWakeWordPattern()

	if len(pattern) == 0 {
		t.Error("generateWakeWordPattern returned empty pattern")
	}

	// Verify pattern values are in reasonable range
	for i, val := range pattern {
		if val < 0 || val > 1 {
			t.Errorf("Pattern value at index %d is out of range [0,1]: %f", i, val)
		}
	}
}

// Test shutdown behavior
func TestShutdown(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}

	// Start some operations
	audioChan := make(chan AudioChunk, 10)

	err = audio.StartRecording(audioChan)
	if err != nil {
		// If recording fails, continue with shutdown test
		t.Logf("StartRecording failed (expected in some environments): %v", err)
	}

	// Queue some audio
	testData := make([]float32, 1000)
	for i := range testData {
		testData[i] = float32(0.1 * math.Sin(2*math.Pi*440*float64(i)/22050))
	}
	_ = audio.PlayAudio(testData)

	// Test shutdown
	audio.Shutdown()

	// Verify recording is stopped
	if audio.isRecording {
		t.Error("Recording should be stopped after shutdown")
	}

	close(audioChan)
}

// Test playAudioChunk function behavior
func TestPlayAudioChunk(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping audio hardware tests in CI environment")
	}

	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio with mock backend: %v", err)
	}
	defer audio.Shutdown()

	// Test with empty audio data
	err = audio.playAudioChunk([]float32{})
	if err != nil {
		t.Errorf("playAudioChunk should handle empty data gracefully: %v", err)
	}

	// Test with normal audio data
	testData := make([]float32, 2048)
	for i := range testData {
		testData[i] = float32(0.2 * math.Sin(2*math.Pi*440*float64(i)/22050))
	}

	err = audio.playAudioChunk(testData)
	if err != nil {
		t.Errorf("playAudioChunk failed with normal data: %v", err)
	}

	// Wait for playback to complete
	time.Sleep(100 * time.Millisecond)
}

// TestMockBackendErrorInjection tests error injection capabilities
func TestMockBackendErrorInjection(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*MockAudioBackend)
		action  func(*PuckAudio) error
		wantErr bool
	}{
		{
			name: "initialization_error",
			setup: func(mb *MockAudioBackend) {
				mb.SetInitError(fmt.Errorf("init failed"))
			},
			action: func(pa *PuckAudio) error {
				return nil // Error happens during construction
			},
			wantErr: true,
		},
		{
			name: "stream_creation_error",
			setup: func(mb *MockAudioBackend) {
				mb.SetCreateStreamError(fmt.Errorf("stream creation failed"))
			},
			action: func(pa *PuckAudio) error {
				audioChan := make(chan AudioChunk, 10)
				return pa.StartRecording(audioChan)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockBackend := NewMockAudioBackend()
			tt.setup(mockBackend)

			audio, err := NewPuckAudio(mockBackend)
			if tt.name == "initialization_error" {
				if err == nil {
					t.Error("Expected initialization error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Failed to create PuckAudio: %v", err)
			}
			defer audio.Shutdown()

			err = tt.action(audio)
			if (err != nil) != tt.wantErr {
				t.Errorf("action() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestMockBackendDataCollection tests mock data collection capabilities
func TestMockBackendDataCollection(t *testing.T) {
	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio: %v", err)
	}
	defer audio.Shutdown()

	// Test audio data recording
	testData := []float32{0.1, 0.2, 0.3, 0.4}
	err = audio.PlayAudio(testData)
	if err != nil {
		t.Fatalf("Failed to play audio: %v", err)
	}

	// Wait for playback to complete
	time.Sleep(200 * time.Millisecond)

	// Check that data was recorded
	playbackData := mockBackend.GetPlaybackAudioData()
	if len(playbackData) == 0 {
		t.Error("Expected playback data to be recorded")
	}
}

// TestMockStreamErrorInjection tests stream-level error injection
func TestMockStreamErrorInjection(t *testing.T) {
	mockBackend := NewMockAudioBackend()
	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio: %v", err)
	}
	defer audio.Shutdown()

	// Create a stream and inject errors
	stream, err := mockBackend.CreateInputStream(22050, 1, 1024)
	if err != nil {
		t.Fatalf("Failed to create input stream: %v", err)
	}

	mockStream := stream.(*MockStream)
	mockStream.SetStartError(fmt.Errorf("start failed"))

	// Test start error
	err = stream.Start()
	if err == nil {
		t.Error("Expected start error")
	}

	// Reset error and test write error
	mockStream.SetStartError(nil)
	mockStream.SetWriteError(fmt.Errorf("write failed"))

	if err := stream.Start(); err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Test write error
	err = stream.Write([]float32{0.1, 0.2})
	if err == nil {
		t.Error("Expected write error")
	}
}

// TestMockTimingSimulation tests timing simulation features
func TestMockTimingSimulation(t *testing.T) {
	mockBackend := NewMockAudioBackend()

	// Test with timing simulation disabled
	mockBackend.SetSimulateRealTiming(false)

	audio, err := NewPuckAudio(mockBackend)
	if err != nil {
		t.Fatalf("Failed to create PuckAudio: %v", err)
	}
	defer audio.Shutdown()

	// Test playback with no timing delay
	testData := []float32{0.1, 0.2, 0.3, 0.4}
	start := time.Now()
	err = audio.PlayAudio(testData)
	if err != nil {
		t.Fatalf("Failed to play audio: %v", err)
	}

	// Should complete quickly without timing simulation
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected fast playback without timing simulation, took %v", elapsed)
	}
}

// TestStartRecording_ErrorPaths tests error paths in StartRecording
func TestStartRecording_ErrorPaths(t *testing.T) {
	t.Run("backend_initialization_error", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		mockBackend.SetInitError(fmt.Errorf("backend init failed"))

		_, err := NewPuckAudio(mockBackend)
		if err == nil {
			t.Fatal("Expected error with backend initialization failure")
		}
	})

	t.Run("stream_creation_error", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}
		defer audio.Shutdown()

		// Set error for stream creation
		mockBackend.SetCreateStreamError(fmt.Errorf("stream creation failed"))

		audioChan := make(chan AudioChunk, 10)
		err = audio.StartRecording(audioChan)
		if err == nil {
			t.Error("Expected error with stream creation failure")
		}
	})

	t.Run("stream_start_error", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}
		defer audio.Shutdown()

		audioChan := make(chan AudioChunk, 10)

		// Start recording first time - should succeed
		err = audio.StartRecording(audioChan)
		if err != nil {
			t.Fatalf("First StartRecording failed: %v", err)
		}

		// Create a new input stream and inject start error
		inputStream, _ := mockBackend.CreateInputStream(audio.sampleRate, audio.channels, audio.framesPerBuffer)
		if mockStream, ok := inputStream.(*MockStream); ok {
			mockStream.SetStartError(fmt.Errorf("start failed"))
		}

		audio.StopRecording()

		// Try again - might fail due to start error
		_ = audio.StartRecording(audioChan) // Ignore errors in test error injection scenario
	})

	t.Run("recording_with_nil_channel", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}
		defer audio.Shutdown()

		// Try to start recording with nil channel
		err = audio.StartRecording(nil)
		if err == nil {
			t.Error("Expected error with nil audio channel")
		}
	})
}

// TestShutdown_ComprehensiveCleanup tests thorough shutdown behavior
func TestShutdown_ComprehensiveCleanup(t *testing.T) {
	t.Run("shutdown_during_recording", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}

		audioChan := make(chan AudioChunk, 10)

		// Start recording
		err = audio.StartRecording(audioChan)
		if err != nil {
			t.Logf("StartRecording failed (may be expected): %v", err)
		}

		// Start playback
		testData := make([]float32, 1000)
		for i := range testData {
			testData[i] = float32(0.1 * math.Sin(2*math.Pi*440*float64(i)/22050))
		}
		_ = audio.PlayAudio(testData)

		// Shutdown should clean everything up
		audio.Shutdown()

		// Verify recording is stopped
		if audio.isRecording {
			t.Error("Recording should be stopped after shutdown")
		}

		close(audioChan)
	})

	t.Run("shutdown_with_backend_error", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		mockBackend.SetInitError(nil) // Allow initialization
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}

		// Set terminate error
		mockBackend.terminateError = fmt.Errorf("terminate failed")

		// Shutdown should handle backend errors gracefully
		audio.Shutdown()
	})

	t.Run("multiple_shutdowns", func(t *testing.T) {
		mockBackend := NewMockAudioBackend()
		audio, err := NewPuckAudio(mockBackend)
		if err != nil {
			t.Fatalf("Failed to create PuckAudio: %v", err)
		}

		// Multiple shutdowns should be safe
		audio.Shutdown()
		audio.Shutdown()
		audio.Shutdown()
	})
}

// TestAudioDataProcessing tests audio data processing edge cases
func TestAudioDataProcessing(t *testing.T) {
	t.Run("various_audio_formats", func(t *testing.T) {
		testCases := []struct {
			name string
			data []float32
		}{
			{"empty_audio", []float32{}},
			{"single_sample", []float32{0.5}},
			{"very_quiet", []float32{0.001, 0.001, 0.001}},
			{"very_loud", []float32{0.99, -0.99, 0.99}},
			{"mixed_levels", []float32{0.1, 0.5, 0.9, 0.05, 0.75}},
			{"dc_offset", []float32{0.5, 0.5, 0.5, 0.5}},
			{"alternating", []float32{1.0, -1.0, 1.0, -1.0}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				mockBackend := NewMockAudioBackend()
				audio, err := NewPuckAudio(mockBackend)
				if err != nil {
					t.Fatalf("Failed to create PuckAudio: %v", err)
				}
				defer audio.Shutdown()

				err = audio.PlayAudio(tc.data)
				if err != nil {
					t.Errorf("PlayAudio failed for %s: %v", tc.name, err)
				}
			})
		}
	})

	t.Run("wake_word_detection_edge_cases", func(t *testing.T) {
		audio := &PuckAudio{
			wakeWordPattern:   []float64{0.1, 0.3, 0.8, 0.6, 0.2},
			wakeWordThreshold: 0.7,
		}

		edgeCases := []struct {
			name     string
			buffer   []float32
			expected bool
		}{
			{"all_zeros", make([]float32, 1000), false},
			{"very_short", make([]float32, 10), false},
			{"pattern_at_end", func() []float32 {
				buf := make([]float32, 1000)
				// Put a strong pattern at the end
				start := len(buf) - 500
				for i := 0; i < 5; i++ {
					for j := 0; j < 100; j++ {
						if start+i*100+j < len(buf) {
							buf[start+i*100+j] = float32(audio.wakeWordPattern[i])
						}
					}
				}
				return buf
			}(), true},
		}

		for _, tc := range edgeCases {
			t.Run(tc.name, func(t *testing.T) {
				confidence := audio.detectWakeWord(tc.buffer)
				detected := confidence >= audio.wakeWordThreshold

				if detected != tc.expected {
					t.Errorf("Wake word detection for %s: got %v, want %v (confidence: %f)",
						tc.name, detected, tc.expected, confidence)
				}
			})
		}
	})
}
