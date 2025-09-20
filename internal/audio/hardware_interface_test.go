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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardwareInterfaceBasics tests basic hardware interface operations
func TestHardwareInterfaceBasics(t *testing.T) {
	t.Run("backend_lifecycle", func(t *testing.T) {
		backend := NewMockAudioBackend()

		// Test initialization
		err := backend.Initialize()
		require.NoError(t, err, "should initialize successfully")

		// Test termination
		err = backend.Terminate()
		require.NoError(t, err, "should terminate successfully")
	})

	t.Run("backend_initialization_error", func(t *testing.T) {
		backend := NewMockAudioBackend()
		expectedError := fmt.Errorf("hardware initialization failed")
		backend.SetInitError(expectedError)

		err := backend.Initialize()
		require.Error(t, err, "should fail initialization")
		assert.Contains(t, err.Error(), "hardware initialization failed")
	})

	t.Run("double_initialization", func(t *testing.T) {
		backend := NewMockAudioBackend()

		err := backend.Initialize()
		require.NoError(t, err)

		// Second initialization should be safe
		err = backend.Initialize()
		require.NoError(t, err, "double initialization should be safe")

		_ = backend.Terminate() // Ignore errors during test cleanup
	})
}

// TestInputStreamOperations tests audio input stream operations
func TestInputStreamOperations(t *testing.T) {
	t.Run("create_input_stream", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err, "should create input stream")
		require.NotNil(t, stream, "stream should not be nil")

		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup
	})

	t.Run("input_stream_lifecycle", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)

		// Test start
		err = stream.Start()
		require.NoError(t, err, "should start stream")
		assert.True(t, stream.IsActive(), "stream should be active")

		// Test stop
		err = stream.Stop()
		require.NoError(t, err, "should stop stream")

		// Test close
		err = stream.Close()
		require.NoError(t, err, "should close stream")
	})

	t.Run("input_stream_read_operations", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)

		// Test reading audio data
		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.NoError(t, err, "should read audio data")

		// Verify we got some data (mock generates sine wave)
		hasNonZero := false
		for _, sample := range buffer {
			if sample != 0.0 {
				hasNonZero = true
				break
			}
		}
		assert.True(t, hasNonZero, "should receive non-zero audio data")

		_ = stream.Stop() // Ignore errors during test cleanup
	})

	t.Run("input_stream_custom_generator", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		// Set custom audio data generator
		mockStream := stream.(*MockStream)
		testValue := float32(0.5)
		mockStream.SetAudioDataGenerator(func(data []float32) {
			for i := range data {
				data[i] = testValue
			}
		})

		err = stream.Start()
		require.NoError(t, err)

		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.NoError(t, err)

		// Verify all samples have our test value
		for i, sample := range buffer {
			assert.Equal(t, testValue, sample, "sample %d should have test value", i)
		}

		_ = stream.Stop() // Ignore errors during test cleanup
	})
}

// TestOutputStreamOperations tests audio output stream operations
func TestOutputStreamOperations(t *testing.T) {
	t.Run("create_output_stream", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateOutputStream(16000, 1, 512)
		require.NoError(t, err, "should create output stream")
		require.NotNil(t, stream, "stream should not be nil")

		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup
	})

	t.Run("output_stream_write_operations", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateOutputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)

		// Generate test audio data
		testData := make([]float32, 512)
		for i := range testData {
			testData[i] = float32(0.3 * math.Sin(2*math.Pi*440*float64(i)/16000))
		}

		// Write audio data
		err = stream.Write(testData)
		require.NoError(t, err, "should write audio data")

		// Verify data was recorded
		playbackData := backend.GetPlaybackAudioData()
		require.Len(t, playbackData, 1, "should have one recorded buffer")
		assert.Equal(t, len(testData), len(playbackData[0]), "buffer size should match")

		// Verify data content
		for i, expected := range testData {
			assert.Equal(t, expected, playbackData[0][i], "sample %d should match", i)
		}

		_ = stream.Stop() // Ignore errors during test cleanup
	})
}

