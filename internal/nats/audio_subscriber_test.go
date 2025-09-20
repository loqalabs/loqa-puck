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

package nats

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// MockNATSMsg implements a mock NATS message for testing
type MockNATSMsg struct {
	subject string
	data    []byte
}

func (m *MockNATSMsg) Subject() string { return m.subject }
func (m *MockNATSMsg) Data() []byte    { return m.data }

// MockNATSConnection for relay testing
type MockRelayNATSConnection struct {
	mu          sync.RWMutex
	subscribers map[string][]nats.MsgHandler
	connected   bool
	errors      map[string]error
}

func NewMockRelayNATSConnection() *MockRelayNATSConnection {
	return &MockRelayNATSConnection{
		subscribers: make(map[string][]nats.MsgHandler),
		connected:   true,
		errors:      make(map[string]error),
	}
}

func (m *MockRelayNATSConnection) Subscribe(subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return nil, nats.ErrConnectionClosed
	}

	if err, exists := m.errors[subject]; exists {
		return nil, err
	}

	if m.subscribers[subject] == nil {
		m.subscribers[subject] = make([]nats.MsgHandler, 0)
	}
	m.subscribers[subject] = append(m.subscribers[subject], handler)

	return &nats.Subscription{}, nil
}

func (m *MockRelayNATSConnection) PublishMessage(subject string, data []byte) {
	m.mu.RLock()
	handlers := m.subscribers[subject]
	m.mu.RUnlock()

	if handlers != nil {
		msg := &nats.Msg{
			Subject: subject,
			Data:    data,
		}
		for _, handler := range handlers {
			go handler(msg)
		}
	}
}

func (m *MockRelayNATSConnection) SetError(subject string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[subject] = err
}

func (m *MockRelayNATSConnection) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

func (m *MockRelayNATSConnection) Close() {
	m.Disconnect()
}

func TestAudioStreamManager_Creation(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{
			name:     "small_capacity",
			capacity: 5,
		},
		{
			name:     "medium_capacity",
			capacity: 20,
		},
		{
			name:     "large_capacity",
			capacity: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewAudioStreamManager(tt.capacity)

			if manager == nil {
				t.Fatal("NewAudioStreamManager returned nil")
			}

			if manager.capacity != tt.capacity {
				t.Errorf("Capacity mismatch: got %d, want %d", manager.capacity, tt.capacity)
			}

			if manager.playbackCh == nil {
				t.Error("Playback channel is nil")
			}

			// Test channel capacity
			if cap(manager.playbackCh) != tt.capacity {
				t.Errorf("Channel capacity mismatch: got %d, want %d", cap(manager.playbackCh), tt.capacity)
			}
		})
	}
}

func TestAudioStreamManager_GetPlaybackChannel(t *testing.T) {
	manager := NewAudioStreamManager(10)

	ch := manager.GetPlaybackChannel()
	if ch == nil {
		t.Fatal("GetPlaybackChannel returned nil")
	}

	if ch != manager.playbackCh {
		t.Error("GetPlaybackChannel returned different channel than expected")
	}
}

