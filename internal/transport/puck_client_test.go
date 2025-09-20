package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-puck-go/internal/audio"
)

const (
	streamPuckPath = "/stream/puck"
	sendPuckPath   = "/send/puck"
)


// Mock HTTP server for puck client testing
func createMockPuckServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path
		switch r.URL.Path {
		case streamPuckPath:
			// Handle streaming connection
			handlePuckStreamConnection(t, w, r)
		case sendPuckPath:
			// Handle frame sending
			handlePuckFrameSend(t, w, r)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func handlePuckStreamConnection(t *testing.T, w http.ResponseWriter, r *http.Request) {
	// Validate puck_id parameter
	puckID := r.URL.Query().Get("puck_id")
	if puckID == "" {
		t.Error("Expected puck_id query parameter")
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)

	// Send a mock response frame
	responseFrame := NewFrame(FrameTypeResponse, 12345, 1, timeToMicros(time.Now()), []byte("mock response"))
	responseData, err := responseFrame.Serialize()
	if err != nil {
		t.Errorf("Failed to serialize response frame: %v", err)
		return
	}

	if _, err := w.Write(responseData); err != nil {
		t.Errorf("Failed to write response frame: %v", err)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Keep connection alive for testing
	time.Sleep(100 * time.Millisecond)
}

func handlePuckFrameSend(t *testing.T, w http.ResponseWriter, r *http.Request) {
	// Validate puck_id parameter
	puckID := r.URL.Query().Get("puck_id")
	if puckID == "" {
		t.Error("Expected puck_id query parameter")
	}

	// Read and validate the frame data (optional - for debugging)
	frameData, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("Failed to read frame data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Just log that we received the frame
	t.Logf("Received frame via /send/puck (%d bytes)", len(frameData))

	// Return success
	w.WriteHeader(http.StatusOK)
}

func TestNewPuckClient(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	if client == nil {
		t.Fatal("Expected non-nil puck client")
	}
	if client.hubAddress != "http://localhost:3000" {
		t.Errorf("Expected hubAddress: http://localhost:3000, got: %s", client.hubAddress)
	}
	if client.puckID != "test-puck" {
		t.Errorf("Expected puckID: test-puck, got: %s", client.puckID)
	}
	if client.isConnected {
		t.Error("Expected isConnected to be false initially")
	}
	if client.streamingClient == nil {
		t.Error("Expected non-nil streaming client")
	}
}

func TestPuckClient_Connect_Success(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")

	err := client.Connect()
	if err != nil {
		t.Fatalf("Expected successful connection, got error: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected")
	}

	defer client.Disconnect()
}

func TestPuckClient_SendHandshake(t *testing.T) {
	var receivedFrames [][]byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path
		switch r.URL.Path {
		case streamPuckPath:
			// Handle streaming connection - just return OK
			w.WriteHeader(http.StatusOK)
		case sendPuckPath:
			// Handle frame sending - capture handshake frame
			frameData, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("Failed to read frame data: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			mu.Lock()
			receivedFrames = append(receivedFrames, append([]byte(nil), frameData...))
			mu.Unlock()

			t.Logf("Received handshake frame via /send/puck (%d bytes)", len(frameData))
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Send handshake frame
	err = client.SendHandshake()
	if err != nil {
		t.Fatalf("Failed to send handshake: %v", err)
	}

	// Give server time to receive the frame
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedFrames) == 0 {
		t.Fatal("Expected to receive handshake frame")
	}

	// Verify the frame was properly serialized as a handshake frame
	frame, err := DeserializeFrame(receivedFrames[0])
	if err != nil {
		t.Fatalf("Failed to deserialize received frame: %v", err)
	}

	if frame.Type != FrameTypeHandshake {
		t.Errorf("Expected frame type %d (handshake), got %d", FrameTypeHandshake, frame.Type)
	}

	// Verify handshake data contains session and puck information
	handshakeData := string(frame.Data)
	if !strings.Contains(handshakeData, "session:") {
		t.Errorf("Expected handshake data to contain session info, got: %s", handshakeData)
	}
	if !strings.Contains(handshakeData, "puck:test-puck") {
		t.Errorf("Expected handshake data to contain puck ID, got: %s", handshakeData)
	}
}

func TestPuckClient_SendHandshake_NotConnected(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	err := client.SendHandshake()
	if err == nil {
		t.Fatal("Expected error when sending handshake without connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}
}

func TestPuckClient_Connect_ServerDown(t *testing.T) {
	// Try to connect to non-existent server
	client := NewPuckClient("http://localhost:9999", "test-puck")

	err := client.Connect()
	if err == nil {
		t.Fatal("Expected connection error when server is down")
	}

	if client.IsConnected() {
		t.Error("Expected client to not be connected")
	}
}

func TestPuckClient_StreamAudio_Basic(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Create audio channel
	audioChan := make(chan audio.AudioChunk, 5)

	// Start streaming
	err = client.StreamAudio(audioChan, nil)
	if err != nil {
		t.Fatalf("Failed to start audio streaming: %v", err)
	}

	// Send test audio chunk
	testChunk := audio.AudioChunk{
		Data:       []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Timestamp:  time.Now().UnixMicro(),
		IsWakeWord: false,
	}

	audioChan <- testChunk
	close(audioChan)

	// Give time for processing
	time.Sleep(200 * time.Millisecond)
}

func TestPuckClient_StreamAudio_WakeWord(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 5)

	err = client.StreamAudio(audioChan, nil)
	if err != nil {
		t.Fatalf("Failed to start audio streaming: %v", err)
	}

	// Send wake word chunk
	wakeWordChunk := audio.AudioChunk{
		Data:       []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		Timestamp:  time.Now().UnixMicro(),
		IsWakeWord: true,
	}

	audioChan <- wakeWordChunk
	close(audioChan)

	time.Sleep(200 * time.Millisecond)
}

func TestPuckClient_StreamAudio_NotConnected(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	audioChan := make(chan audio.AudioChunk, 1)
	err := client.StreamAudio(audioChan, nil)

	if err == nil {
		t.Fatal("Expected error when streaming audio without connection")
	}
}

func TestPuckClient_StreamAudio_WithResponseHandler(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 1)
	responseChan := make(chan []byte, 1)

	var receivedResponses [][]byte
	var mu sync.Mutex

	responseStreamChan := make(chan interface{}, 10)

	// Handle responses in a goroutine
	go func() {
		for response := range responseStreamChan {
			if data, ok := response.([]byte); ok {
				mu.Lock()
				receivedResponses = append(receivedResponses, data)
				responseChan <- data
				mu.Unlock()
			}
		}
	}()

	err = client.StreamAudio(audioChan, responseStreamChan)
	if err != nil {
		t.Fatalf("Failed to start audio streaming: %v", err)
	}

	// Send audio chunk
	testChunk := audio.AudioChunk{
		Data:      []float32{0.1, 0.2},
		Timestamp: time.Now().UnixMicro(),
	}
	audioChan <- testChunk

	// Wait for response
	select {
	case response := <-responseChan:
		if string(response) != "mock response" {
			t.Errorf("Expected 'mock response', got: %s", response)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for response")
	}

	close(audioChan)
}

func TestPuckClient_StreamAudio_HighVolume(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 100)

	err = client.StreamAudio(audioChan, nil)
	if err != nil {
		t.Fatalf("Failed to start audio streaming: %v", err)
	}

	// Send many audio chunks
	const numChunks = 50
	for i := 0; i < numChunks; i++ {
		chunk := audio.AudioChunk{
			Data:      []float32{float32(i) * 0.01, float32(i) * 0.02},
			Timestamp: time.Now().UnixMicro(),
		}
		audioChan <- chunk
	}

	close(audioChan)
	time.Sleep(500 * time.Millisecond)
}

func TestPuckClient_ConvertAudioChunkToBytes(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	testCases := []struct {
		name     string
		chunk    audio.AudioChunk
		expected []byte
	}{
		{
			name: "simple_chunk",
			chunk: audio.AudioChunk{
				Data: []float32{0.5, -0.5, 1.0, -1.0},
			},
			expected: []byte{0x00, 0x40, 0x00, 0xC0, 0xFF, 0x7F, 0x01, 0x80},
		},
		{
			name: "zero_chunk",
			chunk: audio.AudioChunk{
				Data: []float32{0.0, 0.0},
			},
			expected: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "empty_chunk",
			chunk: audio.AudioChunk{
				Data: []float32{},
			},
			expected: []byte{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := client.convertAudioChunkToBytes(tc.chunk)

			if len(result) != len(tc.expected) {
				t.Errorf("Expected length %d, got %d", len(tc.expected), len(result))
				return
			}

			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("At index %d: expected 0x%02X, got 0x%02X", i, expected, result[i])
				}
			}
		})
	}
}

func TestPuckClient_HandleIncomingFrame(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	var handledData [][]byte
	var mu sync.Mutex

	testHandlerChan := make(chan interface{}, 10)

	// Handle responses in a goroutine
	go func() {
		for data := range testHandlerChan {
			if bytes, ok := data.([]byte); ok {
				mu.Lock()
				handledData = append(handledData, bytes)
				mu.Unlock()
			}
		}
	}()

	testCases := []struct {
		name      string
		frameType FrameType
		data      []byte
		expectCall bool
	}{
		{
			name:      "response_frame",
			frameType: FrameTypeResponse,
			data:      []byte("response data"),
			expectCall: true,
		},
		{
			name:      "status_frame",
			frameType: FrameTypeStatus,
			data:      []byte("status data"),
			expectCall: true,
		},
		{
			name:      "heartbeat_frame",
			frameType: FrameTypeHeartbeat,
			data:      []byte{},
			expectCall: false,
		},
		{
			name:      "error_frame",
			frameType: FrameTypeError,
			data:      []byte("error message"),
			expectCall: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			handledData = nil // Reset
			mu.Unlock()

			frame := NewFrame(tc.frameType, 12345, 1, timeToMicros(time.Now()), tc.data)

			err := client.handleIncomingFrame(frame, testHandlerChan)
			if err != nil {
				t.Errorf("Unexpected error handling frame: %v", err)
			}

			// Give goroutine time to process the channel data
			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			defer mu.Unlock()

			if tc.expectCall {
				if len(handledData) == 0 {
					t.Error("Expected handler to be called")
				} else if string(handledData[0]) != string(tc.data) {
					t.Errorf("Expected data %s, got %s", tc.data, handledData[0])
				}
			} else {
				if len(handledData) > 0 {
					t.Error("Expected handler not to be called")
				}
			}
		})
	}
}

func TestPuckClient_HandleIncomingFrame_NilHandler(t *testing.T) {
	client := NewPuckClient("http://localhost:3000", "test-puck")

	frame := NewFrame(FrameTypeResponse, 12345, 1, timeToMicros(time.Now()), []byte("test"))

	// Should not panic with nil handler
	err := client.handleIncomingFrame(frame, nil)
	if err != nil {
		t.Errorf("Unexpected error with nil handler: %v", err)
	}
}

func TestPuckClient_Disconnect(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected before disconnect")
	}

	client.Disconnect()

	if client.IsConnected() {
		t.Error("Expected client to be disconnected after disconnect")
	}

	// Multiple disconnects should be safe
	client.Disconnect()
}

