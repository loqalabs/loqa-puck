package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-puck-go/internal/audio"
	"github.com/loqalabs/loqa-puck-go/internal/transport"
)

// timeToMicros safely converts time to microseconds
func timeToMicros(t time.Time) uint64 {
	micros := t.UnixNano() / 1000
	if micros < 0 {
		return 0
	}
	return uint64(micros)
}

// Error injection test suite for robustness and reliability testing

// Test network failure scenarios
func TestErrorInjection_NetworkFailures(t *testing.T) {
	testCases := []struct {
		name           string
		serverBehavior func(http.ResponseWriter, *http.Request)
		expectError    bool
		description    string
	}{
		{
			name: "immediate_connection_close",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				// Close connection immediately
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					_ = conn.Close() // Ignore errors during test error injection
				}
			},
			expectError: true,
			description: "Server closes connection immediately",
		},
		{
			name: "partial_response_then_close",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("partial")) // Ignore errors during test error injection
				// Connection will be closed when handler returns
			},
			expectError: false,
			description: "Server sends partial response then closes",
		},
		{
			name: "slow_response",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
			description: "Server responds slowly",
		},
		{
			name: "invalid_status_code",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			expectError: true,
			description: "Server returns HTTP 400",
		},
		{
			name: "chunked_then_error",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)

				// Send some valid data
				validFrame := transport.NewFrame(
					transport.FrameTypeHeartbeat,
					12345,
					1,
					timeToMicros(time.Now()),
					nil,
				)
				validData, _ := validFrame.Serialize()
				_, _ = w.Write(validData) // Ignore errors during test error injection

				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}

				// Then close abruptly
				time.Sleep(100 * time.Millisecond)
			},
			expectError: false,
			description: "Server sends valid data then closes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tc.serverBehavior))
			defer server.Close()

			client := transport.NewPuckClient(server.URL, "error-test-puck")

			err := client.Connect()

			if tc.expectError && err == nil {
				t.Errorf("Expected connection error for %s", tc.description)
			} else if !tc.expectError && err != nil {
				// Some errors are acceptable for error injection tests
				t.Logf("Connection resulted in error (may be expected): %v", err)
			}

			if err == nil {
				defer client.Disconnect()
			}
		})
	}
}

// Test malformed frame handling
func TestErrorInjection_MalformedFrames(t *testing.T) {
	testCases := []struct {
		name         string
		frameData    []byte
		expectError  bool
		description  string
	}{
		{
			name:        "empty_frame",
			frameData:   []byte{},
			expectError: true,
			description: "Empty frame data",
		},
		{
			name:        "partial_header",
			frameData:   []byte{0x4C, 0x4F, 0x51, 0x41}, // Just magic number
			expectError: true,
			description: "Incomplete frame header",
		},
		{
			name:        "invalid_magic",
			frameData:   append([]byte{0x42, 0x41, 0x44, 0x21}, make([]byte, transport.HeaderSize-4)...),
			expectError: true,
			description: "Invalid magic number",
		},
		{
			name:        "length_mismatch",
			frameData:   createMalformedFrame(1000, 10), // Claims 1000 bytes but only has 10
			expectError: true,
			description: "Frame length mismatch",
		},
		{
			name:        "oversized_frame",
			frameData:   createOversizedFrame(),
			expectError: true,
			description: "Frame exceeds maximum size",
		},
		{
			name:        "invalid_frame_type",
			frameData:   createFrameWithType(0xFF), // Invalid frame type
			expectError: false, // Should handle gracefully
			description: "Unknown frame type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := transport.DeserializeFrame(tc.frameData)

			if tc.expectError && err == nil {
				t.Errorf("Expected deserialization error for %s", tc.description)
			} else if !tc.expectError && err != nil {
				t.Logf("Deserialization error (may be expected): %v", err)
			}
		})
	}
}

