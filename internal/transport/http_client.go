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

package transport

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPStreamingClient handles HTTP/1.1 streaming communication with the hub
type HTTPStreamingClient struct {
	hubURL         string
	puckID         string
	sessionID      string
	sequence       uint32
	mutex          sync.Mutex
	connectTimeout time.Duration

	// HTTP client for requests
	client      *http.Client
	isConnected bool

	// Context for connection lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Streaming connection
	response *http.Response
	reader   *bufio.Reader
	writer   io.WriteCloser
}

// NewHTTPStreamingClient creates a new HTTP streaming client
func NewHTTPStreamingClient(hubURL, puckID string) *HTTPStreamingClient {
	ctx, cancel := context.WithCancel(context.Background())

	// Create HTTP transport with aggressive cleanup settings
	transport := &http.Transport{
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     1 * time.Second,
		DisableKeepAlives:   true, // Disable connection pooling for cleaner shutdown
		ForceAttemptHTTP2:   false,
	}

	return &HTTPStreamingClient{
		hubURL:         hubURL,
		puckID:         puckID,
		sessionID:      generateSessionID(),
		sequence:       0,
		connectTimeout: 10 * time.Second, // Default timeout
		client:         &http.Client{
			Timeout:   0, // No timeout for persistent streaming
			Transport: transport,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetConnectTimeout sets the connection timeout for testing
func (c *HTTPStreamingClient) SetConnectTimeout(timeout time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.connectTimeout = timeout
}

// Connect establishes HTTP streaming connection to the hub
func (c *HTTPStreamingClient) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.isConnected {
		return fmt.Errorf("already connected")
	}

	log.Printf("🔗 Connecting to hub at %s", c.hubURL)

	// Parse the hub URL and construct streaming endpoint
	u, err := url.Parse(c.hubURL)
	if err != nil {
		return fmt.Errorf("invalid hub URL: %w", err)
	}

	// Construct the streaming endpoint URL
	streamURL := fmt.Sprintf("http://%s/stream/puck?puck_id=%s", u.Host, c.puckID)

	// Create HTTP request for streaming with empty body initially
	// We'll handle bidirectional communication after connection establishment
	req, err := http.NewRequestWithContext(c.ctx, "POST", streamURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers for bidirectional streaming
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Transfer-Encoding", "chunked")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-Puck-ID", c.puckID)
	req.Header.Set("X-Session-ID", c.sessionID)

	// Start the request in a goroutine
	responseChan := make(chan *http.Response, 1)
	errorChan := make(chan error, 1)

	go func() {
		log.Printf("🚀 Starting HTTP request...")
		resp, err := c.client.Do(req)
		if err != nil {
			log.Printf("❌ HTTP request failed: %v", err)
			errorChan <- err
			return
		}
		log.Printf("✅ HTTP request succeeded with status: %d", resp.StatusCode)
		responseChan <- resp
	}()

	// Wait for the connection to establish with proper timeout
	select {
	case resp := <-responseChan:
		if resp.StatusCode != http.StatusOK {
			if err := resp.Body.Close(); err != nil {
				log.Printf("⚠️ Failed to close response body: %v", err)
			}
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		c.response = resp
		c.reader = bufio.NewReader(resp.Body)
		c.writer = nil // We'll set this up later for streaming

		// Connection established successfully
		// Note: Handshake frame sending moved to after isConnected is set

	case err := <-errorChan:
		return fmt.Errorf("failed to connect to hub: %w", err)

	case <-time.After(c.connectTimeout):
		return fmt.Errorf("connection timeout")
	}

	c.isConnected = true
	log.Printf("✅ Connected to hub successfully (session: %s)", c.sessionID)
	return nil
}

// SendHandshake sends a handshake frame to establish the session with the hub
func (c *HTTPStreamingClient) SendHandshake() error {
	if !c.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// Create handshake data with session information
	handshakeData := fmt.Sprintf("session:%s;puck:%s", c.sessionID, c.puckID)

	log.Printf("🤝 Sending handshake frame (session: %s)", c.sessionID)
	return c.SendFrame(FrameTypeHandshake, []byte(handshakeData))
}

// SendFrame sends a frame to the hub via separate HTTP request
func (c *HTTPStreamingClient) SendFrame(frameType FrameType, data []byte) error {
	if !c.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	c.mutex.Lock()
	c.sequence++
	seq := c.sequence
	c.mutex.Unlock()

	frame := NewFrame(
		frameType,
		c.getSessionIDHash(),
		seq,
		uint64(time.Now().UnixMicro()), //nolint:gosec // Safe conversion from int64 to uint64
		data,
	)

	frameData, err := frame.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize frame: %w", err)
	}

	// For HTTP/1.1, send frames via separate POST requests
	// This is more compatible than trying to use bidirectional streaming
	sendURL := fmt.Sprintf("%s/send/puck?puck_id=%s", c.hubURL, c.puckID)

	req, err := http.NewRequestWithContext(c.ctx, "POST", sendURL, bytes.NewReader(frameData))
	if err != nil {
		return fmt.Errorf("failed to create send request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Puck-ID", c.puckID)
	req.Header.Set("X-Session-ID", c.sessionID)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send frame: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("⚠️ Failed to close send response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send frame failed with status: %d", resp.StatusCode)
	}

	// Only log non-audio frames to reduce test noise
	if frame.Type != FrameTypeAudioData {
		log.Printf("📤 Sent frame type %d (%d bytes)", frame.Type, len(frameData))
	}
	return nil
}

// StartStreaming begins the streaming process with frame handler
func (c *HTTPStreamingClient) StartStreaming(frameHandler func(*Frame) error) error {
	if !c.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// Start incoming frame reader
	go c.handleIncomingFrames(frameHandler)

	log.Println("🎙️ HTTP streaming started")
	return nil
}

// handleIncomingFrames processes frames received from the hub
func (c *HTTPStreamingClient) handleIncomingFrames(frameHandler func(*Frame) error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Incoming frame handler panic: %v", r)
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Read frame header first
			headerData := make([]byte, HeaderSize)
			if _, err := io.ReadFull(c.reader, headerData); err != nil {
				if err == io.EOF {
					log.Println("🎙️ Stream ended")
					return
				}
				log.Printf("❌ Failed to read frame header: %v", err)
				return
			}

			// Parse header
			header, err := parseFrameHeader(headerData)
			if err != nil {
				log.Printf("❌ Invalid frame header: %v", err)
				continue
			}

			// Read frame data if present
			var frameData []byte
			if header.Length > 0 {
				frameData = make([]byte, header.Length)
				if _, err := io.ReadFull(c.reader, frameData); err != nil {
					log.Printf("❌ Failed to read frame data: %v", err)
					continue
				}
			}

			// Reconstruct full frame data for deserialization
			fullFrameData := append(headerData, frameData...)
			frame, err := DeserializeFrame(fullFrameData)
			if err != nil {
				log.Printf("❌ Failed to deserialize frame: %v", err)
				continue
			}

			log.Printf("📥 Received frame type %d (%d bytes)", frame.Type, len(frame.Data))

			// Handle the frame
			if frameHandler != nil {
				if err := frameHandler(frame); err != nil {
					log.Printf("❌ Frame handler error: %v", err)
				}
			}
		}
	}
}

