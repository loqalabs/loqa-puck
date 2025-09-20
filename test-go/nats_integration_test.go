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
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockNATSServer provides a test-friendly NATS server interface
type MockNATSServer struct {
	conn       *nats.Conn
	subs       map[string]*nats.Subscription
	messages   []nats.Msg
	mu         sync.RWMutex
	connected  bool
	url        string
	clientID   string
}

// NewMockNATSServer creates a new mock NATS server for testing
func NewMockNATSServer(url, clientID string) *MockNATSServer {
	return &MockNATSServer{
		subs:     make(map[string]*nats.Subscription),
		messages: []nats.Msg{},
		url:      url,
		clientID: clientID,
	}
}

// Connect simulates NATS connection
func (m *MockNATSServer) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simulate connection failure for invalid URLs
	if m.url == "nats://localhost:9999" {
		return fmt.Errorf("connection refused")
	}

	m.connected = true
	return nil
}

// Disconnect simulates NATS disconnection
func (m *MockNATSServer) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connected = false
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}

	// Clear subscriptions
	for _, sub := range m.subs {
		_ = sub.Unsubscribe() // Ignore errors during test cleanup
	}
	m.subs = make(map[string]*nats.Subscription)
}

// Publish simulates publishing a message
func (m *MockNATSServer) Publish(subject string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return fmt.Errorf("not connected to NATS")
	}

	// Create mock message
	msg := nats.Msg{
		Subject: subject,
		Data:    data,
	}
	m.messages = append(m.messages, msg)

	return nil
}

// Subscribe simulates subscribing to a subject
func (m *MockNATSServer) Subscribe(subject string, _ nats.MsgHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return fmt.Errorf("not connected to NATS")
	}

	// Create mock subscription
	sub := &nats.Subscription{}
	m.subs[subject] = sub

	return nil
}

// GetMessages returns all published messages for testing
func (m *MockNATSServer) GetMessages() []nats.Msg {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]nats.Msg, len(m.messages))
	copy(result, m.messages)
	return result
}

// IsConnected returns connection status
func (m *MockNATSServer) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// ClearMessages clears the message history
func (m *MockNATSServer) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = []nats.Msg{}
}

// TestNATSConnection tests basic NATS connection functionality
func TestNATSConnection(t *testing.T) {
	t.Run("successful_connection", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")

		err := mock.Connect()
		require.NoError(t, err, "should connect successfully")

		assert.True(t, mock.IsConnected(), "should be connected")

		mock.Disconnect()
		assert.False(t, mock.IsConnected(), "should be disconnected")
	})

	t.Run("connection_failure", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:9999", "test-puck")

		err := mock.Connect()
		require.Error(t, err, "should fail to connect to invalid server")

		assert.False(t, mock.IsConnected(), "should not be connected")
	})

	t.Run("reconnection_handling", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")

		// Initial connection
		err := mock.Connect()
		require.NoError(t, err)
		assert.True(t, mock.IsConnected())

		// Simulate disconnection
		mock.Disconnect()
		assert.False(t, mock.IsConnected())

		// Reconnection
		err = mock.Connect()
		require.NoError(t, err)
		assert.True(t, mock.IsConnected())

		mock.Disconnect()
	})
}

// TestNATSAudioStreaming tests audio streaming via NATS
func TestNATSAudioStreaming(t *testing.T) {
	t.Run("audio_publish", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Simulate audio data
		audioData := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
		subject := "audio.stream.test-puck"

		err = mock.Publish(subject, audioData)
		require.NoError(t, err, "should publish audio data successfully")

		messages := mock.GetMessages()
		require.Len(t, messages, 1, "should have one published message")

		assert.Equal(t, subject, messages[0].Subject, "subject should match")
		assert.Equal(t, audioData, messages[0].Data, "audio data should match")
	})

	t.Run("large_audio_buffer", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Simulate large audio buffer (1 second at 16kHz, 16-bit)
		largeAudioData := make([]byte, 16000*2)
		for i := range largeAudioData {
			largeAudioData[i] = byte(i % 256)
		}

		subject := "audio.stream.test-puck"
		err = mock.Publish(subject, largeAudioData)
		require.NoError(t, err, "should handle large audio buffers")

		messages := mock.GetMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, len(largeAudioData), len(messages[0].Data), "large buffer should be preserved")
	})

	t.Run("publish_without_connection", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		// Don't connect

		audioData := []byte{0x00, 0x01}
		err := mock.Publish("audio.stream.test", audioData)
		require.Error(t, err, "should fail to publish without connection")
	})
}

