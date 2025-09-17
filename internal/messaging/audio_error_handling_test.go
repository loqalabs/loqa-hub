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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
)

// TestAudioStreamPublisher_ErrorScenarios tests various error conditions
func TestAudioStreamPublisher_ErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		setupError    func(*MockNATSConnection)
		relayID       string
		audioData     []byte
		expectError   bool
		errorContains string
	}{
		{
			name:        "connection_closed",
			relayID:     "relay-closed",
			audioData:   []byte("test-data"),
			expectError: true,
			setupError: func(conn *MockNATSConnection) {
				conn.Disconnect()
			},
		},
		{
			name:          "empty_relay_id",
			relayID:       "",
			audioData:     []byte("test-data"),
			expectError:   false, // Should work with empty relay ID
		},
		{
			name:        "nil_audio_data",
			relayID:     "relay-nil",
			audioData:   nil,
			expectError: false, // Should handle nil gracefully
		},
		{
			name:        "very_large_audio",
			relayID:     "relay-large",
			audioData:   make([]byte, 100*1024*1024), // 100MB
			expectError: false, // Should handle large data
		},
		{
			name:        "special_characters_relay_id",
			relayID:     "relay-特殊字符-émojis-🎵",
			audioData:   []byte("test-data"),
			expectError: false,
		},
		{
			name:        "nats_publish_timeout",
			relayID:     "relay-timeout",
			audioData:   []byte("test-data"),
			expectError: true,
			setupError: func(conn *MockNATSConnection) {
				// Simulate a timeout by setting a slow error
				conn.SetError("audio.relay-timeout", context.DeadlineExceeded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testable := NewAudioStreamPublisherTestable()

			if tt.setupError != nil {
				tt.setupError(testable.mockConn)
			}

			err := testable.asp.StreamAudioToRelay(
				tt.relayID,
				bytes.NewReader(tt.audioData),
				"wav",
				22050,
				"response",
				1,
			)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.errorContains != "" && err != nil {
				if !contains(err.Error(), tt.errorContains) {
					t.Errorf("Error message doesn't contain expected text. Got: %s, Want: %s",
						err.Error(), tt.errorContains)
				}
			}
		})
	}
}

func TestAudioStreamPublisher_NetworkResilience(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	// Test connection recovery
	t.Run("connection_recovery", func(t *testing.T) {
		// First, disconnect
		testable.mockConn.Disconnect()

		err := testable.asp.StreamAudioToRelay(
			"relay-recovery",
			bytes.NewReader([]byte("test-data")),
			"wav",
			22050,
			"response",
			1,
		)

		if err == nil {
			t.Error("Expected error when disconnected")
		}

		// Reconnect
		testable.mockConn.Reconnect()

		err = testable.asp.StreamAudioToRelay(
			"relay-recovery",
			bytes.NewReader([]byte("test-data")),
			"wav",
			22050,
			"response",
			1,
		)

		if err != nil {
			t.Errorf("Unexpected error after reconnection: %v", err)
		}
	})

	// Test intermittent failures
	t.Run("intermittent_failures", func(t *testing.T) {
		subject := "audio.stream.relay-intermittent"
		successCount := 0
		errorCount := 0

		for i := 0; i < 10; i++ {
			// Alternate between success and failure
			if i%2 == 0 {
				testable.mockConn.SetError(subject, nats.ErrConnectionClosed)
			} else {
				// Remove error for success
				testable.mockConn.SetError(subject, nil)
			}

			err := testable.asp.StreamAudioToRelay(
				"relay-intermittent",
				bytes.NewReader([]byte("test-data")),
				"wav",
				22050,
				"response",
				1,
			)

			if err != nil {
				errorCount++
			} else {
				successCount++
			}
		}

		if successCount == 0 {
			t.Error("No successful operations in intermittent test")
		}

		if errorCount == 0 {
			t.Error("No failed operations in intermittent test")
		}

		t.Logf("Intermittent test results: %d successes, %d errors", successCount, errorCount)
	})
}

