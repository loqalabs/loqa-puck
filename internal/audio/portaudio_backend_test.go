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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPortAudioBackend tests the PortAudio backend implementation
func TestPortAudioBackend(t *testing.T) {
	// Skip if in CI environment where PortAudio may not be available
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	t.Run("backend_creation", func(t *testing.T) {
		backend := NewPortAudioBackend()
		require.NotNil(t, backend, "should create PortAudio backend")
		assert.False(t, backend.initialized, "should not be initialized by default")
	})

	t.Run("initialization", func(t *testing.T) {
		backend := NewPortAudioBackend()

		err := backend.Initialize()
		if err != nil {
			// PortAudio may not be available in test environment
			t.Skipf("PortAudio initialization failed (may be expected): %v", err)
		}

		assert.True(t, backend.initialized, "should be marked as initialized")

		// Cleanup
		_ = backend.Terminate() // Ignore errors during test cleanup
	})

	t.Run("double_initialization", func(t *testing.T) {
		backend := NewPortAudioBackend()

		err := backend.Initialize()
		if err != nil {
			t.Skipf("PortAudio initialization failed (may be expected): %v", err)
		}

		// Second initialization should be safe
		err = backend.Initialize()
		assert.NoError(t, err, "double initialization should be safe")

		_ = backend.Terminate() // Ignore errors during test cleanup
	})

	t.Run("termination", func(t *testing.T) {
		backend := NewPortAudioBackend()

		err := backend.Initialize()
		if err != nil {
			t.Skipf("PortAudio initialization failed (may be expected): %v", err)
		}

		err = backend.Terminate()
		assert.NoError(t, err, "should terminate successfully")
		assert.False(t, backend.initialized, "should be marked as not initialized")
	})

	t.Run("terminate_without_init", func(t *testing.T) {
		backend := NewPortAudioBackend()

		// Should handle terminate without initialization
		err := backend.Terminate()
		assert.NoError(t, err, "should handle terminate without init")
	})
}

// TestPortAudioStream tests PortAudio stream operations
func TestPortAudioStream(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	backend := NewPortAudioBackend()
	err := backend.Initialize()
	if err != nil {
		t.Skipf("PortAudio initialization failed (may be expected): %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	t.Run("create_input_stream", func(t *testing.T) {
		stream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			t.Skipf("CreateInputStream failed (may be expected): %v", err)
		}
		require.NotNil(t, stream, "should create input stream")
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup
	})

	t.Run("create_output_stream", func(t *testing.T) {
		stream, err := backend.CreateOutputStream(16000, 1, 512)
		if err != nil {
			t.Skipf("CreateOutputStream failed (may be expected): %v", err)
		}
		require.NotNil(t, stream, "should create output stream")
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup
	})

	t.Run("stream_without_initialization", func(t *testing.T) {
		uninitBackend := NewPortAudioBackend()

		stream, err := uninitBackend.CreateInputStream(16000, 1, 512)
		require.Error(t, err, "should fail without initialization")
		assert.Nil(t, stream, "stream should be nil on error")
		assert.Contains(t, err.Error(), "not initialized", "error should mention initialization")
	})
}

// TestPortAudioStreamOperations tests stream operations
func TestPortAudioStreamOperations(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	backend := NewPortAudioBackend()
	err := backend.Initialize()
	if err != nil {
		t.Skipf("PortAudio initialization failed (may be expected): %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	t.Run("input_stream_operations", func(t *testing.T) {
		stream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			t.Skipf("CreateInputStream failed (may be expected): %v", err)
		}
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		// Test start
		err = stream.Start()
		if err != nil {
			t.Skipf("Stream start failed (may be expected): %v", err)
		}

		// Test IsActive
		isActive := stream.IsActive()
		assert.True(t, isActive, "stream should report as active")

		// Test read
		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		if err != nil {
			t.Logf("Stream read failed (may be expected): %v", err)
		}

		// Test write to input stream (should fail)
		err = stream.Write(buffer)
		require.Error(t, err, "should not allow writing to input stream")
		assert.Contains(t, err.Error(), "cannot write to input stream", "should have appropriate error message")

		// Test stop
		err = stream.Stop()
		if err != nil {
			t.Logf("Stream stop failed (may be expected): %v", err)
		}
	})

	t.Run("output_stream_operations", func(t *testing.T) {
		stream, err := backend.CreateOutputStream(16000, 1, 512)
		if err != nil {
			t.Skipf("CreateOutputStream failed (may be expected): %v", err)
		}
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		// Test start
		err = stream.Start()
		if err != nil {
			t.Skipf("Stream start failed (may be expected): %v", err)
		}

		// Test write
		buffer := make([]float32, 512)
		for i := range buffer {
			buffer[i] = 0.1 // Small test signal
		}
		err = stream.Write(buffer)
		if err != nil {
			t.Logf("Stream write failed (may be expected): %v", err)
		}

		// Test read from output stream (should fail)
		err = stream.Read(buffer)
		require.Error(t, err, "should not allow reading from output stream")
		assert.Contains(t, err.Error(), "cannot read from output stream", "should have appropriate error message")

		// Test stop
		err = stream.Stop()
		if err != nil {
			t.Logf("Stream stop failed (may be expected): %v", err)
		}
	})

	t.Run("stream_callback", func(t *testing.T) {
		stream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			t.Skipf("CreateInputStream failed (may be expected): %v", err)
		}
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		// Test setting callback
		callback := func(input, output []float32) error {
			return nil
		}

		err = stream.SetCallback(callback)
		assert.NoError(t, err, "should set callback successfully")

		// Note: The actual callback isn't triggered in this implementation
		// This just tests that SetCallback doesn't error
	})
}