// SendAudioData sends audio data as a frame
func (c *HTTPStreamingClient) SendAudioData(audioData []byte) error {
	return c.SendFrame(FrameTypeAudioData, audioData)
}

// SendWakeWord sends a wake word detection frame
func (c *HTTPStreamingClient) SendWakeWord(audioData []byte) error {
	return c.SendFrame(FrameTypeWakeWord, audioData)
}

// SendHeartbeat sends a heartbeat frame to keep the connection alive
func (c *HTTPStreamingClient) SendHeartbeat() error {
	return c.SendFrame(FrameTypeHeartbeat, nil)
}

// Disconnect closes the connection to the hub
func (c *HTTPStreamingClient) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isConnected {
		return
	}

	log.Println("🔗 Disconnecting from hub...")

	// Cancel context to stop goroutines
	c.cancel()

	// Close writer
	if c.writer != nil {
		if err := c.writer.Close(); err != nil {
			log.Printf("⚠️ Failed to close writer: %v", err)
		}
	}

	// Close response body
	if c.response != nil {
		if err := c.response.Body.Close(); err != nil {
			log.Printf("⚠️ Failed to close response body: %v", err)
		}
	}

	// Force close idle connections in the HTTP transport
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	c.isConnected = false
	log.Println("👋 Disconnected from hub")
}

// IsConnected returns whether the client is currently connected
func (c *HTTPStreamingClient) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.isConnected
}

// GetSessionID returns the current session ID
func (c *HTTPStreamingClient) GetSessionID() string {
	return c.sessionID
}

// getSessionIDHash returns a hash of the session ID for frame headers
func (c *HTTPStreamingClient) getSessionIDHash() uint32 {
	hash := uint32(0)
	for _, b := range []byte(c.sessionID) {
		hash = hash*31 + uint32(b)
	}
	return hash
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	// Session ID generation with timestamp and random component for uniqueness
	now := time.Now()
	random := rand.Int63n(1000000) //nolint:gosec // G404: Non-cryptographic random OK for session ID
	return fmt.Sprintf("puck-%d-%d-%d", now.Unix(), now.Nanosecond(), random)
}
