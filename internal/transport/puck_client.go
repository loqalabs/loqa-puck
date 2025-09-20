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
	"fmt"
	"log"
	"time"

	"github.com/loqalabs/loqa-puck-go/internal/audio"
)

// PuckClient provides a high-level interface for streaming audio to the hub
// This replaces the previous gRPC-based client with HTTP/1.1 streaming
type PuckClient struct {
	streamingClient *HTTPStreamingClient
	hubAddress      string
	puckID          string
	isConnected     bool

	// Response handling would go here if needed in future
}

// NewPuckClient creates a new puck client for HTTP streaming
func NewPuckClient(hubAddress, puckID string) *PuckClient {
	return &PuckClient{
		streamingClient: NewHTTPStreamingClient(hubAddress, puckID),
		hubAddress:      hubAddress,
		puckID:          puckID,
		isConnected:     false,
	}
}

// Connect establishes connection to the hub
func (pc *PuckClient) Connect() error {
	log.Printf("🔗 Puck: Connecting to hub at %s", pc.hubAddress)

	if err := pc.streamingClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	pc.isConnected = true
	log.Printf("✅ Puck: Connected to hub successfully")
	return nil
}

// SendHandshake sends a handshake frame to establish the session with the hub
func (pc *PuckClient) SendHandshake() error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	log.Printf("🤝 Puck: Sending handshake frame")
	return pc.streamingClient.SendHandshake()
}

// StreamAudio starts streaming audio chunks to the hub
// This maintains the same interface as the previous gRPC client
func (pc *PuckClient) StreamAudio(audioChan <-chan audio.AudioChunk, responseChan chan<- interface{}) error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// Start the HTTP streaming
	frameHandler := func(frame *Frame) error {
		return pc.handleIncomingFrame(frame, responseChan)
	}

	if err := pc.streamingClient.StartStreaming(frameHandler); err != nil {
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	log.Println("🎙️ Puck: Audio streaming started")

	// Start goroutine to handle outgoing audio chunks
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("❌ Audio streaming panic: %v", r)
			}
		}()

		chunkCount := 0
		for { //nolint:staticcheck // Channel-based communication pattern
			select {
			case audioChunk, ok := <-audioChan:
				if !ok {
					log.Println("🎙️ Puck: Audio channel closed")
					return
				}

				// Convert audio chunk to binary data
				audioData := pc.convertAudioChunkToBytes(audioChunk)

				// Determine frame type based on audio chunk properties
				var frameType FrameType
				if audioChunk.IsWakeWord {
					frameType = FrameTypeWakeWord
				} else {
					frameType = FrameTypeAudioData
				}

				// Send frame to hub
				if err := pc.streamingClient.SendFrame(frameType, audioData); err != nil {
					log.Printf("❌ Failed to send audio frame: %v", err)
					continue
				}

				chunkCount++
				// Only log occasionally to reduce test noise
				if chunkCount%10 == 0 {
					log.Printf("📤 Puck: Sent audio chunk (%d samples)", len(audioChunk.Data))
				}
			}
		}
	}()

	// Start heartbeat goroutine
	go pc.startHeartbeat()

	return nil
}

// handleIncomingFrame processes frames received from the hub
func (pc *PuckClient) handleIncomingFrame(frame *Frame, responseChan chan<- interface{}) error {
	switch frame.Type {
	case FrameTypeResponse:
		log.Printf("📥 Puck: Received response frame (%d bytes)", len(frame.Data))
		// Send the raw frame data directly to maintain compatibility
		if responseChan != nil {
			select {
			case responseChan <- frame.Data:
				// Successfully sent response data
			default:
				log.Println("⚠️ Response channel full, dropping response")
			}
		}

	case FrameTypeStatus:
		log.Printf("📥 Puck: Received status frame (%d bytes)", len(frame.Data))
		// Send status data to response channel for processing
		if responseChan != nil {
			select {
			case responseChan <- frame.Data:
				// Successfully sent status data
			default:
				log.Println("⚠️ Response channel full, dropping status")
			}
		}

	case FrameTypeHeartbeat:
		log.Printf("📥 Puck: Received heartbeat from hub")
		// Hub is alive, no action needed

	case FrameTypeError:
		log.Printf("❌ Puck: Received error frame: %s", string(frame.Data))
		// Log error but don't return error to avoid breaking the streaming loop

	default:
		log.Printf("⚠️ Puck: Received unknown frame type: %d", frame.Type)
	}

	return nil
}

// convertAudioChunkToBytes converts an AudioChunk to binary data
func (pc *PuckClient) convertAudioChunkToBytes(chunk audio.AudioChunk) []byte {
	// Convert float32 samples to 16-bit PCM bytes
	data := make([]byte, len(chunk.Data)*2)
	for i, sample := range chunk.Data {
		// Convert from float32 [-1,1] to int16 [-32768,32767]
		// Scale by 32768 but clamp to valid int16 range
		scaled := sample * 32768
		var val int16
		if scaled > 32767 {
			val = 32767
		} else if scaled <= -32768 {
			val = -32767  // Use -32767 instead of -32768 for symmetry
		} else {
			val = int16(scaled)
		}
		data[i*2] = byte(val)
		data[i*2+1] = byte(val >> 8)
	}
	return data
}

// startHeartbeat sends periodic heartbeats to keep the connection alive
func (pc *PuckClient) startHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for { //nolint:staticcheck // Ticker-based heartbeat pattern
		select {
		case <-ticker.C:
			if pc.isConnected {
				if err := pc.streamingClient.SendHeartbeat(); err != nil {
					log.Printf("⚠️ Failed to send heartbeat: %v", err)
				}
			}
		}
	}
}

// Disconnect closes the connection to the hub
func (pc *PuckClient) Disconnect() {
	if pc.streamingClient != nil {
		pc.streamingClient.Disconnect()
	}
	pc.isConnected = false
	log.Println("🔗 Puck: Disconnected from hub")
}

// IsConnected returns whether the client is currently connected
func (pc *PuckClient) IsConnected() bool {
	return pc.isConnected && pc.streamingClient.IsConnected()
}