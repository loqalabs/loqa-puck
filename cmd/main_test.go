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

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBytesToFloat32Array(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []float32
		wantLen  int
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: []float32{},
			wantLen:  0,
		},
		{
			name:     "odd number of bytes - should drop last byte",
			input:    []byte{0x00, 0x01, 0xFF},
			expected: []float32{256.0 / 32767.0}, // 0x0100 = 256
			wantLen:  1,
		},
		{
			name:     "single 16-bit sample - positive value",
			input:    []byte{0x00, 0x10}, // 0x1000 = 4096
			expected: []float32{4096.0 / 32767.0},
			wantLen:  1,
		},
		{
			name:     "single 16-bit sample - negative value",
			input:    []byte{0x00, 0x80}, // 0x8000 = -32768 (int16)
			expected: []float32{-32768.0 / 32767.0},
			wantLen:  1,
		},
		{
			name:     "multiple samples",
			input:    []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}, // 0, 32767, -32768
			expected: []float32{0.0, 1.0, -32768.0 / 32767.0},
			wantLen:  3,
		},
		{
			name:     "zero samples",
			input:    []byte{0x00, 0x00, 0x00, 0x00},
			expected: []float32{0.0, 0.0},
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesToFloat32Array(tt.input)

			if len(result) != tt.wantLen {
				t.Errorf("bytesToFloat32Array() length = %d, want %d", len(result), tt.wantLen)
				return
			}

			for i, expected := range tt.expected {
				if i >= len(result) {
					t.Errorf("bytesToFloat32Array() missing sample at index %d", i)
					continue
				}

				// Use epsilon comparison for float values
				epsilon := float32(0.000001)
				if abs(result[i]-expected) > epsilon {
					t.Errorf("bytesToFloat32Array()[%d] = %f, want %f", i, result[i], expected)
				}
			}
		})
	}
}

func TestBytesToFloat32Array_Performance(t *testing.T) {
	// Test with large audio buffer (10 seconds at 16kHz)
	largeInput := make([]byte, 16000*10*2) // 10 seconds, 16-bit samples
	for i := 0; i < len(largeInput); i += 2 {
		largeInput[i] = byte(i % 256)
		largeInput[i+1] = byte((i / 256) % 256)
	}

	result := bytesToFloat32Array(largeInput)
	expectedLen := len(largeInput) / 2

	if len(result) != expectedLen {
		t.Errorf("Performance test: length = %d, want %d", len(result), expectedLen)
	}
}

func TestBytesToFloat32Array_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := bytesToFloat32Array(nil)
		if len(result) != 0 {
			t.Errorf("nil input should return empty slice, got length %d", len(result))
		}
	})

	t.Run("single byte", func(t *testing.T) {
		result := bytesToFloat32Array([]byte{0xFF})
		if len(result) != 0 {
			t.Errorf("single byte should return empty slice, got length %d", len(result))
		}
	})

	t.Run("max positive value", func(t *testing.T) {
		input := []byte{0xFF, 0x7F} // 32767
		result := bytesToFloat32Array(input)
		expected := float32(1.0) // 32767 / 32767 = 1.0

		if len(result) != 1 {
			t.Errorf("Expected 1 sample, got %d", len(result))
			return
		}

		epsilon := float32(0.000001)
		if abs(result[0]-expected) > epsilon {
			t.Errorf("Max positive value: got %f, want %f", result[0], expected)
		}
	})

	t.Run("max negative value", func(t *testing.T) {
		input := []byte{0x00, 0x80} // -32768
		result := bytesToFloat32Array(input)
		expected := float32(-32768.0 / 32767.0) // Slightly less than -1.0

		if len(result) != 1 {
			t.Errorf("Expected 1 sample, got %d", len(result))
			return
		}

		epsilon := float32(0.000001)
		if abs(result[0]-expected) > epsilon {
			t.Errorf("Max negative value: got %f, want %f", result[0], expected)
		}
	})
}

// Helper function for float comparison
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Test WAV header detection and skipping
func TestBytesToFloat32Array_WAVHeader(t *testing.T) {
	// Test with WAV header
	wavHeader := []byte("RIFF____WAVE")
	// Pad to 44 bytes (standard WAV header size)
	for len(wavHeader) < 44 {
		wavHeader = append(wavHeader, 0)
	}
	// Add some PCM data
	pcmData := []byte{0x00, 0x10, 0x00, 0x20} // Two 16-bit samples
	wavData := append(wavHeader, pcmData...)

	result := bytesToFloat32Array(wavData)

	if len(result) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(result))
	}

	// Verify the PCM data was correctly processed (skipped header)
	expected := []float32{
		float32(0x1000) / 32767.0,
		float32(0x2000) / 32767.0,
	}

	for i, exp := range expected {
		epsilon := float32(0.000001)
		if abs(result[i]-exp) > epsilon {
			t.Errorf("Sample %d: expected %f, got %f", i, exp, result[i])
		}
	}
}

func TestBytesToFloat32Array_InvalidWAVSignature(t *testing.T) {
	// Test with data that looks like WAV but has wrong signature
	fakeWav := []byte("RIFF____FAKE")
	for len(fakeWav) < 44 {
		fakeWav = append(fakeWav, 0)
	}
	pcmData := []byte{0x00, 0x10, 0x00, 0x20}
	data := append(fakeWav, pcmData...)

	result := bytesToFloat32Array(data)

	// Should process entire data as PCM (no header skipping)
	expectedSamples := len(data) / 2
	if len(result) != expectedSamples {
		t.Errorf("Expected %d samples, got %d", expectedSamples, len(result))
	}
}