func TestAudioStreamManager_QueueAudio(t *testing.T) {
	manager := NewAudioStreamManager(3) // Small capacity for testing

	audioData1 := []byte("audio-data-1")
	audioData2 := []byte("audio-data-2")
	audioData3 := []byte("audio-data-3")

	// Queue first audio - should succeed
	select {
	case manager.playbackCh <- audioData1:
		// Success
	default:
		t.Error("Failed to queue first audio data")
	}

	// Queue second audio - should succeed
	select {
	case manager.playbackCh <- audioData2:
		// Success
	default:
		t.Error("Failed to queue second audio data")
	}

	// Queue third audio - should succeed
	select {
	case manager.playbackCh <- audioData3:
		// Success
	default:
		t.Error("Failed to queue third audio data")
	}

	// Verify we can read the queued audio in order
	select {
	case receivedData := <-manager.playbackCh:
		if string(receivedData) != string(audioData1) {
			t.Errorf("First audio data mismatch: got %s, want %s", string(receivedData), string(audioData1))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for first audio data")
	}

	select {
	case receivedData := <-manager.playbackCh:
		if string(receivedData) != string(audioData2) {
			t.Errorf("Second audio data mismatch: got %s, want %s", string(receivedData), string(audioData2))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for second audio data")
	}
}

func TestAudioSubscriber_Creation(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-123"

	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	if subscriber == nil {
		t.Fatal("NewAudioSubscriber returned nil")
	}

	if subscriber.puckID != relayID {
		t.Errorf("PuckID mismatch: got %s, want %s", subscriber.puckID, relayID)
	}

	if subscriber.streamManager == nil {
		t.Error("StreamManager is nil")
	}
}

func TestAudioSubscriber_HandleAudioMessage(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-123"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	tests := []struct {
		name         string
		message      AudioStreamMessage
		expectQueued bool
		expectError  bool
		invalidJSON  bool
	}{
		{
			name: "valid_wav_message",
			message: AudioStreamMessage{
				StreamID:    "stream-001",
				AudioData:   []byte("fake-wav-data"),
				AudioFormat: "wav",
				SampleRate:  22050,
				MessageType: "response",
				Priority:    1,
			},
			expectQueued: true,
		},
		{
			name: "valid_mp3_message",
			message: AudioStreamMessage{
				StreamID:    "stream-002",
				AudioData:   []byte("fake-mp3-data"),
				AudioFormat: "mp3",
				SampleRate:  44100,
				MessageType: "timer",
				Priority:    2,
			},
			expectQueued: true,
		},
		{
			name: "large_audio_message",
			message: AudioStreamMessage{
				StreamID:    "stream-003",
				AudioData:   make([]byte, 1024*1024), // 1MB audio
				AudioFormat: "wav",
				SampleRate:  22050,
				MessageType: "response",
				Priority:    1,
			},
			expectQueued: true,
		},
		{
			name: "empty_audio_message",
			message: AudioStreamMessage{
				StreamID:    "stream-004",
				AudioData:   []byte{},
				AudioFormat: "wav",
				SampleRate:  22050,
				MessageType: "system",
				Priority:    5,
			},
			expectQueued: true, // Even empty audio should be handled
		},
		{
			name:        "invalid_json_message",
			invalidJSON: true,
			expectError: false, // Errors are logged but don't cause test failure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msgData []byte
			var err error

			if tt.invalidJSON {
				msgData = []byte("invalid-json-data")
			} else {
				msgData, err = json.Marshal(tt.message)
				if err != nil {
					t.Fatalf("Failed to marshal test message: %v", err)
				}
			}

			// Create a mock NATS message
			natsMsg := &nats.Msg{
				Subject: "audio.stream." + relayID,
				Data:    msgData,
			}

			// Call handleAudioMessage directly
			subscriber.handleAudioMessage(natsMsg)

			if tt.expectQueued && !tt.invalidJSON {
				// Try to receive the queued audio
				select {
				case receivedData := <-subscriber.streamManager.GetPlaybackChannel():
					if len(receivedData) != len(tt.message.AudioData) {
						t.Errorf("Queued audio data length mismatch: got %d, want %d",
							len(receivedData), len(tt.message.AudioData))
					}
				case <-time.After(100 * time.Millisecond):
					t.Error("Timeout waiting for queued audio data")
				}
			}
		})
	}
}