func TestAudioStreamPublisher_MemoryHandling(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	// Test memory pressure scenarios
	tests := []struct {
		name      string
		audioSize int
	}{
		{
			name:      "small_audio_1kb",
			audioSize: 1024,
		},
		{
			name:      "medium_audio_1mb",
			audioSize: 1024 * 1024,
		},
		{
			name:      "large_audio_10mb",
			audioSize: 10 * 1024 * 1024,
		},
		{
			name:      "very_large_audio_50mb",
			audioSize: 50 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audioData := make([]byte, tt.audioSize)

			// Fill with pattern for verification
			for i := range audioData {
				audioData[i] = byte(i % 256)
			}

			err := testable.asp.StreamAudioToRelay(
				"relay-memory",
				bytes.NewReader(audioData),
				"wav",
				22050,
				"response",
				1,
			)

			if err != nil {
				t.Errorf("Memory handling test failed for size %d: %v", tt.audioSize, err)
			}

			// Verify data integrity
			publishedData := testable.mockConn.GetPublishedMessage("audio.stream.relay-memory")
			if publishedData == nil {
				t.Fatal("No message published for memory test")
			}

			// Decode and verify
			var decodedMsg AudioStreamMessage
			err = json.Unmarshal(publishedData, &decodedMsg)
			if err != nil {
				t.Fatalf("Failed to decode memory test message: %v", err)
			}

			if len(decodedMsg.AudioData) != tt.audioSize {
				t.Errorf("Audio data size mismatch: got %d, want %d",
					len(decodedMsg.AudioData), tt.audioSize)
			}

			// Verify first and last bytes (pattern check)
			if len(decodedMsg.AudioData) > 0 {
				if decodedMsg.AudioData[0] != 0 {
					t.Error("First byte pattern mismatch")
				}
				if len(decodedMsg.AudioData) > 255 && decodedMsg.AudioData[255] != 255 {
					t.Error("Pattern verification failed at position 255")
				}
			}
		})
	}
}

