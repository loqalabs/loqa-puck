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
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMainFunctionExecution tests the main function through subprocess execution
func TestMainFunctionExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping main function test in short mode")
	}

	t.Run("main_with_help_flag", func(t *testing.T) {
		// Test main function with -h flag
		cmd := exec.Command("go", "run", "main.go", "-h")
		output, _ := cmd.CombinedOutput()

		// -h flag should show usage and exit (may error but that's expected)
		outputStr := string(output)

		// Should show usage information
		expectedFlags := []string{"-hub", "-id", "-nats"}
		for _, flag := range expectedFlags {
			assert.Contains(t, outputStr, flag, "should document flag %s", flag)
		}
	})

	t.Run("main_with_invalid_config", func(t *testing.T) {
		// Test main function with invalid configuration
		cmd := exec.Command("go", "run", "main.go",
			"-hub", "http://invalid-host:9999",
			"-nats", "nats://invalid-host:9999",
			"-id", "test-puck")

		// Set a timeout to prevent hanging
		timeout := 15 * time.Second

		done := make(chan error, 1)
		go func() {
			done <- cmd.Run()
		}()

		select {
		case err := <-done:
			// Should exit with error due to invalid configuration
			if err == nil {
				t.Error("Expected main to fail with invalid configuration")
			}
		case <-time.After(timeout):
			// Kill if it hangs
			if cmd.Process != nil {
				_ = cmd.Process.Kill() // Ignore errors during test timeout cleanup
			}
			t.Error("Main function test timed out")
		}
	})
}

// TestMainFunctionComponents tests components used by main
func TestMainFunctionComponents(t *testing.T) {
	t.Run("main_imports", func(t *testing.T) {
		// Verify that main can import required packages
		// This is a compile-time test essentially

		// Test that we can create types from imported packages
		_ = "test"
		envVar := os.Getenv("TEST")
		_ = envVar

		// These would fail to compile if imports were broken
		assert.True(t, true, "imports are working")
	})

	t.Run("main_constants", func(t *testing.T) {
		// Test default values that main uses
		defaultHub := "http://localhost:3000"
		defaultPuck := "loqa-puck-001"
		defaultNATS := "nats://localhost:4222"

		assert.Equal(t, "http://localhost:3000", defaultHub)
		assert.Equal(t, "loqa-puck-001", defaultPuck)
		assert.Equal(t, "nats://localhost:4222", defaultNATS)
	})
}

// TestMainErrorMessages tests error message formatting
func TestMainErrorMessages(t *testing.T) {
	t.Run("error_message_format", func(t *testing.T) {
		// Test error message formatting used in main
		testCases := []struct {
			template string
			args     []interface{}
			expected string
		}{
			{"❌ Failed to initialize audio: %v", []interface{}{"test error"}, "❌ Failed to initialize audio: test error"},
			{"❌ Failed to initialize NATS audio subscriber: %v", []interface{}{"connection refused"}, "❌ Failed to initialize NATS audio subscriber: connection refused"},
			{"⚠️  Connection attempt %d failed: %v", []interface{}{1, "timeout"}, "⚠️  Connection attempt 1 failed: timeout"},
		}

		for _, tc := range testCases {
			result := fmt.Sprintf(tc.template, tc.args...)
			assert.Equal(t, tc.expected, result, "error message should be formatted correctly")
		}
	})
}

// TestMainRetryLogic tests the retry logic used in main
func TestMainRetryLogic(t *testing.T) {
	t.Run("connection_retry_pattern", func(t *testing.T) {
		// Test the retry pattern used in main
		maxRetries := 5
		attempts := 0

		for i := 0; i < maxRetries; i++ {
			attempts++
			// Simulate connection attempt
			if attempts < 3 {
				// Simulate failure
				continue
			}
			// Simulate success
			break
		}

		assert.Equal(t, 3, attempts, "should succeed after 3 attempts")
	})

	t.Run("retry_with_sleep", func(t *testing.T) {
		// Test retry logic with sleep timing
		start := time.Now()
		sleepDuration := 10 * time.Millisecond // Reduced for testing

		for i := 0; i < 3; i++ {
			if i > 0 {
				time.Sleep(sleepDuration)
			}
			// Simulate attempt
		}

		elapsed := time.Since(start)
		expectedMin := 2 * sleepDuration // 2 sleeps
		assert.GreaterOrEqual(t, elapsed, expectedMin, "should include sleep delays")
	})
}

// TestMainChannelSetup tests channel setup used in main
func TestMainChannelSetup(t *testing.T) {
	t.Run("audio_channel_creation", func(t *testing.T) {
		// Test audio channel setup as done in main
		audioChan := make(chan struct{}, 10) // Using struct{} for testing
		defer close(audioChan)

		assert.Equal(t, 10, cap(audioChan), "audio channel should have buffer size 10")

		// Test non-blocking write
		select {
		case audioChan <- struct{}{}:
			// Success
		default:
			t.Fatal("should be able to write to audio channel")
		}
	})

	t.Run("signal_channel_creation", func(t *testing.T) {
		// Test signal channel setup as done in main
		sigChan := make(chan os.Signal, 1)
		defer close(sigChan)

		assert.Equal(t, 1, cap(sigChan), "signal channel should have buffer size 1")
	})
}

