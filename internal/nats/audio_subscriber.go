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
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// AudioStreamMessage represents complete audio file from hub to puck
type AudioStreamMessage struct {
	StreamID    string `json:"stream_id"`    // Unique identifier for this audio stream
	AudioData   []byte `json:"audio_data"`   // Complete audio file data
	AudioFormat string `json:"audio_format"` // Format (e.g., "wav", "mp3")
	SampleRate  int    `json:"sample_rate"`  // Sample rate for audio data
	MessageType string `json:"message_type"` // "response", "timer", "reminder", "system"
	Priority    int    `json:"priority"`     // 1=highest, 5=lowest
}

// PuckNATSConnection interface for dependency injection
type PuckNATSConnection interface {
	Subscribe(subject string, cb nats.MsgHandler) (*nats.Subscription, error)
	Close()
}

// PuckNATSConnectionAdapter adapts *nats.Conn to PuckNATSConnection interface
type PuckNATSConnectionAdapter struct {
	conn *nats.Conn
}

func NewPuckNATSConnectionAdapter(conn *nats.Conn) *PuckNATSConnectionAdapter {
	return &PuckNATSConnectionAdapter{conn: conn}
}

func (r *PuckNATSConnectionAdapter) Subscribe(subject string, cb nats.MsgHandler) (*nats.Subscription, error) {
	return r.conn.Subscribe(subject, cb)
}

func (r *PuckNATSConnectionAdapter) Close() {
	r.conn.Close()
}

// AudioStreamManager manages audio playback queue (simplified for complete file delivery)
type AudioStreamManager struct {
	playbackCh chan []byte // Channel for immediate audio playback
	capacity   int
}

// NewAudioStreamManager creates a new audio stream manager
func NewAudioStreamManager(capacity int) *AudioStreamManager {
	return &AudioStreamManager{
		playbackCh: make(chan []byte, capacity),
		capacity:   capacity,
	}
}

// GetPlaybackChannel returns the channel for audio playback
func (asm *AudioStreamManager) GetPlaybackChannel() <-chan []byte {
	return asm.playbackCh
}

// AudioSubscriber handles NATS subscriptions for audio streams
type AudioSubscriber struct {
	natsConn      PuckNATSConnection
	puckID       string
	streamManager *AudioStreamManager
}

// NewAudioSubscriber creates a new NATS audio subscriber
func NewAudioSubscriber(natsURL, puckID string, streamCapacity int) (*AudioSubscriber, error) {
	// Connect to NATS with retry
	var nc *nats.Conn
	var err error

	for i := 0; i < 5; i++ {
		nc, err = nats.Connect(natsURL)
		if err == nil {
			break
		}
		log.Printf("⚠️  Failed to connect to NATS (attempt %d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS after 5 attempts: %w", err)
	}

	log.Printf("✅ Connected to NATS at %s", natsURL)

	return &AudioSubscriber{
		natsConn:      NewPuckNATSConnectionAdapter(nc),
		puckID:       puckID,
		streamManager: NewAudioStreamManager(streamCapacity),
	}, nil
}

// NewAudioSubscriberWithConnection creates a new NATS audio subscriber with an existing connection (for testing)
func NewAudioSubscriberWithConnection(natsConn PuckNATSConnection, puckID string, streamCapacity int) *AudioSubscriber {
	return &AudioSubscriber{
		natsConn:      natsConn,
		puckID:       puckID,
		streamManager: NewAudioStreamManager(streamCapacity),
	}
}

// Start begins listening for audio messages
func (as *AudioSubscriber) Start() error {
	// Subscribe to relay-specific audio topic
	puckTopic := fmt.Sprintf("audio.%s", as.puckID)
	_, err := as.natsConn.Subscribe(puckTopic, as.handleAudioMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", puckTopic, err)
	}

	// Subscribe to broadcast audio topic
	broadcastTopic := "audio.broadcast"
	_, err = as.natsConn.Subscribe(broadcastTopic, as.handleAudioMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", broadcastTopic, err)
	}

	log.Printf("🎧 Subscribed to audio topics: %s, %s", puckTopic, broadcastTopic)
	return nil
}

// handleAudioMessage processes incoming complete audio files from NATS
func (as *AudioSubscriber) handleAudioMessage(msg *nats.Msg) {
	var streamMsg AudioStreamMessage
	if err := json.Unmarshal(msg.Data, &streamMsg); err != nil {
		log.Printf("❌ Failed to unmarshal audio stream message: %v", err)
		return
	}

	log.Printf("📥 Received complete audio file: stream=%s, size=%d bytes, type=%s, format=%s",
		streamMsg.StreamID, len(streamMsg.AudioData), streamMsg.MessageType, streamMsg.AudioFormat)

	// Send complete audio file directly to playback channel
	select {
	case as.streamManager.playbackCh <- streamMsg.AudioData:
		log.Printf("🔊 Queued complete audio file for playback: %s", streamMsg.StreamID)
	default:
		log.Printf("⚠️  Playback channel full, dropping audio file: %s", streamMsg.StreamID)
	}
}

// GetStreamManager returns the audio stream manager for processing
func (as *AudioSubscriber) GetStreamManager() *AudioStreamManager {
	return as.streamManager
}

// Close closes the NATS connection
func (as *AudioSubscriber) Close() {
	if as.natsConn != nil {
		as.natsConn.Close()
		log.Println("🔌 NATS connection closed")
	}
}
