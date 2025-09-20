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
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFrameSerialization(t *testing.T) {
	tests := []struct {
		name  string
		frame *Frame
	}{
		{
			name: "Empty frame",
			frame: &Frame{
				Type:      FrameTypeHeartbeat,
				SessionID: 12345,
				Sequence:  1,
				Timestamp: 1640995200000000, // 2022-01-01 00:00:00 UTC in microseconds
				Data:      nil,
			},
		},
		{
			name: "Frame with small data",
			frame: &Frame{
				Type:      FrameTypeAudioData,
				SessionID: 67890,
				Sequence:  42,
				Timestamp: 1640995200123456,
				Data:      []byte("hello world"),
			},
		},
		{
			name: "Frame with maximum data size",
			frame: &Frame{
				Type:      FrameTypeWakeWord,
				SessionID: 99999,
				Sequence:  999,
				Timestamp: 1640995299999999,
				Data:      make([]byte, MaxDataSize), // 4072 bytes
			},
		},
		{
			name: "All frame types",
			frame: &Frame{
				Type:      FrameTypeArbitration,
				SessionID: 11111,
				Sequence:  5,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      []byte(`{"test": "data"}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			serialized, err := tt.frame.Serialize()
			if err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}

			// Verify frame structure
			if len(serialized) != HeaderSize+len(tt.frame.Data) {
				t.Errorf("Serialized frame size = %d, want %d", len(serialized), HeaderSize+len(tt.frame.Data))
			}

			// Test deserialization
			deserialized, err := DeserializeFrame(serialized)
			if err != nil {
				t.Fatalf("DeserializeFrame() error = %v", err)
			}

			// Verify all fields match
			if deserialized.Type != tt.frame.Type {
				t.Errorf("Type = %v, want %v", deserialized.Type, tt.frame.Type)
			}
			if deserialized.SessionID != tt.frame.SessionID {
				t.Errorf("SessionID = %d, want %d", deserialized.SessionID, tt.frame.SessionID)
			}
			if deserialized.Sequence != tt.frame.Sequence {
				t.Errorf("Sequence = %d, want %d", deserialized.Sequence, tt.frame.Sequence)
			}
			if deserialized.Timestamp != tt.frame.Timestamp {
				t.Errorf("Timestamp = %d, want %d", deserialized.Timestamp, tt.frame.Timestamp)
			}
			if !bytes.Equal(deserialized.Data, tt.frame.Data) {
				t.Errorf("Data mismatch. Got %v, want %v", deserialized.Data, tt.frame.Data)
			}
		})
	}
}

func TestFrameValidation(t *testing.T) {
	tests := []struct {
		name        string
		frame       *Frame
		expectValid bool
	}{
		{
			name: "Valid frame",
			frame: &Frame{
				Type:      FrameTypeAudioData,
				SessionID: 1,
				Sequence:  1,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      []byte("test"),
			},
			expectValid: true,
		},
		{
			name: "Frame with maximum data size",
			frame: &Frame{
				Type:      FrameTypeAudioData,
				SessionID: 1,
				Sequence:  1,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      make([]byte, MaxDataSize),
			},
			expectValid: true,
		},
		{
			name: "Frame exceeding maximum data size",
			frame: &Frame{
				Type:      FrameTypeAudioData,
				SessionID: 1,
				Sequence:  1,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      make([]byte, MaxDataSize+1),
			},
			expectValid: false,
		},
		{
			name: "Empty data frame",
			frame: &Frame{
				Type:      FrameTypeHeartbeat,
				SessionID: 1,
				Sequence:  1,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      nil,
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.frame.IsValid()
			if isValid != tt.expectValid {
				t.Errorf("IsValid() = %v, want %v", isValid, tt.expectValid)
			}

			// Test serialization behavior
			_, err := tt.frame.Serialize()
			hasError := err != nil
			if tt.expectValid && hasError {
				t.Errorf("Expected valid frame but got serialization error: %v", err)
			}
			if !tt.expectValid && !hasError {
				t.Errorf("Expected invalid frame but serialization succeeded")
			}
		})
	}
}

func TestFrameDeserialization_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectError bool
		errorType   string
	}{
		{
			name:        "Too small data",
			data:        make([]byte, HeaderSize-1),
			expectError: true,
			errorType:   "frame too small",
		},
		{
			name:        "Invalid magic number",
			data:        make([]byte, HeaderSize),
			expectError: true,
			errorType:   "invalid frame magic",
		},
		{
			name: "Size mismatch",
			data: func() []byte {
				frame := &Frame{
					Type:      FrameTypeHeartbeat,
					SessionID: 1,
					Sequence:  1,
					Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
					Data:      []byte("test"),
				}
				data, _ := frame.Serialize()
				// Truncate to create size mismatch
				return data[:len(data)-1]
			}(),
			expectError: true,
			errorType:   "frame size mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeserializeFrame(tt.data)
			if !tt.expectError {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing '%s' but got no error", tt.errorType)
				}
			}
		})
	}
}

func TestFrameSize(t *testing.T) {
	tests := []struct {
		name         string
		dataSize     int
		expectedSize int
	}{
		{"Empty frame", 0, HeaderSize},
		{"Small data", 10, HeaderSize + 10},
		{"Large data", 1000, HeaderSize + 1000},
		{"Maximum data", MaxDataSize, HeaderSize + MaxDataSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := &Frame{
				Type:      FrameTypeAudioData,
				SessionID: 1,
				Sequence:  1,
				Timestamp: uint64(time.Now().UnixMicro()), //nolint:gosec // G115: Safe conversion for test timestamp
				Data:      make([]byte, tt.dataSize),
			}

			size := frame.Size()
			if size != tt.expectedSize {
				t.Errorf("Size() = %d, want %d", size, tt.expectedSize)
			}
		})
	}
}

func TestNewFrame(t *testing.T) {
	frameType := FrameTypeWakeWord
	sessionID := uint32(12345)
	sequence := uint32(67)
	timestamp := uint64(1640995200000000)
	data := []byte("test data")

	frame := NewFrame(frameType, sessionID, sequence, timestamp, data)

	if frame.Type != frameType {
		t.Errorf("Type = %v, want %v", frame.Type, frameType)
	}
	if frame.SessionID != sessionID {
		t.Errorf("SessionID = %d, want %d", frame.SessionID, sessionID)
	}
	if frame.Sequence != sequence {
		t.Errorf("Sequence = %d, want %d", frame.Sequence, sequence)
	}
	if frame.Timestamp != timestamp {
		t.Errorf("Timestamp = %d, want %d", frame.Timestamp, timestamp)
	}
	if !bytes.Equal(frame.Data, data) {
		t.Errorf("Data = %v, want %v", frame.Data, data)
	}
}

func TestFrameTypes(t *testing.T) {
	// Test all defined frame types
	frameTypes := []FrameType{
		FrameTypeAudioData,
		FrameTypeAudioEnd,
		FrameTypeWakeWord,
		FrameTypeHeartbeat,
		FrameTypeHandshake,
		FrameTypeError,
		FrameTypeArbitration,
		FrameTypeResponse,
		FrameTypeStatus,
	}

	for _, frameType := range frameTypes {
		t.Run(string(rune(frameType)), func(t *testing.T) {
			frame := NewFrame(frameType, 1, 1, uint64(time.Now().UnixMicro()), []byte("test")) //nolint:gosec // G115: Safe conversion for test timestamp

			serialized, err := frame.Serialize()
			if err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}

			deserialized, err := DeserializeFrame(serialized)
			if err != nil {
				t.Fatalf("DeserializeFrame() error = %v", err)
			}

			if deserialized.Type != frameType {
				t.Errorf("Frame type = %v, want %v", deserialized.Type, frameType)
			}
		})
	}
}

func TestFrameConstants(t *testing.T) {
	// Test frame constants are reasonable
	if FrameMagic != 0x4C4F5141 {
		t.Errorf("FrameMagic = 0x%08X, want 0x4C4F5141", FrameMagic)
	}

	if MaxFrameSize != 1536 {
		t.Errorf("MaxFrameSize = %d, want 1536", MaxFrameSize)
	}

	if HeaderSize != 24 {
		t.Errorf("HeaderSize = %d, want 24", HeaderSize)
	}

	if MaxDataSize != MaxFrameSize-HeaderSize {
		t.Errorf("MaxDataSize = %d, want %d", MaxDataSize, MaxFrameSize-HeaderSize)
	}
}

func TestFrameRoundTrip_LargeData(t *testing.T) {
	// Test with various data sizes to ensure no corruption
	sizes := []int{0, 1, 100, 1000, 1512, MaxDataSize}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			// Create test data with pattern
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			frame := NewFrame(FrameTypeAudioData, 123, 456, uint64(time.Now().UnixMicro()), data) //nolint:gosec // G115: Safe conversion for test timestamp

			serialized, err := frame.Serialize()
			if err != nil {
				t.Fatalf("Serialize() error = %v", err)
			}

			deserialized, err := DeserializeFrame(serialized)
			if err != nil {
				t.Fatalf("DeserializeFrame() error = %v", err)
			}

			if !bytes.Equal(deserialized.Data, data) {
				t.Errorf("Data corruption detected for size %d", size)
			}
		})
	}
}

func TestParseFrameHeader(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() []byte
		wantErr   bool
		errMsg    string
		validate  func(*testing.T, *FrameHeader)
	}{
		{
			name: "Valid header with no data",
			setupFunc: func() []byte {
				frame := NewFrame(FrameTypeWakeWord, 12345, 100, 1640995200000000, nil)
				data, _ := frame.Serialize()
				return data[:HeaderSize] // Just the header
			},
			wantErr: false,
			validate: func(t *testing.T, header *FrameHeader) {
				if header.Magic != FrameMagic {
					t.Errorf("Magic = 0x%08X, want 0x%08X", header.Magic, FrameMagic)
				}
				if header.Type != FrameTypeWakeWord {
					t.Errorf("Type = %v, want %v", header.Type, FrameTypeWakeWord)
				}
				if header.SessionID != 12345 {
					t.Errorf("SessionID = %d, want %d", header.SessionID, 12345)
				}
				if header.Sequence != 100 {
					t.Errorf("Sequence = %d, want %d", header.Sequence, 100)
				}
				if header.Length != 0 {
					t.Errorf("Length = %d, want %d", header.Length, 0)
				}
			},
		},
		{
			name: "Valid header with data",
			setupFunc: func() []byte {
				testData := []byte("test payload")
				frame := NewFrame(FrameTypeAudioData, 67890, 200, 1640995300000000, testData)
				data, _ := frame.Serialize()
				return data[:HeaderSize] // Just the header
			},
			wantErr: false,
			validate: func(t *testing.T, header *FrameHeader) {
				if header.Type != FrameTypeAudioData {
					t.Errorf("Type = %v, want %v", header.Type, FrameTypeAudioData)
				}
				if header.SessionID != 67890 {
					t.Errorf("SessionID = %d, want %d", header.SessionID, 67890)
				}
				if header.Length != 12 { // len("test payload")
					t.Errorf("Length = %d, want %d", header.Length, 12)
				}
			},
		},
		{
			name: "Invalid header size - too small",
			setupFunc: func() []byte {
				return make([]byte, HeaderSize-1)
			},
			wantErr: true,
			errMsg:  "invalid header size",
		},
		{
			name: "Invalid header size - too large",
			setupFunc: func() []byte {
				return make([]byte, HeaderSize+1)
			},
			wantErr: true,
			errMsg:  "invalid header size",
		},
		{
			name: "Invalid magic number",
			setupFunc: func() []byte {
				headerBuf := make([]byte, HeaderSize)
				// Write invalid magic number
				binary.BigEndian.PutUint32(headerBuf[0:4], 0xDEADBEEF)
				return headerBuf
			},
			wantErr: true,
			errMsg:  "invalid frame magic",
		},
		{
			name: "Data length too large",
			setupFunc: func() []byte {
				headerBuf := make([]byte, HeaderSize)
				binary.BigEndian.PutUint32(headerBuf[0:4], FrameMagic)    // Magic
				headerBuf[4] = uint8(FrameTypeWakeWord)                   // Type
				headerBuf[5] = 0                                          // Reserved
				binary.BigEndian.PutUint16(headerBuf[6:8], MaxDataSize+1) // Length (too large)
				return headerBuf
			},
			wantErr: true,
			errMsg:  "frame data too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headerData := tt.setupFunc()
			header, err := parseFrameHeader(headerData)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got: %v", tt.errMsg, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if header == nil {
				t.Error("Expected header but got nil")
				return
			}

			if tt.validate != nil {
				tt.validate(t, header)
			}
		})
	}
}