func TestAudioStreamPublisher_RaceConditions(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	// Test concurrent access to the same relay
	t.Run("concurrent_same_relay", func(t *testing.T) {
		numGoroutines := 20
		relayID := "relay-race"
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				audioData := []byte(fmt.Sprintf("race-data-%d", id))
				err := testable.asp.StreamAudioToRelay(
					relayID,
					bytes.NewReader(audioData),
					"wav",
					22050,
					"response",
					1,
				)

				if err != nil {
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for race condition errors
		for err := range errors {
			t.Errorf("Race condition error: %v", err)
		}
	})

	// Test concurrent access to different relays
	t.Run("concurrent_different_relays", func(t *testing.T) {
		numGoroutines := 10
		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				relayID := fmt.Sprintf("relay-race-%d", id)
				audioData := []byte(fmt.Sprintf("data-%d", id))

				err := testable.asp.StreamAudioToRelay(
					relayID,
					bytes.NewReader(audioData),
					"wav",
					22050,
					"response",
					1,
				)

				if err != nil {
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("Concurrent relay error: %v", err)
		}
	})
}

func TestAudioStreamPublisher_InvalidInputValidation(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	tests := []struct {
		name        string
		relayID     string
		audioData   []byte
		audioFormat string
		sampleRate  int
		messageType string
		priority    int
		expectError bool
	}{
		{
			name:        "negative_sample_rate",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  -1000,
			messageType: "response",
			priority:    1,
			expectError: false, // Current implementation doesn't validate
		},
		{
			name:        "zero_sample_rate",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  0,
			messageType: "response",
			priority:    1,
			expectError: false,
		},
		{
			name:        "very_high_sample_rate",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  192000,
			messageType: "response",
			priority:    1,
			expectError: false,
		},
		{
			name:        "invalid_audio_format",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "invalid-format",
			sampleRate:  22050,
			messageType: "response",
			priority:    1,
			expectError: false,
		},
		{
			name:        "empty_audio_format",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "",
			sampleRate:  22050,
			messageType: "response",
			priority:    1,
			expectError: false,
		},
		{
			name:        "invalid_priority_negative",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  22050,
			messageType: "response",
			priority:    -1,
			expectError: false,
		},
		{
			name:        "invalid_priority_too_high",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  22050,
			messageType: "response",
			priority:    999,
			expectError: false,
		},
		{
			name:        "empty_message_type",
			relayID:     "relay-test",
			audioData:   []byte("test"),
			audioFormat: "wav",
			sampleRate:  22050,
			messageType: "",
			priority:    1,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testable.asp.StreamAudioToRelay(
				tt.relayID,
				bytes.NewReader(tt.audioData),
				tt.audioFormat,
				tt.sampleRate,
				tt.messageType,
				tt.priority,
			)

			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

func TestAudioStreamPublisher_ContextCancellation(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	// Test behavior when context is cancelled
	t.Run("context_cancellation", func(t *testing.T) {
		// This test would be more meaningful if the actual implementation
		// supported context cancellation. For now, we test basic behavior.

		audioData := []byte("context-test-data")

		err := testable.asp.StreamAudioToRelay(
			"relay-context",
			bytes.NewReader(audioData),
			"wav",
			22050,
			"response",
			1,
		)

		if err != nil {
			t.Errorf("Basic operation failed: %v", err)
		}
	})
}

func TestAudioStreamPublisher_BroadcastErrorHandling(t *testing.T) {
	testable := NewAudioStreamPublisherTestable()

	// Test broadcast-specific error scenarios
	tests := []struct {
		name        string
		setupError  func(*MockNATSConnection)
		expectError bool
	}{
		{
			name:        "broadcast_connection_closed",
			expectError: true,
			setupError: func(conn *MockNATSConnection) {
				conn.SetError("audio.broadcast", nats.ErrConnectionClosed)
			},
		},
		{
			name:        "broadcast_permission_denied",
			expectError: true,
			setupError: func(conn *MockNATSConnection) {
				conn.SetError("audio.broadcast", nats.ErrAuthorization)
			},
		},
		{
			name:        "broadcast_normal_operation",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupError != nil {
				tt.setupError(testable.mockConn)
			}

			err := testable.asp.BroadcastAudioToAllRelays(
				bytes.NewReader([]byte("broadcast-test")),
				"wav",
				22050,
				"system",
				1,
			)

			if tt.expectError && err == nil {
				t.Error("Expected broadcast error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected broadcast error: %v", err)
			}
		})
	}
}

func TestAudioStreamPublisher_SerializationErrors(t *testing.T) {
	// Test edge cases in JSON serialization
	tests := []struct {
		name      string
		audioData []byte
	}{
		{
			name:      "binary_data_with_null_bytes",
			audioData: []byte{0x00, 0x01, 0x02, 0x03, 0x00, 0xFF},
		},
		{
			name:      "audio_with_unicode_bytes",
			audioData: []byte("Hello 世界 🎵"),
		},
		{
			name:      "audio_with_control_characters",
			audioData: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x1F},
		},
		{
			name:      "very_long_audio_data",
			audioData: make([]byte, 1000000), // 1MB of zeros
		},
	}

	testable := NewAudioStreamPublisherTestable()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testable.asp.StreamAudioToRelay(
				"relay-serialization",
				bytes.NewReader(tt.audioData),
				"wav",
				22050,
				"response",
				1,
			)

			if err != nil {
				t.Errorf("Serialization test failed: %v", err)
			}

			// Verify the message can be deserialized
			publishedData := testable.mockConn.GetPublishedMessage("audio.stream.relay-serialization")
			if publishedData != nil {
				var decodedMsg AudioStreamMessage
				err = json.Unmarshal(publishedData, &decodedMsg)
				if err != nil {
					t.Errorf("Failed to deserialize message: %v", err)
				} else {
					// Verify data integrity
					if len(decodedMsg.AudioData) != len(tt.audioData) {
						t.Errorf("Audio data length mismatch after serialization: got %d, want %d",
							len(decodedMsg.AudioData), len(tt.audioData))
					}
				}
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		   (len(substr) == 0 ||
		    findIndex(s, substr) >= 0)
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}