// Test concurrent access under error conditions
func TestErrorInjection_ConcurrentErrors(t *testing.T) {
	const numGoroutines = 10
	const operationsPerGoroutine = 20

	// Create a server that randomly fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Randomly succeed or fail
		if time.Now().UnixNano()%3 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)

		// Sometimes close early
		if time.Now().UnixNano()%4 == 0 {
			return
		}

		// Send some data
		time.Sleep(time.Duration(time.Now().UnixNano()%10) * time.Millisecond)
	}))
	defer server.Close()

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				client := transport.NewPuckClient(server.URL, fmt.Sprintf("concurrent-test-%d", goroutineID))

				err := client.Connect()
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, op %d: connect failed: %v", goroutineID, j, err)
					continue
				}

				// Try to send some data
				audioChan := make(chan audio.AudioChunk, 1)
				err = client.StreamAudio(audioChan, nil)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d, op %d: stream failed: %v", goroutineID, j, err)
					client.Disconnect()
					continue
				}

				chunk := audio.AudioChunk{
					Data:      []float32{float32(goroutineID), float32(j)},
					Timestamp: time.Now().UnixMicro(),
				}

				select {
				case audioChan <- chunk:
				case <-time.After(100 * time.Millisecond):
					errors <- fmt.Errorf("goroutine %d, op %d: channel send timeout", goroutineID, j)
				}

				close(audioChan)
				client.Disconnect()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Count errors (some are expected due to random failures)
	errorCount := 0
	for err := range errors {
		errorCount++
		t.Logf("Concurrent error: %v", err)
	}

	totalOperations := numGoroutines * operationsPerGoroutine
	errorRate := float64(errorCount) / float64(totalOperations)

	t.Logf("Error injection results: %d errors out of %d operations (%.2f%% error rate)",
		errorCount, totalOperations, errorRate*100)

	// Error rate should be reasonable but not 100% (some should succeed)
	if errorRate > 0.8 {
		t.Errorf("Error rate too high: %.2f%% (system may be completely broken)", errorRate*100)
	}
}

// Test memory exhaustion simulation
func TestErrorInjection_MemoryExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory exhaustion test in short mode")
	}

	// Simulate memory pressure by creating many large frames
	const maxFrames = 1000
	const frameSize = transport.MaxDataSize // Use actual maximum size (1512 bytes)

	frames := make([]*transport.Frame, 0, maxFrames)

	for i := 0; i < maxFrames; i++ {
		data := make([]byte, frameSize)
		for j := range data {
			data[j] = byte(j % 256)
		}

		frame := transport.NewFrame(
			transport.FrameTypeAudioData,
			12345,
			uint32(i), //nolint:gosec // G115: Safe conversion in test context
			timeToMicros(time.Now()),
			data,
		)

		if !frame.IsValid() {
			t.Errorf("Frame %d became invalid under memory pressure", i)
			break
		}

		serialized, err := frame.Serialize()
		if err != nil {
			t.Errorf("Frame %d serialization failed under memory pressure: %v", i, err)
			break
		}

		// Verify we can still deserialize
		_, err = transport.DeserializeFrame(serialized)
		if err != nil {
			t.Errorf("Frame %d deserialization failed under memory pressure: %v", i, err)
			break
		}

		frames = append(frames, frame)

		// Periodically check if we should stop (before actual memory exhaustion)
		if i%100 == 0 {
			t.Logf("Created %d frames so far", i+1)
		}
	}

	t.Logf("Successfully created %d frames under memory pressure", len(frames))

	// Verify frames are still valid
	for i, frame := range frames {
		if !frame.IsValid() {
			t.Errorf("Frame %d became invalid after memory pressure test", i)
			break
		}
	}
}

// Test timeout scenarios
func TestErrorInjection_TimeoutScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		delay       time.Duration
		timeout     time.Duration
		expectError bool
	}{
		{
			name:        "normal_response",
			delay:       100 * time.Millisecond,
			timeout:     1 * time.Second,
			expectError: false,
		},
		{
			name:        "slow_response",
			delay:       500 * time.Millisecond,
			timeout:     1 * time.Second,
			expectError: false,
		},
		{
			name:        "timeout_response",
			delay:       2 * time.Second,
			timeout:     1 * time.Second,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(tc.delay)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := transport.NewPuckClient(server.URL, "timeout-test-puck")

			// Create a context with timeout
			done := make(chan error, 1)
			go func() {
				done <- client.Connect()
			}()

			select {
			case err := <-done:
				if tc.expectError && err == nil {
					t.Errorf("Expected timeout error for delay %v", tc.delay)
				} else if !tc.expectError && err != nil {
					t.Errorf("Unexpected error for delay %v: %v", tc.delay, err)
				}

				if err == nil {
					client.Disconnect()
				}

			case <-time.After(tc.timeout):
				if !tc.expectError {
					t.Errorf("Unexpected timeout for delay %v", tc.delay)
				}
			}
		})
	}
}