// TestNATSSubscriptions tests NATS subscription functionality
func TestNATSSubscriptions(t *testing.T) {
	t.Run("command_subscription", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Subscribe to command subject
		commandSubject := "cmd.puck.test-puck"
		err = mock.Subscribe(commandSubject, func(msg *nats.Msg) {
			// Command handler
		})
		require.NoError(t, err, "should subscribe to commands successfully")
	})

	t.Run("status_subscription", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Subscribe to status updates
		statusSubject := "status.puck.test-puck"
		err = mock.Subscribe(statusSubject, func(msg *nats.Msg) {
			// Status handler
		})
		require.NoError(t, err, "should subscribe to status updates successfully")
	})

	t.Run("subscribe_without_connection", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		// Don't connect

		err := mock.Subscribe("test.subject", func(msg *nats.Msg) {})
		require.Error(t, err, "should fail to subscribe without connection")
	})
}

// TestNATSConcurrentOperations tests concurrent NATS operations
func TestNATSConcurrentOperations(t *testing.T) {
	t.Run("concurrent_publish", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		numWorkers := 10
		messagesPerWorker := 5

		var wg sync.WaitGroup
		wg.Add(numWorkers)

		// Start concurrent publishers
		for i := 0; i < numWorkers; i++ {
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < messagesPerWorker; j++ {
					data := []byte(fmt.Sprintf("worker-%d-msg-%d", workerID, j))
					subject := fmt.Sprintf("test.worker.%d", workerID)

					err := mock.Publish(subject, data)
					if err != nil {
						t.Errorf("worker %d failed to publish message %d: %v", workerID, j, err)
						return
					}
				}
			}(i)
		}

		wg.Wait()

		messages := mock.GetMessages()
		expectedCount := numWorkers * messagesPerWorker
		assert.Equal(t, expectedCount, len(messages), "should receive all concurrent messages")
	})

	t.Run("concurrent_subscribe", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		numSubscribers := 5
		var wg sync.WaitGroup
		wg.Add(numSubscribers)

		// Start concurrent subscribers
		for i := 0; i < numSubscribers; i++ {
			go func(subID int) {
				defer wg.Done()

				subject := fmt.Sprintf("test.sub.%d", subID)
				err := mock.Subscribe(subject, func(msg *nats.Msg) {
					// Handler
				})
				if err != nil {
					t.Errorf("subscriber %d failed to subscribe: %v", subID, err)
				}
			}(i)
		}

		wg.Wait()
	})
}

// TestNATSErrorHandling tests NATS error handling scenarios
func TestNATSErrorHandling(t *testing.T) {
	t.Run("network_interruption", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)

		// Simulate network interruption
		mock.Disconnect()

		// Try to publish during disconnection
		err = mock.Publish("test.subject", []byte("test"))
		require.Error(t, err, "should fail during network interruption")

		// Reconnect and verify recovery
		err = mock.Connect()
		require.NoError(t, err)

		err = mock.Publish("test.subject", []byte("test"))
		require.NoError(t, err, "should recover after reconnection")

		mock.Disconnect()
	})

	t.Run("invalid_subject", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Test with empty subject (should be handled gracefully)
		_ = mock.Publish("", []byte("test")) // Ignore errors in edge case test
	})

	t.Run("message_size_limits", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Test with very large message
		largeMessage := make([]byte, 10*1024*1024) // 10MB
		for i := range largeMessage {
			largeMessage[i] = byte(i % 256)
		}

		err = mock.Publish("test.large", largeMessage)
		// Mock accepts any size, but real NATS has limits
		require.NoError(t, err)

		messages := mock.GetMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, len(largeMessage), len(messages[0].Data))
	})
}