// TestPortAudioStreamErrorConditions tests error conditions
func TestPortAudioStreamErrorConditions(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	t.Run("operations_on_nil_stream", func(t *testing.T) {
		// Create a stream with nil internal stream (simulating error condition)
		stream := &PortAudioStream{
			stream:  nil,
			isInput: true,
		}

		// All operations should fail gracefully
		err := stream.Start()
		require.Error(t, err, "should fail with nil stream")
		assert.Contains(t, err.Error(), "stream is nil")

		err = stream.Stop()
		require.Error(t, err, "should fail with nil stream")
		assert.Contains(t, err.Error(), "stream is nil")

		err = stream.Close()
		require.Error(t, err, "should fail with nil stream")
		assert.Contains(t, err.Error(), "stream is nil")

		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.Error(t, err, "should fail with nil stream")
		assert.Contains(t, err.Error(), "stream is nil")

		err = stream.Write(buffer)
		require.Error(t, err, "should fail with nil stream")
		assert.Contains(t, err.Error(), "stream is nil")

		isActive := stream.IsActive()
		assert.False(t, isActive, "nil stream should not be active")
	})

	t.Run("stream_direction_validation", func(t *testing.T) {
		// Test input stream validation
		inputStream := &PortAudioStream{
			isInput: true,
		}

		buffer := make([]float32, 512)
		err := inputStream.Write(buffer)
		require.Error(t, err, "should not allow writing to input stream")

		// Test output stream validation
		outputStream := &PortAudioStream{
			isInput: false,
		}

		err = outputStream.Read(buffer)
		require.Error(t, err, "should not allow reading from output stream")
	})
}

// TestPortAudioStreamParameters tests various stream parameters
func TestPortAudioStreamParameters(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	backend := NewPortAudioBackend()
	err := backend.Initialize()
	if err != nil {
		t.Skipf("PortAudio initialization failed (may be expected): %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	paramTests := []struct {
		name       string
		sampleRate float64
		channels   int
		bufferSize int
	}{
		{"standard_voice", 16000, 1, 512},
		{"cd_quality_mono", 44100, 1, 1024},
		{"cd_quality_stereo", 44100, 2, 1024},
		{"high_res_mono", 48000, 1, 2048},
	}

	for _, tt := range paramTests {
		t.Run(tt.name, func(t *testing.T) {
			// Test input stream
			inputStream, err := backend.CreateInputStream(tt.sampleRate, tt.channels, tt.bufferSize)
			if err != nil {
				t.Skipf("CreateInputStream failed for %s (may be expected): %v", tt.name, err)
			} else {
				_ = inputStream.Close() // Ignore errors during test cleanup
			}

			// Test output stream
			outputStream, err := backend.CreateOutputStream(tt.sampleRate, tt.channels, tt.bufferSize)
			if err != nil {
				t.Skipf("CreateOutputStream failed for %s (may be expected): %v", tt.name, err)
			} else {
				_ = outputStream.Close() // Ignore errors during test cleanup
			}
		})
	}
}

// TestPortAudioBackendLifecycle tests full backend lifecycle
func TestPortAudioBackendLifecycle(t *testing.T) {
	if isCIEnvironment() {
		t.Skip("Skipping PortAudio tests in CI environment")
	}

	t.Run("full_lifecycle", func(t *testing.T) {
		backend := NewPortAudioBackend()

		// Initialize
		err := backend.Initialize()
		if err != nil {
			t.Skipf("PortAudio initialization failed (may be expected): %v", err)
		}

		// Create streams
		inputStream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			t.Logf("CreateInputStream failed (may be expected): %v", err)
		} else {
			_ = inputStream.Close() // Ignore errors during test cleanup
		}

		outputStream, err := backend.CreateOutputStream(16000, 1, 512)
		if err != nil {
			t.Logf("CreateOutputStream failed (may be expected): %v", err)
		} else {
			_ = outputStream.Close() // Ignore errors during test cleanup
		}

		// Terminate
		err = backend.Terminate()
		assert.NoError(t, err, "should terminate successfully")
	})

	t.Run("multiple_init_terminate_cycles", func(t *testing.T) {
		backend := NewPortAudioBackend()

		for i := 0; i < 3; i++ {
			err := backend.Initialize()
			if err != nil {
				t.Skipf("PortAudio initialization failed (may be expected): %v", err)
			}

			err = backend.Terminate()
			assert.NoError(t, err, "should terminate successfully in cycle %d", i+1)
		}
	})
}

// BenchmarkPortAudioOperations benchmarks PortAudio operations
func BenchmarkPortAudioOperations(b *testing.B) {
	if isCIEnvironment() {
		b.Skip("Skipping PortAudio benchmarks in CI environment")
	}

	backend := NewPortAudioBackend()
	err := backend.Initialize()
	if err != nil {
		b.Skipf("PortAudio initialization failed (may be expected): %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	b.Run("stream_creation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			stream, err := backend.CreateInputStream(16000, 1, 512)
			if err != nil {
				b.Skipf("Stream creation failed: %v", err)
			}
			_ = stream.Close() // Ignore errors during test cleanup
		}
	})

	b.Run("stream_start_stop", func(b *testing.B) {
		stream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			b.Skipf("Stream creation failed: %v", err)
		}
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			err := stream.Start()
			if err != nil {
				b.Skipf("Stream start failed: %v", err)
			}
			err = stream.Stop()
			if err != nil {
				b.Skipf("Stream stop failed: %v", err)
			}
		}
	})
}