func TestPuckClient_IsConnected(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")

	// Initially not connected
	if client.IsConnected() {
		t.Error("Expected client to not be connected initially")
	}

	// Connect
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected after Connect()")
	}

	// Disconnect
	client.Disconnect()

	if client.IsConnected() {
		t.Error("Expected client to not be connected after Disconnect()")
	}
}

func TestPuckClient_ConcurrentOperations(t *testing.T) {
	server := createMockPuckServer(t)
	defer server.Close()

	client := NewPuckClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	audioChan := make(chan audio.AudioChunk, 10)

	err = client.StreamAudio(audioChan, nil)
	if err != nil {
		t.Fatalf("Failed to start audio streaming: %v", err)
	}

	const numGoroutines = 5
	const chunksPerGoroutine = 10

	var wg sync.WaitGroup

	// Send audio chunks concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < chunksPerGoroutine; j++ {
				chunk := audio.AudioChunk{
					Data:      []float32{float32(goroutineID), float32(j)},
					Timestamp: time.Now().UnixMicro(),
				}

				select {
				case audioChan <- chunk:
				case <-time.After(1 * time.Second):
					t.Errorf("Timeout sending chunk in goroutine %d", goroutineID)
					return
				}
			}
		}(i)
	}

	// Also test concurrent connection status checks
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if !client.IsConnected() {
					t.Error("Expected client to remain connected during concurrent operations")
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	close(audioChan)
	time.Sleep(200 * time.Millisecond)
}