// TestNATSPerformance tests NATS performance characteristics
func TestNATSPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Run("throughput_test", func(t *testing.T) {
		mock := NewMockNATSServer("nats://localhost:4222", "test-puck")
		err := mock.Connect()
		require.NoError(t, err)
		defer mock.Disconnect()

		// Clear any existing messages
		mock.ClearMessages()

		numMessages := 1000
		messageSize := 1024 // 1KB per message

		startTime := time.Now()

		for i := 0; i < numMessages; i++ {
			data := make([]byte, messageSize)
			for j := range data {
				data[j] = byte(j % 256)
			}

			subject := fmt.Sprintf("perf.test.%d", i)
			err := mock.Publish(subject, data)
			require.NoError(t, err)
		}

		duration := time.Since(startTime)

		messages := mock.GetMessages()
		assert.Equal(t, numMessages, len(messages), "should publish all messages")

		// Log performance metrics (not strict requirements for mock)
		throughput := float64(numMessages) / duration.Seconds()
		t.Logf("Published %d messages in %v (%.2f msg/sec)", numMessages, duration, throughput)

		// Very lenient performance check for mock
		assert.Less(t, duration, 5*time.Second, "should complete within reasonable time")
	})
}

// TestNATSIntegrationLifecycle tests full NATS integration lifecycle
func TestNATSIntegrationLifecycle(t *testing.T) {
	t.Run("full_lifecycle", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		mock := NewMockNATSServer("nats://localhost:4222", "test-puck-lifecycle")

		// Phase 1: Connection
		err := mock.Connect()
		require.NoError(t, err, "should connect successfully")
		require.True(t, mock.IsConnected(), "should be connected")

		// Phase 2: Setup subscriptions
		commandReceived := make(chan []byte, 1)
		err = mock.Subscribe("cmd.puck.test-puck-lifecycle", func(msg *nats.Msg) {
			select {
			case commandReceived <- msg.Data:
			case <-ctx.Done():
			}
		})
		require.NoError(t, err, "should setup command subscription")

		// Phase 3: Audio streaming
		audioData := []byte{0x01, 0x02, 0x03, 0x04}
		err = mock.Publish("audio.stream.test-puck-lifecycle", audioData)
		require.NoError(t, err, "should stream audio")

		// Phase 4: Status reporting
		statusData := []byte(`{"status":"active","timestamp":"2024-01-01T00:00:00Z"}`)
		err = mock.Publish("status.puck.test-puck-lifecycle", statusData)
		require.NoError(t, err, "should report status")

		// Phase 5: Verify messages
		messages := mock.GetMessages()
		require.Len(t, messages, 2, "should have audio and status messages")

		// Verify audio message
		audioMsg := messages[0]
		assert.Equal(t, "audio.stream.test-puck-lifecycle", audioMsg.Subject)
		assert.Equal(t, audioData, audioMsg.Data)

		// Verify status message
		statusMsg := messages[1]
		assert.Equal(t, "status.puck.test-puck-lifecycle", statusMsg.Subject)
		assert.Equal(t, statusData, statusMsg.Data)

		// Phase 6: Graceful shutdown
		mock.Disconnect()
		assert.False(t, mock.IsConnected(), "should be disconnected")
	})
}

// Benchmark NATS operations
func BenchmarkNATSPublish(b *testing.B) {
	mock := NewMockNATSServer("nats://localhost:4222", "bench-puck")
	err := mock.Connect()
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer mock.Disconnect()

	data := make([]byte, 1024) // 1KB message
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := mock.Publish("bench.test", data)
		if err != nil {
			b.Fatalf("Publish failed: %v", err)
		}
	}
}

func BenchmarkNATSSubscribe(b *testing.B) {
	mock := NewMockNATSServer("nats://localhost:4222", "bench-puck")
	err := mock.Connect()
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer mock.Disconnect()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		subject := fmt.Sprintf("bench.sub.%d", i)
		err := mock.Subscribe(subject, func(msg *nats.Msg) {})
		if err != nil {
			b.Fatalf("Subscribe failed: %v", err)
		}
	}
}