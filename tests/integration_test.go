package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-puck-go/internal/audio"
	"github.com/loqalabs/loqa-puck-go/internal/nats"
	"github.com/loqalabs/loqa-puck-go/internal/transport"
)


// Integration test suite for end-to-end audio streaming workflow
func TestIntegration_AudioStreamingWorkflow(t *testing.T) {
	// Create mock hub server
	var receivedFrames []*transport.Frame
	var mu sync.Mutex

	// Create a mux to handle both endpoints
	mux := http.NewServeMux()

	// Handle streaming endpoint
	mux.HandleFunc("/stream/puck", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Session-ID", "test-session-workflow")
		w.WriteHeader(http.StatusOK)

		// Send mock response frame after a delay
		time.Sleep(100 * time.Millisecond)
		responseFrame := transport.NewFrame(
			transport.FrameTypeResponse,
			12345,
			1,
			timeToMicros(time.Now()),
			[]byte("integration test response"),
		)
		responseData, _ := responseFrame.Serialize()
		_, _ = w.Write(responseData)
	})

	// Handle frame sending endpoint
	mux.HandleFunc("/send/puck", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		if len(body) >= transport.HeaderSize {
			frame, err := transport.DeserializeFrame(body)
			if err == nil {
				mu.Lock()
				receivedFrames = append(receivedFrames, frame)
				mu.Unlock()
			}
		}

		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Create puck client
	client := transport.NewPuckClient(server.URL, "integration-test-puck")

	// Connect to server
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Create audio channel
	audioChan := make(chan audio.AudioChunk, 5)

	// Track responses
	var responses [][]byte
	var responseMu sync.Mutex
	responseChan := make(chan interface{}, 10)

	// Start goroutine to handle responses
	go func() {
		for response := range responseChan {
			if data, ok := response.([]byte); ok {
				responseMu.Lock()
				responses = append(responses, data)
				responseMu.Unlock()
			}
		}
	}()

	// Start streaming
	err = client.StreamAudio(audioChan, responseChan)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Send test audio chunks
	testChunks := []audio.AudioChunk{
		{
			Data:       []float32{0.1, 0.2, 0.3},
			Timestamp:  time.Now().UnixMicro(),
			IsWakeWord: false,
		},
		{
			Data:       []float32{0.4, 0.5, 0.6},
			Timestamp:  time.Now().UnixMicro(),
			IsWakeWord: true, // Wake word chunk
		},
		{
			Data:       []float32{0.7, 0.8, 0.9},
			Timestamp:  time.Now().UnixMicro(),
			IsWakeWord: false,
		},
	}

	for _, chunk := range testChunks {
		audioChan <- chunk
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)
	close(audioChan)

	// Verify frames were received
	mu.Lock()
	if len(receivedFrames) == 0 {
		t.Fatal("Expected to receive frames from client")
	}

	// Check for different frame types
	audioFrames := 0
	wakeWordFrames := 0

	for _, frame := range receivedFrames {
		switch frame.Type {
		case transport.FrameTypeAudioData:
			audioFrames++
		case transport.FrameTypeWakeWord:
			wakeWordFrames++
		}
	}
	mu.Unlock()

	if audioFrames == 0 {
		t.Error("Expected to receive audio data frames")
	}
	if wakeWordFrames == 0 {
		t.Error("Expected to receive wake word frames")
	}

	// Verify response was received
	responseMu.Lock()
	if len(responses) == 0 {
		t.Error("Expected to receive response from server")
	} else {
		responseStr := string(responses[0])
		if responseStr != "integration test response" {
			t.Errorf("Expected 'integration test response', got: %s", responseStr)
		}
	}
	responseMu.Unlock()
}

// Test binary frame protocol compatibility
func TestIntegration_BinaryFrameProtocol(t *testing.T) {
	testCases := []struct {
		name      string
		frameType transport.FrameType
		data      []byte
	}{
		{
			name:      "audio_data_frame",
			frameType: transport.FrameTypeAudioData,
			data:      []byte{0x00, 0x01, 0x02, 0x03},
		},
		{
			name:      "wake_word_frame",
			frameType: transport.FrameTypeWakeWord,
			data:      []byte("wake word data"),
		},
		{
			name:      "heartbeat_frame",
			frameType: transport.FrameTypeHeartbeat,
			data:      nil,
		},
		{
			name:      "large_frame",
			frameType: transport.FrameTypeAudioData,
			data:      make([]byte, transport.MaxDataSize), // Actual max frame size (1512 bytes)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create frame
			frame := transport.NewFrame(
				tc.frameType,
				12345,
				1,
				timeToMicros(time.Now()),
				tc.data,
			)

			// Serialize
			serialized, err := frame.Serialize()
			if err != nil {
				t.Fatalf("Failed to serialize frame: %v", err)
			}

			// Deserialize
			deserialized, err := transport.DeserializeFrame(serialized)
			if err != nil {
				t.Fatalf("Failed to deserialize frame: %v", err)
			}

			// Verify round-trip
			if deserialized.Type != tc.frameType {
				t.Errorf("Frame type mismatch: expected %d, got %d", tc.frameType, deserialized.Type)
			}

			if len(deserialized.Data) != len(tc.data) {
				t.Errorf("Data length mismatch: expected %d, got %d", len(tc.data), len(deserialized.Data))
			}

			for i, expected := range tc.data {
				if i < len(deserialized.Data) && deserialized.Data[i] != expected {
					t.Errorf("Data mismatch at index %d: expected 0x%02X, got 0x%02X", i, expected, deserialized.Data[i])
				}
			}
		})
	}
}