// TestStreamErrorHandling tests error handling in stream operations
func TestStreamErrorHandling(t *testing.T) {
	t.Run("stream_creation_without_initialization", func(t *testing.T) {
		backend := NewMockAudioBackend()
		// Don't initialize

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.Error(t, err, "should fail without initialization")
		assert.Nil(t, stream, "stream should be nil on error")
	})

	t.Run("stream_creation_error", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		expectedError := fmt.Errorf("hardware stream creation failed")
		backend.SetCreateStreamError(expectedError)

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.Error(t, err, "should fail stream creation")
		assert.Nil(t, stream, "stream should be nil on error")
	})

	t.Run("stream_start_error", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		mockStream := stream.(*MockStream)
		expectedError := fmt.Errorf("hardware start failed")
		mockStream.SetStartError(expectedError)

		err = stream.Start()
		require.Error(t, err, "should fail to start")
		assert.Contains(t, err.Error(), "hardware start failed")
	})

	t.Run("write_to_input_stream", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)
		defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

		testData := make([]float32, 512)
		err = stream.Write(testData)
		require.Error(t, err, "should not allow writing to input stream")
		assert.Contains(t, err.Error(), "cannot write to input stream")
	})

	t.Run("read_from_output_stream", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateOutputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)
		defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.Error(t, err, "should not allow reading from output stream")
		assert.Contains(t, err.Error(), "cannot read from output stream")
	})
}

// TestConcurrentStreamOperations tests concurrent stream usage
func TestConcurrentStreamOperations(t *testing.T) {
	t.Run("multiple_input_streams", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		numStreams := 5
		streams := make([]StreamInterface, numStreams)

		// Create multiple input streams
		for i := 0; i < numStreams; i++ {
			stream, err := backend.CreateInputStream(16000, 1, 512)
			require.NoError(t, err, "should create input stream %d", i)
			streams[i] = stream
		}

		// Start all streams
		for i, stream := range streams {
			err := stream.Start()
			require.NoError(t, err, "should start stream %d", i)
		}

		// Stop and close all streams
		for i, stream := range streams {
			err := stream.Stop()
			require.NoError(t, err, "should stop stream %d", i)
			err = stream.Close()
			require.NoError(t, err, "should close stream %d", i)
		}
	})

	t.Run("concurrent_read_write", func(t *testing.T) {
		backend := NewMockAudioBackend()
		backend.SetSimulateRealTiming(false) // Speed up test
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		inputStream, err := backend.CreateInputStream(16000, 1, 256)
		require.NoError(t, err)
		defer func() { _ = inputStream.Close() }() // Ignore errors during test cleanup

		outputStream, err := backend.CreateOutputStream(16000, 1, 256)
		require.NoError(t, err)
		defer func() { _ = outputStream.Close() }() // Ignore errors during test cleanup

		err = inputStream.Start()
		require.NoError(t, err)
		err = outputStream.Start()
		require.NoError(t, err)

		numOperations := 10
		var wg sync.WaitGroup
		wg.Add(2)

		// Concurrent reading
		go func() {
			defer wg.Done()
			buffer := make([]float32, 256)
			for i := 0; i < numOperations; i++ {
				err := inputStream.Read(buffer)
				if err != nil {
					t.Errorf("read operation %d failed: %v", i, err)
					return
				}
			}
		}()

		// Concurrent writing
		go func() {
			defer wg.Done()
			buffer := make([]float32, 256)
			for i := range buffer {
				buffer[i] = float32(0.1)
			}
			for i := 0; i < numOperations; i++ {
				err := outputStream.Write(buffer)
				if err != nil {
					t.Errorf("write operation %d failed: %v", i, err)
					return
				}
			}
		}()

		wg.Wait()

		_ = inputStream.Stop() // Ignore errors during test cleanup
		_ = outputStream.Stop() // Ignore errors during test cleanup

		// Verify data was recorded
		recordedData := backend.GetRecordedAudioData()
		playbackData := backend.GetPlaybackAudioData()

		assert.GreaterOrEqual(t, len(recordedData), numOperations, "should have recorded data")
		assert.Equal(t, numOperations, len(playbackData), "should have playback data")
	})
}

// TestStreamParameters tests various stream parameter configurations
func TestStreamParameters(t *testing.T) {
	paramTests := []struct {
		name       string
		sampleRate float64
		channels   int
		bufferSize int
	}{
		{"CD_quality_mono", 44100, 1, 1024},
		{"CD_quality_stereo", 44100, 2, 1024},
		{"voice_quality", 16000, 1, 512},
		{"high_res_audio", 96000, 2, 2048},
		{"low_latency", 48000, 1, 128},
	}

	for _, tt := range paramTests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewMockAudioBackend()
			err := backend.Initialize()
			require.NoError(t, err)
			defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

			// Test input stream
			inputStream, err := backend.CreateInputStream(tt.sampleRate, tt.channels, tt.bufferSize)
			require.NoError(t, err, "should create input stream with parameters")
			_ = inputStream.Close() // Ignore errors during test cleanup

			// Test output stream
			outputStream, err := backend.CreateOutputStream(tt.sampleRate, tt.channels, tt.bufferSize)
			require.NoError(t, err, "should create output stream with parameters")
			_ = outputStream.Close() // Ignore errors during test cleanup
		})
	}
}