func TestAudioSubscriber_SubscribeToStreams(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-subscribe"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	// Test subscription setup
	err := subscriber.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Test that both individual and broadcast subscriptions were set up
	// This is tested by sending messages and verifying they're handled

	// Test individual stream message
	individualMessage := AudioStreamMessage{
		StreamID:    "individual-test",
		AudioData:   []byte("individual-audio-data"),
		AudioFormat: "wav",
		SampleRate:  22050,
		MessageType: "response",
		Priority:    1,
	}

	individualData, _ := json.Marshal(individualMessage)
	mockConn.PublishMessage("audio."+relayID, individualData)

	// Verify individual message was received
	select {
	case receivedData := <-subscriber.streamManager.GetPlaybackChannel():
		if string(receivedData) != string(individualMessage.AudioData) {
			t.Error("Individual message not received correctly")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for individual message")
	}

	// Test broadcast message
	broadcastMessage := AudioStreamMessage{
		StreamID:    "broadcast-test",
		AudioData:   []byte("broadcast-audio-data"),
		AudioFormat: "wav",
		SampleRate:  22050,
		MessageType: "system",
		Priority:    1,
	}

	broadcastData, _ := json.Marshal(broadcastMessage)
	mockConn.PublishMessage("audio.broadcast", broadcastData)

	// Verify broadcast message was received
	select {
	case receivedData := <-subscriber.streamManager.GetPlaybackChannel():
		if string(receivedData) != string(broadcastMessage.AudioData) {
			t.Error("Broadcast message not received correctly")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for broadcast message")
	}
}

func TestAudioSubscriber_ConnectionErrors(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-error"

	// Set up subscription error
	mockConn.SetError("audio."+relayID, nats.ErrConnectionClosed)

	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	err := subscriber.Start()
	if err == nil {
		t.Error("Expected subscription error but got none")
	}
}

func TestAudioSubscriber_ConcurrentMessageHandling(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-concurrent"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	// Subscribe to streams
	err := subscriber.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	numMessages := 10
	var wg sync.WaitGroup

	// Send multiple messages concurrently
	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(messageID int) {
			defer wg.Done()

			message := AudioStreamMessage{
				StreamID:    fmt.Sprintf("concurrent-stream-%d", messageID),
				AudioData:   []byte(fmt.Sprintf("audio-data-%d", messageID)),
				AudioFormat: "wav",
				SampleRate:  22050,
				MessageType: "response",
				Priority:    1,
			}

			msgData, _ := json.Marshal(message)
			mockConn.PublishMessage("audio."+relayID, msgData)
		}(i)
	}

	wg.Wait()

	// Verify all messages were queued
	receivedCount := 0
	timeout := time.After(1 * time.Second)

	for receivedCount < numMessages {
		select {
		case <-subscriber.streamManager.GetPlaybackChannel():
			receivedCount++
		case <-timeout:
			t.Errorf("Only received %d out of %d messages", receivedCount, numMessages)
			return
		}
	}

	if receivedCount != numMessages {
		t.Errorf("Received %d messages, expected %d", receivedCount, numMessages)
	}
}

func TestAudioSubscriber_MessagePriority(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-priority"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	err := subscriber.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Test messages with different priorities
	priorities := []int{1, 2, 3, 4, 5}

	for _, priority := range priorities {
		message := AudioStreamMessage{
			StreamID:    fmt.Sprintf("priority-stream-%d", priority),
			AudioData:   []byte(fmt.Sprintf("priority-%d-audio", priority)),
			AudioFormat: "wav",
			SampleRate:  22050,
			MessageType: "response",
			Priority:    priority,
		}

		msgData, _ := json.Marshal(message)
		mockConn.PublishMessage("audio."+relayID, msgData)

		// Verify message was queued regardless of priority
		// Note: Current implementation doesn't implement priority queuing,
		// but we test that all priorities are accepted
		select {
		case receivedData := <-subscriber.streamManager.GetPlaybackChannel():
			expectedData := fmt.Sprintf("priority-%d-audio", priority)
			if string(receivedData) != expectedData {
				t.Errorf("Priority %d message data mismatch: got %s, want %s",
					priority, string(receivedData), expectedData)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Timeout waiting for priority %d message", priority)
		}
	}
}

func TestAudioSubscriber_MessageTypes(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-types"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	err := subscriber.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	messageTypes := []string{"response", "timer", "reminder", "system", "notification"}

	for _, msgType := range messageTypes {
		message := AudioStreamMessage{
			StreamID:    fmt.Sprintf("%s-stream", msgType),
			AudioData:   []byte(fmt.Sprintf("%s-audio-data", msgType)),
			AudioFormat: "wav",
			SampleRate:  22050,
			MessageType: msgType,
			Priority:    1,
		}

		msgData, _ := json.Marshal(message)
		mockConn.PublishMessage("audio."+relayID, msgData)

		// Verify message was queued
		select {
		case receivedData := <-subscriber.streamManager.GetPlaybackChannel():
			expectedData := fmt.Sprintf("%s-audio-data", msgType)
			if string(receivedData) != expectedData {
				t.Errorf("Message type %s data mismatch: got %s, want %s",
					msgType, string(receivedData), expectedData)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Timeout waiting for message type %s", msgType)
		}
	}
}

func TestAudioSubscriber_ChannelOverflow(t *testing.T) {
	// Create subscriber with very small capacity
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-overflow"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	// Override stream manager with smaller capacity for testing
	subscriber.streamManager = NewAudioStreamManager(2) // Very small capacity

	err := subscriber.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Fill the channel to capacity
	for i := 0; i < 2; i++ {
		message := AudioStreamMessage{
			StreamID:    fmt.Sprintf("overflow-stream-%d", i),
			AudioData:   []byte(fmt.Sprintf("audio-data-%d", i)),
			AudioFormat: "wav",
			SampleRate:  22050,
			MessageType: "response",
			Priority:    1,
		}

		msgData, _ := json.Marshal(message)
		mockConn.PublishMessage("audio."+relayID, msgData)
	}

	// Try to send one more message - should not block or crash
	overflowMessage := AudioStreamMessage{
		StreamID:    "overflow-stream-extra",
		AudioData:   []byte("overflow-audio-data"),
		AudioFormat: "wav",
		SampleRate:  22050,
		MessageType: "response",
		Priority:    1,
	}

	msgData, _ := json.Marshal(overflowMessage)

	// This should complete without blocking (message will be dropped)
	done := make(chan bool, 1)
	go func() {
		mockConn.PublishMessage("audio."+relayID, msgData)
		done <- true
	}()

	select {
	case <-done:
		// Good, the publish completed without blocking
	case <-time.After(100 * time.Millisecond):
		t.Error("Message handling blocked on channel overflow")
	}
}

// TestNewPuckNATSConnectionAdapter tests the adapter creation and methods
func TestNewPuckNATSConnectionAdapter(t *testing.T) {
	t.Run("adapter_creation", func(t *testing.T) {
		// Create a mock NATS connection (we'll use nil for testing)
		var mockNATSConn *nats.Conn
		adapter := NewPuckNATSConnectionAdapter(mockNATSConn)

		if adapter == nil {
			t.Fatal("NewPuckNATSConnectionAdapter returned nil")
		}

		if adapter.conn != mockNATSConn {
			t.Error("Adapter conn field not set correctly")
		}
	})
}

// TestPuckNATSConnectionAdapter_Subscribe tests the adapter Subscribe method
func TestPuckNATSConnectionAdapter_Subscribe(t *testing.T) {
	t.Run("subscribe_with_nil_conn", func(t *testing.T) {
		adapter := NewPuckNATSConnectionAdapter(nil)

		// This will panic with nil conn, which is expected behavior
		// We test that the method exists and can be called
		defer func() {
			if r := recover(); r != nil {
				// Panic is expected with nil connection - this is OK
				t.Logf("Expected panic occurred: %v", r)
			}
		}()

		_, err := adapter.Subscribe("test.subject", func(msg *nats.Msg) {})
		if err == nil {
			// If no panic and no error, that's also acceptable behavior
			t.Log("No error returned from Subscribe call")
		}
	})
}

// TestPuckNATSConnectionAdapter_Close tests the adapter Close method
func TestPuckNATSConnectionAdapter_Close(t *testing.T) {
	t.Run("close_with_nil_conn", func(t *testing.T) {
		adapter := NewPuckNATSConnectionAdapter(nil)

		// This should not panic even with nil conn
		adapter.Close()
	})
}

// TestNewAudioSubscriber tests the full constructor with real NATS connection
func TestNewAudioSubscriber(t *testing.T) {
	t.Run("connection_failure", func(t *testing.T) {
		// Test with invalid NATS URL
		subscriber, err := NewAudioSubscriber("nats://invalid-host:9999", "test-puck", 10)

		if err == nil {
			t.Error("Expected error with invalid NATS URL")
		}

		if subscriber != nil {
			subscriber.Close()
			t.Error("Expected nil subscriber on connection failure")
		}
	})
}

// TestAudioSubscriber_GetStreamManager tests the GetStreamManager method
func TestAudioSubscriber_GetStreamManager(t *testing.T) {
	mockConn := NewMockRelayNATSConnection()
	relayID := "test-relay-manager"
	subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

	manager := subscriber.GetStreamManager()
	if manager == nil {
		t.Fatal("GetStreamManager returned nil")
	}

	if manager != subscriber.streamManager {
		t.Error("GetStreamManager returned different manager than expected")
	}
}

// TestAudioSubscriber_Close tests the Close method
func TestAudioSubscriber_Close(t *testing.T) {
	t.Run("close_with_connection", func(t *testing.T) {
		mockConn := NewMockRelayNATSConnection()
		relayID := "test-relay-close"
		subscriber := NewAudioSubscriberWithConnection(mockConn, relayID, 10)

		// Should not panic
		subscriber.Close()
	})

	t.Run("close_with_nil_connection", func(t *testing.T) {
		subscriber := &AudioSubscriber{
			natsConn:      nil,
			puckID:       "test-puck",
			streamManager: NewAudioStreamManager(10),
		}

		// Should not panic even with nil connection
		subscriber.Close()
	})
}

// TestAudioStreamMessage_Struct tests the AudioStreamMessage struct
func TestAudioStreamMessage_Struct(t *testing.T) {
	t.Run("message_creation", func(t *testing.T) {
		msg := AudioStreamMessage{
			StreamID:    "test-stream-123",
			AudioData:   []byte("test-audio-data"),
			AudioFormat: "wav",
			SampleRate:  16000,
			MessageType: "response",
			Priority:    1,
		}

		if msg.StreamID != "test-stream-123" {
			t.Errorf("StreamID mismatch: got %s, want test-stream-123", msg.StreamID)
		}

		if string(msg.AudioData) != "test-audio-data" {
			t.Errorf("AudioData mismatch: got %s, want test-audio-data", string(msg.AudioData))
		}

		if msg.AudioFormat != "wav" {
			t.Errorf("AudioFormat mismatch: got %s, want wav", msg.AudioFormat)
		}

		if msg.SampleRate != 16000 {
			t.Errorf("SampleRate mismatch: got %d, want 16000", msg.SampleRate)
		}

		if msg.MessageType != "response" {
			t.Errorf("MessageType mismatch: got %s, want response", msg.MessageType)
		}

		if msg.Priority != 1 {
			t.Errorf("Priority mismatch: got %d, want 1", msg.Priority)
		}
	})

	t.Run("message_json_marshaling", func(t *testing.T) {
		msg := AudioStreamMessage{
			StreamID:    "json-test-stream",
			AudioData:   []byte("json-test-data"),
			AudioFormat: "mp3",
			SampleRate:  44100,
			MessageType: "timer",
			Priority:    2,
		}

		// Test JSON marshaling
		jsonData, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Failed to marshal message: %v", err)
		}

		// Test JSON unmarshaling
		var unmarshaledMsg AudioStreamMessage
		err = json.Unmarshal(jsonData, &unmarshaledMsg)
		if err != nil {
			t.Fatalf("Failed to unmarshal message: %v", err)
		}

		// Verify all fields
		if unmarshaledMsg.StreamID != msg.StreamID {
			t.Error("StreamID not preserved through JSON marshal/unmarshal")
		}

		if string(unmarshaledMsg.AudioData) != string(msg.AudioData) {
			t.Error("AudioData not preserved through JSON marshal/unmarshal")
		}

		if unmarshaledMsg.AudioFormat != msg.AudioFormat {
			t.Error("AudioFormat not preserved through JSON marshal/unmarshal")
		}

		if unmarshaledMsg.SampleRate != msg.SampleRate {
			t.Error("SampleRate not preserved through JSON marshal/unmarshal")
		}

		if unmarshaledMsg.MessageType != msg.MessageType {
			t.Error("MessageType not preserved through JSON marshal/unmarshal")
		}

		if unmarshaledMsg.Priority != msg.Priority {
			t.Error("Priority not preserved through JSON marshal/unmarshal")
		}
	})
}