// Test resource leak detection
func TestErrorInjection_ResourceLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping resource leak test in short mode")
	}

	const numIterations = 50 // Reduce iterations to avoid server overhead

	// Track goroutines before test
	runtime.GC() // Clean initial state
	time.Sleep(100 * time.Millisecond)
	initialGoroutines := countGoroutines()

	for i := 0; i < numIterations; i++ {
		// Create server that causes errors
		iteration := i // Capture for closure
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if iteration%2 == 0 {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.WriteHeader(http.StatusOK)
				time.Sleep(10 * time.Millisecond)
			}
		}))

		client := transport.NewPuckClient(server.URL, fmt.Sprintf("leak-test-%d", i))

		// Try operations that might leak resources
		err := client.Connect()
		if err == nil {
			audioChan := make(chan audio.AudioChunk, 1)
			_ = client.StreamAudio(audioChan, nil) // Ignore errors during concurrent test

			chunk := audio.AudioChunk{
				Data:      []float32{0.1, 0.2},
				Timestamp: time.Now().UnixMicro(),
			}
			audioChan <- chunk
			close(audioChan)

			client.Disconnect()
		}

		server.Close()
		server.CloseClientConnections()

		// Allow more time for server cleanup after every few iterations
		if i%10 == 9 {
			time.Sleep(100 * time.Millisecond)
			runtime.GC()
		}
	}

	// Give generous time for all HTTP servers to clean up
	time.Sleep(2 * time.Second) // Longer cleanup time for all servers
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
	runtime.GC() // Force another GC cycle
	time.Sleep(500 * time.Millisecond)

	// Final check for leaks - httptest servers can leave background goroutines
	finalGoroutines := countGoroutines()
	leakThreshold := 25 // Very generous threshold for httptest server cleanup
	if finalGoroutines > initialGoroutines+leakThreshold {
		t.Errorf("Significant goroutine leak detected: initial=%d, final=%d (threshold: +%d)",
			initialGoroutines, finalGoroutines, leakThreshold)

		// Debug: Print current goroutines to help identify the leak
		t.Logf("Current goroutines after cleanup:")
		_ = pprof.Lookup("goroutine").WriteTo(os.Stdout, 1) // Ignore errors in debug output
	}
}

// Test error recovery mechanisms
func TestErrorInjection_ErrorRecovery(t *testing.T) {
	failureCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failureCount++

		// Fail first few requests, then succeed
		if failureCount <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := transport.NewPuckClient(server.URL, "recovery-test-puck")

	// Try multiple times to test recovery
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		err := client.Connect()
		lastErr = err

		if err == nil {
			// Connection succeeded
			t.Logf("Connection succeeded on attempt %d", attempt)
			client.Disconnect()
			break
		}

		t.Logf("Attempt %d failed: %v", attempt, err)
		time.Sleep(100 * time.Millisecond)
	}

	// Should eventually succeed
	if lastErr != nil {
		t.Errorf("All connection attempts failed, no recovery demonstrated")
	}
}

// Helper functions for error injection tests

func createMalformedFrame(claimedLength int, actualDataLength int) []byte {
	header := make([]byte, transport.HeaderSize)
	// Set magic number
	copy(header[0:4], []byte("LOQA"))
	// Set frame type
	header[4] = byte(transport.FrameTypeAudioData)
	// Set incorrect length
	header[6] = byte(claimedLength & 0xFF)
	header[7] = byte((claimedLength >> 8) & 0xFF)
	// Add minimal actual data
	data := make([]byte, actualDataLength)
	return append(header, data...)
}

func createOversizedFrame() []byte {
	// Create frame that exceeds maximum size
	oversizedData := make([]byte, transport.MaxFrameSize+1000)
	frame := transport.NewFrame(
		transport.FrameTypeAudioData,
		12345,
		1,
		timeToMicros(time.Now()),
		oversizedData,
	)

	// This should fail validation
	serialized, _ := frame.Serialize()
	return serialized
}

func createFrameWithType(frameType byte) []byte {
	header := make([]byte, transport.HeaderSize)
	copy(header[0:4], []byte("LOQA"))
	header[4] = frameType // Invalid frame type
	header[5] = 0         // Reserved
	header[6] = 4         // Length (4 bytes)
	header[7] = 0

	data := []byte{0x01, 0x02, 0x03, 0x04}
	return append(header, data...)
}

func countGoroutines() int {
	return runtime.NumGoroutine()
}

// Benchmark error handling performance
func BenchmarkErrorInjection_ErrorHandlingPerformance(b *testing.B) {
	// Test performance impact of error handling
	validData := make([]byte, 1024)
	invalidData := []byte{0x42, 0x41, 0x44} // Invalid frame

	b.Run("valid_frame_processing", func(b *testing.B) {
		frame := transport.NewFrame(
			transport.FrameTypeAudioData,
			12345,
			1,
			timeToMicros(time.Now()),
			validData,
		)
		serialized, _ := frame.Serialize()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := transport.DeserializeFrame(serialized)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("invalid_frame_handling", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := transport.DeserializeFrame(invalidData)
			if err == nil {
				b.Fatal("Expected error for invalid frame")
			}
		}
	})
}