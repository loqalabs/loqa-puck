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
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/loqalabs/loqa-puck/test-go/internal/audio"
	"github.com/loqalabs/loqa-puck/test-go/internal/grpc"
	pb "github.com/loqalabs/loqa-proto/go"
)

func main() {
	// Command line flags
	hubAddr := flag.String("hub", "localhost:50051", "Hub gRPC address")
	puckID := flag.String("id", "test-puck-001", "Puck identifier")
	flag.Parse()

	log.Printf("🚀 Starting Loqa Test Puck Service")
	log.Printf("📋 Puck ID: %s", *puckID)
	log.Printf("🎯 Hub Address: %s", *hubAddr)

	// Initialize audio system
	puckAudio, err := audio.NewPuckAudio()
	if err != nil {
		log.Fatalf("❌ Failed to initialize audio: %v", err)
	}
	defer puckAudio.Shutdown()

	// Initialize gRPC client
	client := grpc.NewPuckClient(*hubAddr, *puckID)

	// Connect to hub with retry
	for i := 0; i < 5; i++ {
		if err := client.Connect(); err != nil {
			log.Printf("⚠️  Connection attempt %d failed: %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}
		break
	}
	defer client.Disconnect()

	// Create channels for audio streaming
	audioChan := make(chan audio.AudioChunk, 10)
	responseChan := make(chan *pb.AudioResponse, 10)

	// Start audio streaming to hub
	if err := client.StreamAudio(audioChan, responseChan); err != nil {
		log.Fatalf("❌ Failed to start audio streaming: %v", err)
	}

	// Start audio recording
	if err := puckAudio.StartRecording(audioChan); err != nil {
		log.Fatalf("❌ Failed to start recording: %v", err)
	}

	// Handle responses from hub
	go func() {
		for response := range responseChan {
			log.Printf("🎤 Heard: \"%s\"", response.Transcription)
			log.Printf("⚡ Command: %s", response.Command)
			log.Printf("💬 Response: %s", response.ResponseText)

			// TODO: Convert response text to audio and play it
			if response.ResponseText != "" {
				log.Printf("🔊 Would play TTS: \"%s\"", response.ResponseText)
			}
		}
	}()

	// Display status
	fmt.Println()
	fmt.Println("🎤 Loqa Test Puck - Audio Interface Active!")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("🎙️  Microphone: Listening for wake word")
	fmt.Println("🎯 Wake Word: \"Hey Loqa\" (enabled)")
	fmt.Println("🔊 Speakers: Ready for audio playback")
	fmt.Println("📡 Hub Connection: Streaming audio via gRPC")
	fmt.Println()
	fmt.Println("💡 Say \"Hey Loqa\" followed by your command!")
	fmt.Println("⏹️  Press Ctrl+C to stop")
	fmt.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("\n🛑 Shutting down puck service...")
	
	// Stop recording
	puckAudio.StopRecording()
	
	// Close channels
	close(audioChan)
	close(responseChan)

	log.Println("👋 Puck service stopped")
}