// Test HTTP streaming with concurrent operations
func TestIntegration_ConcurrentStreaming(t *testing.T) {
	var frameCount int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle both streaming and frame sending endpoints
		if r.URL.Path == "/send/puck" {
			// Handle individual frame sending
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)

			// Read the frame data
			buffer := make([]byte, 4096)
			n, err := r.Body.Read(buffer)
			if err != nil && n == 0 {
				return
			}

			// Look for frame magic bytes in the data
			for i := 0; i <= n-4; i++ {
				// Check for LOQA magic number (0x4C4F5141 in big-endian)
				if buffer[i] == 0x4C && buffer[i+1] == 0x4F && buffer[i+2] == 0x51 && buffer[i+3] == 0x41 {
					mu.Lock()
					frameCount++
					mu.Unlock()
					break // Only count one per request
				}
			}
		} else {
			// Handle streaming endpoint (for connection establishment)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	const numClients = 3
	const framesPerClient = 10

	var wg sync.WaitGroup

	// Create multiple concurrent clients
	for clientID := 0; clientID < numClients; clientID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			client := transport.NewPuckClient(server.URL, "concurrent-test-puck")
			err := client.Connect()
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", id, err)
				return
			}
			defer client.Disconnect()

			audioChan := make(chan audio.AudioChunk, framesPerClient)

			err = client.StreamAudio(audioChan, nil)
			if err != nil {
				t.Errorf("Client %d failed to start streaming: %v", id, err)
				return
			}

			// Send frames
			for i := 0; i < framesPerClient; i++ {
				chunk := audio.AudioChunk{
					Data:      []float32{float32(id), float32(i)},
					Timestamp: time.Now().UnixMicro(),
				}
				audioChan <- chunk
			}

			close(audioChan)
			time.Sleep(100 * time.Millisecond)
		}(clientID)
	}

	wg.Wait()

	// Verify all frames were received
	mu.Lock()
	expectedFrames := numClients * framesPerClient
	if frameCount < expectedFrames {
		t.Errorf("Expected at least %d frames, got %d", expectedFrames, frameCount)
	}
	mu.Unlock()
}

// Test error handling and recovery
func TestIntegration_ErrorHandling(t *testing.T) {
	testCases := []struct {
		name           string
		serverBehavior func(http.ResponseWriter, *http.Request)
		expectError    bool
	}{
		{
			name: "server_returns_500",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectError: true,
		},
		{
			name: "server_closes_connection",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				// Immediately close without sending data
			},
			expectError: false, // Should handle gracefully
		},
		{
			name: "server_sends_invalid_data",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusOK)
				// Send invalid frame data
				_, _ = w.Write([]byte("invalid frame data"))
			},
			expectError: false, // Should handle gracefully
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tc.serverBehavior))
			defer server.Close()

			client := transport.NewPuckClient(server.URL, "error-test-puck")

			err := client.Connect()
			if tc.expectError && err == nil {
				t.Error("Expected connection error")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected connection error: %v", err)
			}

			if err == nil {
				defer client.Disconnect()

				// Try to stream some data
				audioChan := make(chan audio.AudioChunk, 1)
				err = client.StreamAudio(audioChan, nil)
				if err != nil {
					t.Errorf("Failed to start streaming: %v", err)
				}

				// Send a test chunk
				chunk := audio.AudioChunk{
					Data:      []float32{0.1, 0.2},
					Timestamp: time.Now().UnixMicro(),
				}
				audioChan <- chunk
				close(audioChan)

				time.Sleep(100 * time.Millisecond)
			}
		})
	}
}

// Test NATS integration for audio playback
func TestIntegration_NATSAudioPlayback(t *testing.T) {
	// This test would require a mock NATS server
	// For now, we'll test the audio stream manager directly

	streamManager := nats.NewAudioStreamManager(5)
	_ = streamManager.GetPlaybackChannel() // Get channel for test setup

	// Test audio message - since QueueAudio doesn't exist, we'll test
	// the manager directly by simulating how NATS would use it
	_ = []byte("test audio data") // Test data for future implementation

	// Since AudioStreamManager doesn't expose QueueAudio, we'll test
	// that the playback channel works correctly by skipping this test
	// until a proper API is added
	t.Skip("AudioStreamManager.QueueAudio method not implemented - test requires API enhancement")

	// This is how it would work if QueueAudio existed:
	// streamManager.QueueAudio(testAudio)
	// Verify audio is queued for playback through channel
}