// TestHardwareResourceManagement tests resource management
func TestHardwareResourceManagement(t *testing.T) {
	t.Run("stream_cleanup_on_terminate", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)

		// Create multiple streams
		stream1, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		stream2, err := backend.CreateOutputStream(16000, 1, 512)
		require.NoError(t, err)

		_ = stream1.Start() // Ignore errors in concurrency test
		_ = stream2.Start() // Ignore errors in concurrency test

		// Terminate should clean up all streams
		err = backend.Terminate()
		require.NoError(t, err)

		// Streams should no longer be active
		assert.False(t, stream1.IsActive(), "input stream should not be active after terminate")
		assert.False(t, stream2.IsActive(), "output stream should not be active after terminate")
	})

	t.Run("memory_leak_prevention", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		// Create and close many streams to test for memory leaks
		for i := 0; i < 100; i++ {
			stream, err := backend.CreateInputStream(16000, 1, 512)
			require.NoError(t, err)

			err = stream.Start()
			require.NoError(t, err)

			err = stream.Stop()
			require.NoError(t, err)

			err = stream.Close()
			require.NoError(t, err)
		}

		// If we got here without hanging, memory management is working
	})
}

// TestAudioDataIntegrity tests audio data integrity through the pipeline
func TestAudioDataIntegrity(t *testing.T) {
	t.Run("data_roundtrip", func(t *testing.T) {
		backend := NewMockAudioBackend()
		backend.SetSimulateRealTiming(false)
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		// Generate test signal
		sampleRate := 16000.0
		bufferSize := 512
		frequency := 440.0 // A4 note

		testData := make([]float32, bufferSize)
		for i := range testData {
			t := float64(i) / sampleRate
			testData[i] = float32(0.5 * math.Sin(2*math.Pi*frequency*t))
		}

		// Create output stream and write data
		outputStream, err := backend.CreateOutputStream(sampleRate, 1, bufferSize)
		require.NoError(t, err)
		defer func() { _ = outputStream.Close() }() // Ignore errors during test cleanup

		err = outputStream.Start()
		require.NoError(t, err)

		err = outputStream.Write(testData)
		require.NoError(t, err)

		_ = outputStream.Stop() // Ignore errors during test cleanup

		// Verify data integrity
		playbackData := backend.GetPlaybackAudioData()
		require.Len(t, playbackData, 1)

		receivedData := playbackData[0]
		require.Equal(t, len(testData), len(receivedData))

		// Check each sample
		for i, expected := range testData {
			assert.Equal(t, expected, receivedData[i], "sample %d should match", i)
		}
	})

	t.Run("sample_format_validation", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)

		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.NoError(t, err)

		// Validate samples are in valid range [-1.0, 1.0]
		for i, sample := range buffer {
			assert.GreaterOrEqual(t, sample, float32(-1.0), "sample %d should be >= -1.0", i)
			assert.LessOrEqual(t, sample, float32(1.0), "sample %d should be <= 1.0", i)
		}

		_ = stream.Stop() // Ignore errors during test cleanup
	})
}

// TestStreamCallbackInterface tests callback functionality
func TestStreamCallbackInterface(t *testing.T) {
	t.Run("callback_assignment", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		callback := func(input, output []float32) error {
			return nil
		}

		err = stream.SetCallback(callback)
		require.NoError(t, err, "should set callback successfully")

		// Note: Mock implementation stores callback but doesn't use it
		// In real implementation, callback would be used during stream processing
	})
}

