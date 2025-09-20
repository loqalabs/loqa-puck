package transport

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// timeToMicros safely converts time to microseconds
func timeToMicros(t time.Time) uint64 {
	micros := t.UnixNano() / 1000
	if micros < 0 {
		return 0
	}
	return uint64(micros)
}

// Mock HTTP server for testing HTTP streaming
func createMockStreamingServer(t *testing.T, responseFrames [][]byte, simulateError bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path
		switch r.URL.Path {
		case streamPuckPath:
			// Handle streaming connection
			handleStreamConnection(t, w, r, responseFrames, simulateError)
		case sendPuckPath:
			// Handle frame sending
			handleFrameSend(t, w, r, simulateError)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func handleStreamConnection(t *testing.T, w http.ResponseWriter, r *http.Request, responseFrames [][]byte, simulateError bool) {
	// Validate request headers
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("Expected Content-Type: application/octet-stream, got: %s", r.Header.Get("Content-Type"))
	}

	// Validate query parameters
	puckID := r.URL.Query().Get("puck_id")
	if puckID == "" {
		t.Error("Expected puck_id query parameter")
	}

	if simulateError {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Set up streaming response headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// Get flusher to immediately send response headers
	flusher, hasFlusher := w.(http.Flusher)
	if !hasFlusher {
		t.Error("ResponseWriter doesn't support flushing")
		return
	}

	// Flush headers immediately to establish the connection
	flusher.Flush()

	// Send response frames immediately if any
	if len(responseFrames) > 0 {
		for i, frameData := range responseFrames {
			if _, err := w.Write(frameData); err != nil {
				t.Errorf("Failed to write response frame %d: %v", i, err)
				return
			}
			flusher.Flush()
			t.Logf("Sent response frame %d (%d bytes)", i, len(frameData))

			// Small delay between frames
			if i < len(responseFrames)-1 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
}

func handleFrameSend(t *testing.T, w http.ResponseWriter, r *http.Request, simulateError bool) {
	// Validate query parameters
	puckID := r.URL.Query().Get("puck_id")
	if puckID == "" {
		t.Error("Expected puck_id query parameter")
	}

	if simulateError {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Read the frame data
	frameData, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("Failed to read frame data: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	t.Logf("Received frame via /send/puck (%d bytes)", len(frameData))

	// Return success
	w.WriteHeader(http.StatusOK)
}

func TestNewHTTPStreamingClient(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.hubURL != "http://localhost:3000" {
		t.Errorf("Expected hubURL: http://localhost:3000, got: %s", client.hubURL)
	}
	if client.puckID != "test-puck" {
		t.Errorf("Expected puckID: test-puck, got: %s", client.puckID)
	}
	if client.isConnected {
		t.Error("Expected isConnected to be false initially")
	}
	if client.sequence != 0 {
		t.Errorf("Expected sequence to be 0 initially, got: %d", client.sequence)
	}
}

func TestHTTPStreamingClient_Connect_Success(t *testing.T) {
	// Create mock response frames
	responseFrame := NewFrame(FrameTypeHeartbeat, 12345, 1, timeToMicros(time.Now()), nil)
	responseData, err := responseFrame.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize response frame: %v", err)
	}

	server := createMockStreamingServer(t, [][]byte{responseData}, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	client.SetConnectTimeout(5 * time.Second) // Allow enough time for connection

	err = client.Connect()
	if err != nil {
		t.Fatalf("Expected successful connection, got error: %v", err)
	}

	if !client.IsConnected() {
		t.Error("Expected client to be connected")
	}

	defer client.Disconnect()
}

func TestHTTPStreamingClient_SendHandshake(t *testing.T) {
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

			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
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

func TestHTTPStreamingClient_SendHandshake_NotConnected(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	err := client.SendHandshake()
	if err == nil {
		t.Fatal("Expected error when sending handshake without connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}
}

func TestHTTPStreamingClient_Connect_ServerError(t *testing.T) {
	server := createMockStreamingServer(t, nil, true) // Simulate server error
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")

	err := client.Connect()
	if err == nil {
		t.Fatal("Expected connection error due to server error")
	}

	if client.IsConnected() {
		t.Error("Expected client to not be connected")
	}
}

func TestHTTPStreamingClient_Connect_InvalidURL(t *testing.T) {
	client := NewHTTPStreamingClient("invalid-url", "test-puck")

	err := client.Connect()
	if err == nil {
		t.Fatal("Expected connection error due to invalid URL")
	}

	if client.IsConnected() {
		t.Error("Expected client to not be connected")
	}
}

func TestHTTPStreamingClient_Connect_Timeout(t *testing.T) {
	// Create a server that doesn't respond quickly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second) // Longer than reduced client timeout
	}))
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")

	// Set a shorter timeout for this test to avoid long waits
	client.SetConnectTimeout(2 * time.Second)

	start := time.Now()
	err := client.Connect()
	duration := time.Since(start)

	if err == nil {
		t.Fatal("Expected connection timeout error")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	// Should timeout around 2 seconds (with some tolerance)
	if duration > 4*time.Second {
		t.Errorf("Connection took too long: %v", duration)
	}

	if client.IsConnected() {
		t.Error("Expected client to not be connected after timeout")
	}
}

func TestHTTPStreamingClient_SendFrame(t *testing.T) {
	var receivedFrames [][]byte
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path
		switch r.URL.Path {
		case streamPuckPath:
			// Handle streaming connection - just return OK
			w.WriteHeader(http.StatusOK)
		case sendPuckPath:
			// Handle frame sending - this is what we want to test
			frameData, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("Failed to read frame data: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			mu.Lock()
			receivedFrames = append(receivedFrames, append([]byte(nil), frameData...))
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	// Send test frame
	testData := []byte("test audio data")
	err = client.SendFrame(FrameTypeAudioData, testData)
	if err != nil {
		t.Fatalf("Failed to send frame: %v", err)
	}

	// Give server time to receive the frame
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedFrames) == 0 {
		t.Fatal("Expected to receive at least one frame")
	}

	// Verify the frame was properly serialized
	frame, err := DeserializeFrame(receivedFrames[0])
	if err != nil {
		t.Fatalf("Failed to deserialize received frame: %v", err)
	}

	if frame.Type != FrameTypeAudioData {
		t.Errorf("Expected frame type %d, got %d", FrameTypeAudioData, frame.Type)
	}
	if string(frame.Data) != string(testData) {
		t.Errorf("Expected frame data %s, got %s", testData, frame.Data)
	}
}

func TestHTTPStreamingClient_SendFrame_NotConnected(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	err := client.SendFrame(FrameTypeAudioData, []byte("test"))
	if err == nil {
		t.Fatal("Expected error when sending frame without connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}
}

func TestHTTPStreamingClient_SequenceIncrement(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	initialSequence := client.sequence

	// Send multiple frames and verify sequence increments
	for i := 0; i < 5; i++ {
		err = client.SendFrame(FrameTypeAudioData, []byte(fmt.Sprintf("data-%d", i)))
		if err != nil {
			t.Fatalf("Failed to send frame %d: %v", i, err)
		}

		expectedSequence := initialSequence + uint32(i) + 1 //nolint:gosec // G115: Safe conversion in test context
		if client.sequence != expectedSequence {
			t.Errorf("Expected sequence %d, got %d", expectedSequence, client.sequence)
		}
	}
}

func TestHTTPStreamingClient_StartStreaming(t *testing.T) {
	// Create response frames
	responseFrame := NewFrame(FrameTypeResponse, 12345, 1, timeToMicros(time.Now()), []byte("response data"))
	responseData, err := responseFrame.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize response frame: %v", err)
	}

	server := createMockStreamingServer(t, [][]byte{responseData}, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err = client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	var receivedFrames []*Frame
	var mu sync.Mutex

	frameHandler := func(frame *Frame) error {
		mu.Lock()
		defer mu.Unlock()
		receivedFrames = append(receivedFrames, frame)
		return nil
	}

	err = client.StartStreaming(frameHandler)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Wait for frame to be processed
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedFrames) == 0 {
		t.Fatal("Expected to receive response frame")
	}

	frame := receivedFrames[0]
	if frame.Type != FrameTypeResponse {
		t.Errorf("Expected frame type %d, got %d", FrameTypeResponse, frame.Type)
	}
	if string(frame.Data) != "response data" {
		t.Errorf("Expected frame data 'response data', got %s", frame.Data)
	}
}

func TestHTTPStreamingClient_StartStreaming_NotConnected(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	err := client.StartStreaming(func(frame *Frame) error { return nil })
	if err == nil {
		t.Fatal("Expected error when starting streaming without connection")
	}

	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Expected 'not connected' error, got: %v", err)
	}
}

func TestHTTPStreamingClient_Disconnect(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
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

func TestHTTPStreamingClient_SendAudioData(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	testData := []byte("audio data")
	err = client.SendAudioData(testData)
	if err != nil {
		t.Fatalf("Failed to send audio data: %v", err)
	}
}

func TestHTTPStreamingClient_SendWakeWord(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	testData := []byte("wake word data")
	err = client.SendWakeWord(testData)
	if err != nil {
		t.Fatalf("Failed to send wake word: %v", err)
	}
}

func TestHTTPStreamingClient_SendHeartbeat(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	err = client.SendHeartbeat()
	if err != nil {
		t.Fatalf("Failed to send heartbeat: %v", err)
	}
}

func TestHTTPStreamingClient_GetSessionID(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	sessionID := client.GetSessionID()
	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	// Session ID should be consistent
	sessionID2 := client.GetSessionID()
	if sessionID != sessionID2 {
		t.Error("Session ID should remain consistent")
	}

	// Session ID should follow expected pattern
	if !strings.HasPrefix(sessionID, "puck-") {
		t.Errorf("Expected session ID to start with 'puck-', got: %s", sessionID)
	}
}

func TestHTTPStreamingClient_SessionIDHash(t *testing.T) {
	client := NewHTTPStreamingClient("http://localhost:3000", "test-puck")

	hash1 := client.getSessionIDHash()
	hash2 := client.getSessionIDHash()

	if hash1 != hash2 {
		t.Error("Session ID hash should be consistent")
	}

	if hash1 == 0 {
		t.Error("Session ID hash should not be zero")
	}
}

func TestHTTPStreamingClient_ConcurrentSendFrame(t *testing.T) {
	server := createMockStreamingServer(t, nil, false)
	defer server.Close()

	client := NewHTTPStreamingClient(server.URL, "test-puck")
	err := client.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Disconnect()

	const numGoroutines = 10
	const framesPerGoroutine = 5

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines*framesPerGoroutine)

	// Send frames concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < framesPerGoroutine; j++ {
				data := []byte(fmt.Sprintf("data-g%d-f%d", goroutineID, j))
				if err := client.SendFrame(FrameTypeAudioData, data); err != nil {
					errChan <- err
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("Concurrent send frame error: %v", err)
	}

	// Verify sequence number is correct (should be numGoroutines * framesPerGoroutine)
	expectedSequence := uint32(numGoroutines * framesPerGoroutine)
	if client.sequence != expectedSequence {
		t.Errorf("Expected final sequence %d, got %d", expectedSequence, client.sequence)
	}
}

func TestGenerateSessionID(t *testing.T) {
	sessionID := generateSessionID()

	if sessionID == "" {
		t.Error("Expected non-empty session ID")
	}

	if !strings.HasPrefix(sessionID, "puck-") {
		t.Errorf("Expected session ID to start with 'puck-', got: %s", sessionID)
	}

	// Generate multiple session IDs to ensure uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateSessionID()
		if ids[id] {
			t.Errorf("Generated duplicate session ID: %s", id)
		}
		ids[id] = true
	}
}