// Test ESP32 compatibility constraints
func TestIntegration_ESP32Compatibility(t *testing.T) {
	// Test frame size limits
	maxData := make([]byte, transport.MaxFrameSize-transport.HeaderSize)
	for i := range maxData {
		maxData[i] = byte(i % 256)
	}

	frame := transport.NewFrame(
		transport.FrameTypeAudioData,
		12345,
		1,
		timeToMicros(time.Now()),
		maxData,
	)

	// Should be able to serialize maximum size frame
	serialized, err := frame.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize max size frame: %v", err)
	}

	if len(serialized) > transport.MaxFrameSize {
		t.Errorf("Serialized frame size %d exceeds maximum %d", len(serialized), transport.MaxFrameSize)
	}

	// Test oversized frame rejection
	oversizedData := make([]byte, transport.MaxFrameSize)
	oversizedFrame := transport.NewFrame(
		transport.FrameTypeAudioData,
		12345,
		1,
		timeToMicros(time.Now()),
		oversizedData,
	)

	if oversizedFrame.IsValid() {
		t.Error("Oversized frame should not be valid")
	}
}

// Test memory usage and performance under load
func TestIntegration_PerformanceUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	var processedFrames int
	var mu sync.Mutex

	// Create a mux to handle both endpoints
	mux := http.NewServeMux()

	// Handle streaming endpoint
	mux.HandleFunc("/stream/puck", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Session-ID", "test-session-123")
		w.WriteHeader(http.StatusOK)

		// Send a test response frame to start streaming
		testFrame := transport.NewFrame(transport.FrameTypeResponse, 12345, 1, timeToMicros(time.Now()), []byte("test response"))
		frameData, _ := testFrame.Serialize()
		_, _ = w.Write(frameData)

		// Keep connection open for streaming
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			return
		}
	})

	// Handle frame sending endpoint
	mux.HandleFunc("/send/puck", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		if len(body) >= transport.HeaderSize {
			_, err := transport.DeserializeFrame(body)
			if err == nil {
				mu.Lock()
				processedFrames++
				mu.Unlock()
			}
		}

		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := transport.NewPuckClient(server.URL, "load-test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 100)

	err = client.StreamAudio(audioChan, nil)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Send high volume of audio chunks
	const numFrames = 1000
	const frameSize = 756 // Samples per frame (756 * 2 bytes = 1512 bytes, exactly at ESP32 limit)

	start := time.Now()

	go func() {
		defer close(audioChan)

		for i := 0; i < numFrames; i++ {
			chunk := audio.AudioChunk{
				Data:      make([]float32, frameSize),
				Timestamp: time.Now().UnixMicro(),
			}

			// Fill with test data
			for j := range chunk.Data {
				chunk.Data[j] = float32(j%100) / 100.0
			}

			audioChan <- chunk
		}
	}()

	// Wait for processing to complete
	time.Sleep(2 * time.Second)

	duration := time.Since(start)

	mu.Lock()
	finalCount := processedFrames
	mu.Unlock()

	if finalCount < numFrames/2 {
		t.Errorf("Expected to process at least %d frames, got %d", numFrames/2, finalCount)
	}

	framesPerSecond := float64(finalCount) / duration.Seconds()
	t.Logf("Processed %d frames in %v (%.2f frames/sec)", finalCount, duration, framesPerSecond)

	// Performance threshold (adjust based on requirements)
	if framesPerSecond < 100 {
		t.Errorf("Performance below threshold: %.2f frames/sec", framesPerSecond)
	}
}

// Benchmark end-to-end streaming latency
func BenchmarkIntegration_StreamingLatency(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)

		// Echo back received frames immediately
		buffer := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buffer)
			if err != nil {
				return
			}
			if n > 0 {
				_, _ = w.Write(buffer[:n])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}))
	defer server.Close()

	client := transport.NewPuckClient(server.URL, "benchmark-puck")
	err := client.Connect()
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 1)

	var responseLatencies []time.Duration
	var mu sync.Mutex
	responseChan := make(chan interface{}, 10)

	// Handle responses in goroutine
	go func() {
		for response := range responseChan {
			end := time.Now()
			// Extract timestamp from response (would need to be implemented)
			// For now, just record that we got a response
			_ = response // Use response to avoid unused variable error
			mu.Lock()
			responseLatencies = append(responseLatencies, time.Since(end))
			mu.Unlock()
		}
	}()

	err = client.StreamAudio(audioChan, responseChan)
	if err != nil {
		b.Fatalf("Failed to start streaming: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()

		chunk := audio.AudioChunk{
			Data:      []float32{0.1, 0.2, 0.3},
			Timestamp: start.UnixMicro(),
		}

		audioChan <- chunk

		// Measure time to send frame
		_ = time.Since(start)
	}

	close(audioChan)
}