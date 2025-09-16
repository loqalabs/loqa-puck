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

package grpc

import (
	"context"
	"fmt"
	"io"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/loqalabs/loqa-proto/go/audio"
	"github.com/loqalabs/loqa-relay/test-go/internal/audio"
)

// RelayClient handles gRPC communication with the hub
type RelayClient struct {
	conn        *grpc.ClientConn
	audioClient pb.AudioServiceClient
	hubAddress  string
	relayID     string
	isConnected bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewRelayClient creates a new gRPC client for the relay
func NewRelayClient(hubAddress, relayID string) *RelayClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &RelayClient{
		hubAddress: hubAddress,
		relayID:    relayID,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Connect establishes connection to the hub
func (pc *RelayClient) Connect() error {
	log.Printf("🔗 Relay: Connecting to hub at %s", pc.hubAddress)

	conn, err := grpc.NewClient(pc.hubAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	pc.conn = conn
	pc.audioClient = pb.NewAudioServiceClient(conn)
	pc.isConnected = true

	log.Printf("✅ Relay: Connected to hub successfully")
	return nil
}

// StreamAudio sends audio chunks to the hub and receives responses
func (pc *RelayClient) StreamAudio(audioChan <-chan audio.AudioChunk, responseChan chan<- *pb.AudioResponse) error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// Create bidirectional stream
	stream, err := pc.audioClient.StreamAudio(pc.ctx)
	if err != nil {
		return fmt.Errorf("failed to create audio stream: %w", err)
	}

	log.Println("🎙️  Relay: Audio streaming started")

	// Goroutine to send audio chunks
	go func() {
		defer func() {
			if err := stream.CloseSend(); err != nil {
				log.Printf("⚠️ Failed to close gRPC stream: %v", err)
			}
		}()

		for {
			select {
			case audioChunk, ok := <-audioChan:
				if !ok {
					log.Println("🎙️  Relay: Audio channel closed")
					return
				}

				// Convert audio chunk to protobuf message
				chunk := &pb.AudioChunk{
					RelayId:       pc.relayID,
					AudioData:     float32ArrayToBytes(audioChunk.Data),
					SampleRate:    audioChunk.SampleRate,
					Timestamp:     audioChunk.Timestamp,
					IsWakeWord:    audioChunk.IsWakeWord,
					IsEndOfSpeech: audioChunk.IsEndOfSpeech,
				}

				if err := stream.Send(chunk); err != nil {
					log.Printf("❌ Failed to send audio chunk: %v", err)
					return
				}

				log.Printf("📤 Relay: Sent audio chunk (%d samples)", len(audioChunk.Data))

			case <-pc.ctx.Done():
				return
			}
		}
	}()

	// Goroutine to receive responses
	go func() {
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				log.Println("🎙️  Relay: Audio stream ended")
				return
			}
			if err != nil {
				log.Printf("❌ Failed to receive response: %v", err)
				return
			}

			log.Printf("📥 Relay: Received response - Command: %s, Response: %s",
				response.Command, response.ResponseText)

			select {
			case responseChan <- response:
				// Successfully sent response
			default:
				log.Println("⚠️  Response channel full, dropping response")
			}
		}
	}()

	return nil
}

// PlayAudio receives TTS audio from hub and plays it
func (pc *RelayClient) PlayAudio(audioPlayer func([]float32) error) error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// This would be called when the hub wants to send TTS audio
	// For now, this is a placeholder for the TTS playback functionality
	log.Println("🔊 Relay: Ready to receive TTS audio from hub")

	return nil
}

// Disconnect closes the connection to the hub
func (pc *RelayClient) Disconnect() {
	if pc.conn != nil {
		if err := pc.conn.Close(); err != nil {
			log.Printf("⚠️ Failed to close gRPC connection: %v", err)
		}
		pc.isConnected = false
		log.Println("🔗 Relay: Disconnected from hub")
	}
	pc.cancel()
}

// Helper function to convert float32 array to bytes
func float32ArrayToBytes(data []float32) []byte {
	// Convert float32 samples to 16-bit PCM bytes
	bytes := make([]byte, len(data)*2)
	for i, sample := range data {
		// Convert from float32 [-1,1] to int16 [-32768,32767]
		val := int16(sample * 32767)
		bytes[i*2] = byte(val)
		bytes[i*2+1] = byte(val >> 8)
	}
	return bytes
}