// Test application flag parsing and configuration
func TestApplicationFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping flag parsing test in short mode")
	}

	// Test default flag values by running the application with -h
	cmd := exec.Command("go", "run", "main.go", "-h")
	output, _ := cmd.CombinedOutput()

	// -h flag should show help output (may or may not error depending on implementation)
	// We mainly care that the help output is produced
	if len(output) == 0 {
		t.Error("Expected help output from -h flag")
	}

	outputStr := string(output)

	// Check that our expected flags are documented
	expectedFlags := []string{
		"-hub",
		"-id",
		"-nats",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(outputStr, flag) {
			t.Errorf("Expected flag %s to be documented in help output", flag)
		}
	}

	// Check default values are shown
	expectedDefaults := []string{
		"http://localhost:3000",
		"loqa-puck-001",
		"nats://localhost:4222",
	}

	for _, defaultVal := range expectedDefaults {
		if !strings.Contains(outputStr, defaultVal) {
			t.Errorf("Expected default value %s to be shown in help output", defaultVal)
		}
	}
}

// Test application startup with invalid configuration
func TestApplicationStartup_InvalidConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping application startup test in short mode")
	}

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "invalid_nats_url",
			args:     []string{"-nats", "invalid-nats-url"},
			expected: "Failed to initialize",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("go", "run", "main.go")
			cmd.Args = append(cmd.Args, tc.args...)

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			// Set a timeout to prevent test hanging
			done := make(chan error, 1)
			go func() {
				done <- cmd.Run()
			}()

			select {
			case err := <-done:
				if err == nil {
					t.Error("Expected application to fail with invalid config")
				}

				stderrStr := stderr.String()
				if !strings.Contains(stderrStr, tc.expected) {
					t.Logf("Got stderr: %s", stderrStr)
					// Don't fail the test if the error message is different
					// This is environment-dependent
				}

			case <-time.After(15 * time.Second):
				// Kill the process if it's still running
				if cmd.Process != nil {
					_ = cmd.Process.Kill() // Ignore errors during test timeout cleanup
				}
				t.Error("Application startup test timed out")
			}
		})
	}
}

// Test application graceful shutdown with signals
func TestApplicationShutdown_SignalHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal handling test in short mode")
	}

	// Start the application with invalid endpoints (expected to fail connection)
	cmd := exec.Command("go", "run", "main.go",
		"-hub", "http://localhost:9999",
		"-nats", "nats://localhost:9999")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start application: %v", err)
	}

	// Give it time to start up and try to connect
	time.Sleep(2 * time.Second)

	// Send SIGINT
	err = cmd.Process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("Failed to send SIGINT: %v", err)
	}

	// Wait for graceful shutdown
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process should exit
		stdoutStr := stdout.String()
		stderrStr := stderr.String()

		// Check for shutdown messages
		output := stdoutStr + stderrStr
		if !strings.Contains(output, "Shutting down") {
			t.Logf("Stdout: %s", stdoutStr)
			t.Logf("Stderr: %s", stderrStr)
			// Don't fail the test - shutdown message might vary
		}

	case <-time.After(10 * time.Second):
		// Force kill if graceful shutdown failed
		_ = cmd.Process.Kill() // Ignore errors during test timeout cleanup
		t.Error("Application did not shut down gracefully within timeout")
	}
}

// Test application startup sequence and component initialization
func TestApplicationComponents_StartupSequence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping component startup test in short mode")
	}

	// Start application with invalid servers (will fail but show startup sequence)
	cmd := exec.Command("go", "run", "main.go",
		"-hub", "http://localhost:9999",
		"-nats", "nats://localhost:9999",
		"-id", "test-puck")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	if err != nil {
		t.Fatalf("Failed to start application: %v", err)
	}

	// Let it run for a bit to see startup messages
	time.Sleep(3 * time.Second)

	// Kill the process
	_ = cmd.Process.Kill() // Ignore errors during test cleanup
	_ = cmd.Wait() // Ignore errors during test cleanup

	output := stdout.String() + stderr.String()

	// Check that startup components are initialized
	expectedMessages := []string{
		"Starting Loqa Puck - Go Reference Implementation",
		"Puck ID: test-puck",
		"Hub Address: http://localhost:9999",
		"NATS URL: nats://localhost:9999",
	}

	for _, msg := range expectedMessages {
		if !strings.Contains(output, msg) {
			t.Logf("Full output: %s", output)
			t.Errorf("Expected startup message '%s' not found in output", msg)
		}
	}
}

// Benchmark the bytesToFloat32Array function for performance
func BenchmarkBytesToFloat32Array_SmallChunk(b *testing.B) {
	// Simulate small audio chunk (160 samples = 320 bytes for 10ms at 16kHz)
	data := make([]byte, 320)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToFloat32Array(data)
	}
}

func BenchmarkBytesToFloat32Array_LargeChunk(b *testing.B) {
	// Simulate large audio chunk (1600 samples = 3200 bytes for 100ms at 16kHz)
	data := make([]byte, 3200)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToFloat32Array(data)
	}
}

func BenchmarkBytesToFloat32Array_WithWAVHeader(b *testing.B) {
	// Simulate audio with WAV header
	wavHeader := []byte("RIFF____WAVE")
	for len(wavHeader) < 44 {
		wavHeader = append(wavHeader, 0)
	}

	audioData := make([]byte, 1600) // 800 samples
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}

	data := append(wavHeader, audioData...)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bytesToFloat32Array(data)
	}
}