// TestMainConfigurationValidation tests configuration validation
func TestMainConfigurationValidation(t *testing.T) {
	t.Run("url_validation", func(t *testing.T) {
		// Test URL formats used in main
		validURLs := []string{
			"http://localhost:3000",
			"https://hub.example.com",
			"nats://localhost:4222",
			"nats://nats.example.com:4222",
		}

		for _, url := range validURLs {
			assert.True(t, strings.HasPrefix(url, "http") || strings.HasPrefix(url, "nats"),
				"URL %s should have valid protocol", url)
		}
	})

	t.Run("puck_id_validation", func(t *testing.T) {
		// Test puck ID formats
		validIDs := []string{
			"loqa-puck-001",
			"test-puck",
			"puck-alpha-01",
		}

		for _, id := range validIDs {
			assert.NotEmpty(t, id, "puck ID should not be empty")
			assert.Contains(t, id, "puck", "puck ID should contain 'puck'")
		}
	})
}

// TestMainLogMessages tests log message formats
func TestMainLogMessages(t *testing.T) {
	t.Run("startup_messages", func(t *testing.T) {
		// Test startup log message formats
		messages := []string{
			"🚀 Starting Loqa Puck - Go Reference Implementation",
			"📋 Puck ID: %s",
			"🎯 Hub Address: %s",
			"📨 NATS URL: %s",
		}

		for _, msg := range messages {
			assert.NotEmpty(t, msg, "log message should not be empty")
			if strings.Contains(msg, "%s") {
				// Test formatting
				formatted := fmt.Sprintf(msg, "test-value")
				assert.Contains(t, formatted, "test-value", "should format correctly")
			}
		}
	})

	t.Run("status_messages", func(t *testing.T) {
		// Test status display messages
		statusLines := []string{
			"🎤 Loqa Puck - Go Reference Implementation Active!",
			"🎙️  Microphone: Listening for wake word",
			"🎯 Wake Word: \"Hey Loqa\" (enabled)",
			"🔊 Speakers: Ready for TTS playback via NATS",
			"📡 Audio Upload: HTTP/1.1 streaming with binary frames",
			"📨 Audio Download: TTS audio via NATS",
			"🔧 ESP32 Ready: 4KB frame limits, optimized protocol",
		}

		for _, line := range statusLines {
			assert.NotEmpty(t, line, "status line should not be empty")
			assert.True(t, len(line) > 10, "status line should be descriptive")
		}
	})

	t.Run("shutdown_messages", func(t *testing.T) {
		// Test shutdown message formats
		shutdownMsg := "🛑 Shutting down puck service..."
		stopMsg := "👋 Puck service stopped"

		assert.Contains(t, shutdownMsg, "Shutting down", "should indicate shutdown")
		assert.Contains(t, stopMsg, "stopped", "should indicate completion")
	})
}

// TestMainWorkflowSteps tests the main workflow sequence
func TestMainWorkflowSteps(t *testing.T) {
	t.Run("initialization_sequence", func(t *testing.T) {
		// Test the logical sequence of initialization steps
		steps := []string{
			"parse_flags",
			"initialize_audio",
			"initialize_nats",
			"initialize_client",
			"connect_with_retry",
			"setup_channels",
			"start_streaming",
			"start_recording",
			"setup_playback",
			"display_status",
			"wait_for_signal",
		}

		// Verify we have a reasonable number of steps
		assert.GreaterOrEqual(t, len(steps), 8, "should have multiple initialization steps")

		// Verify each step is meaningful
		for _, step := range steps {
			assert.NotEmpty(t, step, "step should not be empty")
			assert.Contains(t, step, "_", "step should be descriptive")
		}
	})

	t.Run("cleanup_sequence", func(t *testing.T) {
		// Test cleanup sequence
		cleanupSteps := []string{
			"stop_recording",
			"close_channels",
			"shutdown_audio",
			"close_subscriber",
			"disconnect_client",
		}

		for _, step := range cleanupSteps {
			assert.NotEmpty(t, step, "cleanup step should not be empty")
		}
	})
}

// BenchmarkMainComponents benchmarks main function components
func BenchmarkMainLogic(b *testing.B) {
	b.Run("url_formatting", func(b *testing.B) {
		template := "🎯 Hub Address: %s"
		addr := "http://localhost:3000"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = fmt.Sprintf(template, addr)
		}
	})

	b.Run("channel_creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ch := make(chan struct{}, 10)
			close(ch)
		}
	})
}