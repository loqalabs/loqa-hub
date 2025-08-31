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

package events

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unsafe"
)

// VoiceEvent represents a complete voice interaction event with full traceability
type VoiceEvent struct {
	// Core identification
	UUID      string    `json:"uuid" db:"uuid"`
	RequestID string    `json:"request_id" db:"request_id"`
	PuckID    string    `json:"puck_id" db:"puck_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`

	// Audio metadata
	AudioHash        string  `json:"audio_hash" db:"audio_hash"`
	AudioDuration    float64 `json:"audio_duration" db:"audio_duration"`
	SampleRate       int     `json:"sample_rate" db:"sample_rate"`
	WakeWordDetected bool    `json:"wake_word_detected" db:"wake_word_detected"`

	// Processing results
	Transcription string            `json:"transcription" db:"transcription"`
	Intent        string            `json:"intent" db:"intent"`
	Entities      map[string]string `json:"entities" db:"entities"`
	Confidence    float64           `json:"confidence" db:"confidence"`

	// Response data
	ResponseText   string `json:"response_text" db:"response_text"`
	ProcessingTime int64  `json:"processing_time_ms" db:"processing_time_ms"`
	Success        bool   `json:"success" db:"success"`
	ErrorMessage   string `json:"error_message,omitempty" db:"error_message"`
}

// NewVoiceEvent creates a new VoiceEvent with generated UUID and current timestamp
func NewVoiceEvent(puckID, requestID string) *VoiceEvent {
	return &VoiceEvent{
		UUID:      generateUUID(),
		RequestID: requestID,
		PuckID:    puckID,
		Timestamp: time.Now(),
		Entities:  make(map[string]string),
		Success:   true,
	}
}

// generateUUID generates a simple UUID without external dependencies
func generateUUID() string {
	b := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, b)
	if err != nil {
		// Fallback to timestamp-based ID if random fails
		return fmt.Sprintf("loqa-%d", time.Now().UnixNano())
	}
	
	// Set version (4) and variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// SetAudioMetadata sets audio-related metadata for the event
func (ve *VoiceEvent) SetAudioMetadata(audioData []float32, sampleRate int, isWakeWord bool) {
	ve.AudioHash = ve.calculateAudioHash(audioData)
	ve.AudioDuration = float64(len(audioData)) / float64(sampleRate)
	ve.SampleRate = sampleRate
	ve.WakeWordDetected = isWakeWord
}

// SetTranscription sets the transcription result
func (ve *VoiceEvent) SetTranscription(transcription string) {
	ve.Transcription = transcription
}

// SetCommandResult sets the command parsing results
func (ve *VoiceEvent) SetCommandResult(intent string, entities map[string]string, confidence float64) {
	ve.Intent = intent
	ve.Entities = entities
	ve.Confidence = confidence
}

// SetResponse sets the response text and marks processing as complete
func (ve *VoiceEvent) SetResponse(responseText string) {
	ve.ResponseText = responseText
	ve.ProcessingTime = time.Since(ve.Timestamp).Milliseconds()
}

// SetError marks the event as failed with an error message
func (ve *VoiceEvent) SetError(err error) {
	ve.Success = false
	ve.ErrorMessage = err.Error()
	ve.ProcessingTime = time.Since(ve.Timestamp).Milliseconds()
}

// calculateAudioHash generates a SHA-256 hash of the audio data for duplicate detection
func (ve *VoiceEvent) calculateAudioHash(audioData []float32) string {
	hasher := sha256.New()
	
	// Convert float32 slice to bytes for hashing
	for _, sample := range audioData {
		bytes := (*[4]byte)(unsafe.Pointer(&sample))[:]
		hasher.Write(bytes)
	}
	
	return hex.EncodeToString(hasher.Sum(nil))
}

// EntitiesJSON returns entities as JSON string for database storage
func (ve *VoiceEvent) EntitiesJSON() (string, error) {
	if ve.Entities == nil {
		return "{}", nil
	}
	
	data, err := json.Marshal(ve.Entities)
	if err != nil {
		return "", fmt.Errorf("failed to marshal entities: %w", err)
	}
	
	return string(data), nil
}

// SetEntitiesFromJSON parses JSON string and sets entities
func (ve *VoiceEvent) SetEntitiesFromJSON(jsonStr string) error {
	if jsonStr == "" || jsonStr == "{}" {
		ve.Entities = make(map[string]string)
		return nil
	}
	
	var entities map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &entities); err != nil {
		return fmt.Errorf("failed to unmarshal entities JSON: %w", err)
	}
	
	ve.Entities = entities
	return nil
}

// IsValid performs basic validation on the voice event
func (ve *VoiceEvent) IsValid() error {
	if ve.UUID == "" {
		return fmt.Errorf("UUID is required")
	}
	
	if ve.PuckID == "" {
		return fmt.Errorf("puckID is required")
	}
	
	if ve.RequestID == "" {
		return fmt.Errorf("requestID is required")
	}
	
	if ve.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	
	if ve.Confidence < 0 || ve.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	
	return nil
}

// String returns a human-readable representation of the voice event
func (ve *VoiceEvent) String() string {
	return fmt.Sprintf("VoiceEvent{UUID: %s, PuckID: %s, Intent: %s, Transcription: %q, Confidence: %.2f, Success: %t}",
		ve.UUID, ve.PuckID, ve.Intent, ve.Transcription, ve.Confidence, ve.Success)
}