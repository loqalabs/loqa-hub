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

package messaging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// AudioStreamMessage represents complete audio file for NATS delivery
type AudioStreamMessage struct {
	StreamID     string `json:"stream_id"`      // Unique identifier for this audio stream
	AudioData    []byte `json:"audio_data"`     // Complete audio file data
	AudioFormat  string `json:"audio_format"`   // Format (e.g., "wav", "mp3")
	SampleRate   int    `json:"sample_rate"`    // Sample rate for audio data
	MessageType  string `json:"message_type"`   // "response", "timer", "reminder", "system"
	Priority     int    `json:"priority"`       // 1=highest, 5=lowest
}

// AudioStreamPublisher handles publishing chunked audio streams via NATS
type AudioStreamPublisher struct {
	natsConn   *nats.Conn
	chunkSize  int           // Size of each audio chunk in bytes
	maxStreams int           // Maximum concurrent streams
	timeout    time.Duration // Timeout for publishing chunks
}

// NewAudioStreamPublisher creates a new audio stream publisher
func NewAudioStreamPublisher(natsConn *nats.Conn, chunkSize int) *AudioStreamPublisher {
	return &AudioStreamPublisher{
		natsConn:   natsConn,
		chunkSize:  chunkSize,
		maxStreams: 50,
		timeout:    5 * time.Second,
	}
}

// StreamAudioToRelay publishes complete audio file to a specific relay
func (asp *AudioStreamPublisher) StreamAudioToRelay(
	relayID string,
	audioReader io.Reader,
	audioFormat string,
	sampleRate int,
	messageType string,
	priority int,
) error {
	// Generate unique stream ID
	streamID := fmt.Sprintf("%s-%d", uuid.New().String()[:8], time.Now().UnixNano())

	log.Printf("🎵 Sending complete audio file: %s to relay %s (type: %s)", streamID, relayID, messageType)

	// Read complete audio file
	audioData, err := io.ReadAll(audioReader)
	if err != nil {
		return fmt.Errorf("failed to read complete audio data: %w", err)
	}

	log.Printf("🎵 Audio file size: %d bytes", len(audioData))

	// NATS topic for this relay
	topic := fmt.Sprintf("audio.%s", relayID)

	// Create complete audio file message
	streamMsg := AudioStreamMessage{
		StreamID:    streamID,
		AudioData:   audioData,
		AudioFormat: audioFormat,
		SampleRate:  sampleRate,
		MessageType: messageType,
		Priority:    priority,
	}

	// Serialize message
	msgData, err := json.Marshal(streamMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal audio message: %w", err)
	}

	// Publish complete audio file via NATS
	if err := asp.natsConn.Publish(topic, msgData); err != nil {
		return fmt.Errorf("failed to publish audio file: %w", err)
	}

	log.Printf("📤 Published complete audio file to %s (size: %d bytes)",
		topic, len(audioData))

	log.Printf("✅ Completed audio delivery %s: %d total bytes",
		streamID, len(audioData))

	return nil
}

// BroadcastAudioToAllRelays publishes audio stream to all relays (for system announcements)
func (asp *AudioStreamPublisher) BroadcastAudioToAllRelays(
	audioReader io.Reader,
	audioFormat string,
	sampleRate int,
	messageType string,
	priority int,
) error {
	// Generate unique stream ID
	streamID := fmt.Sprintf("broadcast-%s-%d", uuid.New().String()[:8], time.Now().UnixNano())

	log.Printf("📢 Starting broadcast audio stream: %s (type: %s)", streamID, messageType)

	// Read all audio data first (needed for broadcast)
	audioData, err := io.ReadAll(audioReader)
	if err != nil {
		return fmt.Errorf("failed to read audio data for broadcast: %w", err)
	}

	// NATS topic for broadcast
	topic := "audio.broadcast"

	// Create complete audio file message for broadcast
	streamMsg := AudioStreamMessage{
		StreamID:    streamID,
		AudioData:   audioData,
		AudioFormat: audioFormat,
		SampleRate:  sampleRate,
		MessageType: messageType,
		Priority:    priority,
	}

	// Serialize message
	msgData, err := json.Marshal(streamMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %w", err)
	}

	// Publish complete audio file via NATS
	if err := asp.natsConn.Publish(topic, msgData); err != nil {
		return fmt.Errorf("failed to publish broadcast audio file: %w", err)
	}

	log.Printf("📢 Broadcast complete audio file (size: %d bytes)", len(audioData))

	log.Printf("✅ Completed broadcast delivery %s: %d total bytes",
		streamID, len(audioData))

	return nil
}