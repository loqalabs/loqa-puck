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

	"github.com/loqalabs/loqa-relay/test-go/internal/audio"
	"github.com/loqalabs/loqa-relay/test-go/internal/grpc"
	"github.com/loqalabs/loqa-relay/test-go/internal/nats"
)

// bytesToFloat32Array converts audio bytes (WAV format or raw 16-bit PCM) back to float32 samples
func bytesToFloat32Array(data []byte) []float32 {
	// Skip WAV header if present (check for "RIFF" signature)
	pcmData := data
	if len(data) >= 44 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		// WAV file detected, skip 44-byte header
		pcmData = data[44:]
	}

	if len(pcmData)%2 != 0 {
		// Ensure even number of bytes for 16-bit samples
		pcmData = pcmData[:len(pcmData)-1]
	}

	samples := make([]float32, len(pcmData)/2)
	for i := 0; i < len(samples); i++ {
		// Convert from 16-bit PCM to float32 (little-endian)
		low := int16(pcmData[i*2])
		high := int16(pcmData[i*2+1])
		val := low | (high << 8)
		samples[i] = float32(val) / 32767.0
	}
	return samples
}

func main() {
	// Command line flags
	hubAddr := flag.String("hub", "localhost:50051", "Hub gRPC address")
	relayID := flag.String("id", "test-relay-001", "Relay identifier")
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS server URL")
	flag.Parse()

	log.Printf("🚀 Starting Loqa Test Relay Service")
	log.Printf("📋 Relay ID: %s", *relayID)
	log.Printf("🎯 Hub Address: %s", *hubAddr)
	log.Printf("📨 NATS URL: %s", *natsURL)

	// Initialize audio system
	relayAudio, err := audio.NewRelayAudio()
	if err != nil {
		log.Fatalf("❌ Failed to initialize audio: %v", err)
	}
	defer relayAudio.Shutdown()

	// Initialize NATS audio subscriber
	audioSubscriber, err := nats.NewAudioSubscriber(*natsURL, *relayID, 10)
	if err != nil {
		log.Fatalf("❌ Failed to initialize NATS audio subscriber: %v", err)
	}
	defer audioSubscriber.Close()

	// Start listening for audio messages
	if err := audioSubscriber.Start(); err != nil {
		log.Fatalf("❌ Failed to start NATS audio subscriber: %v", err)
	}

	// Initialize gRPC client
	client := grpc.NewRelayClient(*hubAddr, *relayID)

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

	// Create channel for audio upload (fire-and-forget)
	audioChan := make(chan audio.AudioChunk, 10)

	// Start audio streaming to hub (upload only)
	if err := client.StreamAudio(audioChan, nil); err != nil {
		log.Fatalf("❌ Failed to start audio streaming: %v", err)
	}

	// Start audio recording
	if err := relayAudio.StartRecording(audioChan); err != nil {
		log.Fatalf("❌ Failed to start recording: %v", err)
	}

	// Handle streaming audio from NATS
	streamManager := audioSubscriber.GetStreamManager()
	go func() {
		for audioChunk := range streamManager.GetPlaybackChannel() {
			log.Printf("🔊 Playing complete audio file (%d bytes)", len(audioChunk))

			// Convert audio bytes to float32 samples for playback
			audioData := bytesToFloat32Array(audioChunk)
			if err := relayAudio.PlayAudio(audioData); err != nil {
				log.Printf("❌ Failed to play audio chunk: %v", err)
			}
		}
	}()

	// Display status
	fmt.Println()
	fmt.Println("🎤 Loqa Test Relay - Streaming Audio Interface Active!")
	fmt.Println("======================================================")
	fmt.Println()
	fmt.Println("🎙️  Microphone: Listening for wake word")
	fmt.Println("🎯 Wake Word: \"Hey Loqa\" (enabled)")
	fmt.Println("🔊 Speakers: Ready for streaming audio playback")
	fmt.Println("📡 Audio Upload: Fire-and-forget via gRPC")
	fmt.Println("📨 Audio Download: Streaming chunks via NATS")
	fmt.Println()
	fmt.Println("💡 Say \"Hey Loqa\" followed by your command!")
	fmt.Println("⚡ Audio responses will stream immediately when ready")
	fmt.Println("⏹️  Press Ctrl+C to stop")
	fmt.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("\n🛑 Shutting down relay service...")

	// Stop recording
	relayAudio.StopRecording()

	// Close channels
	close(audioChan)

	log.Println("👋 Relay service stopped")
}