// TestPerformanceCharacteristics tests performance-related aspects
func TestPerformanceCharacteristics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Run("latency_measurement", func(t *testing.T) {
		backend := NewMockAudioBackend()
		backend.SetSimulateRealTiming(false) // Disable timing simulation for speed
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)
		defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

		// Measure read latency
		buffer := make([]float32, 512)
		startTime := time.Now()

		numReads := 100
		for i := 0; i < numReads; i++ {
			err := stream.Read(buffer)
			require.NoError(t, err)
		}

		totalTime := time.Since(startTime)
		avgLatency := totalTime / time.Duration(numReads)

		t.Logf("Average read latency: %v", avgLatency)

		// With timing simulation disabled, latency should be very low
		assert.Less(t, avgLatency, 1*time.Millisecond, "read latency should be low without timing simulation")
	})

	t.Run("throughput_measurement", func(t *testing.T) {
		backend := NewMockAudioBackend()
		backend.SetSimulateRealTiming(false)
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateOutputStream(16000, 1, 1024)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)
		defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

		buffer := make([]float32, 1024)
		numWrites := 1000

		startTime := time.Now()
		for i := 0; i < numWrites; i++ {
			err := stream.Write(buffer)
			require.NoError(t, err)
		}
		totalTime := time.Since(startTime)

		samplesPerSecond := float64(numWrites*1024) / totalTime.Seconds()
		t.Logf("Throughput: %.2f samples/second", samplesPerSecond)

		// Should achieve high throughput without timing simulation
		assert.Greater(t, samplesPerSecond, 1000000.0, "should achieve high throughput")
	})
}

// TestHardwareFailureScenarios tests various hardware failure scenarios
func TestHardwareFailureScenarios(t *testing.T) {
	t.Run("device_disconnection_simulation", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)

		// Simulate device disconnection by injecting write error
		mockStream := stream.(*MockStream)
		mockStream.SetWriteError(fmt.Errorf("device disconnected"))

		// Read should still work, but write should fail
		buffer := make([]float32, 512)
		err = stream.Read(buffer)
		require.NoError(t, err, "read should work before disconnection simulation")

		_ = stream.Stop() // Ignore errors during test cleanup
	})

	t.Run("buffer_underrun_handling", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		// Create very small buffer to simulate underrun conditions
		stream, err := backend.CreateOutputStream(16000, 1, 32)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		err = stream.Start()
		require.NoError(t, err)
		defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

		// Write should handle small buffers gracefully
		buffer := make([]float32, 32)
		err = stream.Write(buffer)
		require.NoError(t, err, "should handle small buffer writes")
	})
}

// TestStreamStateManagement tests stream state transitions
func TestStreamStateManagement(t *testing.T) {
	t.Run("state_transitions", func(t *testing.T) {
		backend := NewMockAudioBackend()
		err := backend.Initialize()
		require.NoError(t, err)
		defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

		stream, err := backend.CreateInputStream(16000, 1, 512)
		require.NoError(t, err)
		defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

		// Initial state: not active
		assert.False(t, stream.IsActive(), "stream should not be active initially")

		// Start stream
		err = stream.Start()
		require.NoError(t, err)
		assert.True(t, stream.IsActive(), "stream should be active after start")

		// Stop stream
		err = stream.Stop()
		require.NoError(t, err)
		assert.False(t, stream.IsActive(), "stream should not be active after stop")

		// Multiple stops should be safe
		err = stream.Stop()
		require.NoError(t, err, "multiple stops should be safe")

		// Restart should work
		err = stream.Start()
		require.NoError(t, err)
		assert.True(t, stream.IsActive(), "stream should be active after restart")

		_ = stream.Stop() // Ignore errors during test cleanup
	})
}

// Benchmark tests for hardware interface performance
func BenchmarkHardwareInterface(b *testing.B) {
	backend := NewMockAudioBackend()
	backend.SetSimulateRealTiming(false)
	err := backend.Initialize()
	if err != nil {
		b.Fatalf("Failed to initialize backend: %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	stream, err := backend.CreateInputStream(16000, 1, 512)
	if err != nil {
		b.Fatalf("Failed to create stream: %v", err)
	}
	defer func() { _ = stream.Close() }() // Ignore errors during test cleanup

	err = stream.Start()
	if err != nil {
		b.Fatalf("Failed to start stream: %v", err)
	}
	defer func() { _ = stream.Stop() }() // Ignore errors during test cleanup

	buffer := make([]float32, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := stream.Read(buffer)
		if err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}

func BenchmarkStreamCreation(b *testing.B) {
	backend := NewMockAudioBackend()
	err := backend.Initialize()
	if err != nil {
		b.Fatalf("Failed to initialize backend: %v", err)
	}
	defer func() { _ = backend.Terminate() }() // Ignore errors during test cleanup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := backend.CreateInputStream(16000, 1, 512)
		if err != nil {
			b.Fatalf("Stream creation failed: %v", err)
		}
		_ = stream.Close() // Ignore errors during benchmark cleanup
	}
}