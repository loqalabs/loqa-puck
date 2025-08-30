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

	pb "github.com/loqalabs/loqa-proto/go"
	"github.com/loqalabs/loqa-puck/test-go/internal/audio"
)

// PuckClient handles gRPC communication with the hub
type PuckClient struct {
	conn          *grpc.ClientConn
	audioClient   pb.AudioServiceClient
	hubAddress    string
	puckID        string
	isConnected   bool
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewPuckClient creates a new gRPC client for the puck
func NewPuckClient(hubAddress, puckID string) *PuckClient {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &PuckClient{
		hubAddress: hubAddress,
		puckID:     puckID,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Connect establishes connection to the hub
func (pc *PuckClient) Connect() error {
	log.Printf("🔗 Puck: Connecting to hub at %s", pc.hubAddress)

	conn, err := grpc.NewClient(pc.hubAddress, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to hub: %w", err)
	}

	pc.conn = conn
	pc.audioClient = pb.NewAudioServiceClient(conn)
	pc.isConnected = true

	log.Printf("✅ Puck: Connected to hub successfully")
	return nil
}

// StreamAudio sends audio chunks to the hub and receives responses
func (pc *PuckClient) StreamAudio(audioChan <-chan audio.AudioChunk, responseChan chan<- *pb.AudioResponse) error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// Create bidirectional stream
	stream, err := pc.audioClient.StreamAudio(pc.ctx)
	if err != nil {
		return fmt.Errorf("failed to create audio stream: %w", err)
	}

	log.Println("🎙️  Puck: Audio streaming started")

	// Goroutine to send audio chunks
	go func() {
		defer stream.CloseSend()

		for {
			select {
			case audioChunk, ok := <-audioChan:
				if !ok {
					log.Println("🎙️  Puck: Audio channel closed")
					return
				}

				// Convert audio chunk to protobuf message
				chunk := &pb.AudioChunk{
					PuckId:        pc.puckID,
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

				log.Printf("📤 Puck: Sent audio chunk (%d samples)", len(audioChunk.Data))

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
				log.Println("🎙️  Puck: Audio stream ended")
				return
			}
			if err != nil {
				log.Printf("❌ Failed to receive response: %v", err)
				return
			}

			log.Printf("📥 Puck: Received response - Command: %s, Response: %s", 
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
func (pc *PuckClient) PlayAudio(audioPlayer func([]float32) error) error {
	if !pc.isConnected {
		return fmt.Errorf("not connected to hub")
	}

	// This would be called when the hub wants to send TTS audio
	// For now, this is a placeholder for the TTS playback functionality
	log.Println("🔊 Puck: Ready to receive TTS audio from hub")
	
	return nil
}

// Disconnect closes the connection to the hub
func (pc *PuckClient) Disconnect() {
	if pc.conn != nil {
		pc.conn.Close()
		pc.isConnected = false
		log.Println("🔗 Puck: Disconnected from hub")
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

