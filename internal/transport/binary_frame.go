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
	"io"
)

// Binary Frame Protocol for HTTP/1.1 Chunked Transfer
// Designed for ESP32 compatibility with minimal overhead

// FrameType represents the type of frame being transmitted
type FrameType uint8

const (
	// Audio frame types
	FrameTypeAudioData FrameType = 0x01
	FrameTypeAudioEnd  FrameType = 0x02
	FrameTypeWakeWord  FrameType = 0x03

	// Control frame types
	FrameTypeHeartbeat   FrameType = 0x10
	FrameTypeHandshake   FrameType = 0x11
	FrameTypeError       FrameType = 0x12
	FrameTypeArbitration FrameType = 0x13

	// Response frame types
	FrameTypeResponse FrameType = 0x20
	FrameTypeStatus   FrameType = 0x21
)

// Frame represents a binary frame in the protocol
type Frame struct {
	Type      FrameType
	SessionID uint32
	Sequence  uint32
	Timestamp uint64
	Data      []byte
}

// FrameHeader represents the fixed-size frame header (20 bytes)
type FrameHeader struct {
	Magic     uint32    // 0x4C4F5141 ("LOQA")
	Type      FrameType // Frame type (1 byte)
	Reserved  uint8     // Reserved for future use (1 byte)
	Length    uint16    // Data payload length (2 bytes)
	SessionID uint32    // Session identifier (4 bytes)
	Sequence  uint32    // Sequence number (4 bytes)
	Timestamp uint64    // Unix timestamp microseconds (8 bytes)
}

const (
	// Magic number for frame validation
	FrameMagic = 0x4C4F5141 // "LOQA" in big-endian

	// Frame size constraints for ESP32 compatibility
	MaxFrameSize = 4096 // 4KB max frame size
	HeaderSize   = 24   // Fixed header size
	MaxDataSize  = MaxFrameSize - HeaderSize
)

// Serialize converts a frame to binary format
func (f *Frame) Serialize() ([]byte, error) {
	if len(f.Data) > MaxDataSize {
		return nil, fmt.Errorf("frame data too large: %d bytes (max %d)", len(f.Data), MaxDataSize)
	}

	// Check data length to prevent overflow
	dataLen := len(f.Data)
	if dataLen > 65535 { // Max uint16
		dataLen = 65535
	}

	header := FrameHeader{
		Magic:     FrameMagic,
		Type:      f.Type,
		Reserved:  0,
		Length:    uint16(dataLen), //nolint:gosec // G115: Safe conversion after bounds check above
		SessionID: f.SessionID,
		Sequence:  f.Sequence,
		Timestamp: f.Timestamp,
	}

	buf := new(bytes.Buffer)

	// Write header in big-endian format
	if err := binary.Write(buf, binary.BigEndian, header); err != nil {
		return nil, fmt.Errorf("failed to write frame header: %w", err)
	}

	// Write data payload
	if len(f.Data) > 0 {
		if _, err := buf.Write(f.Data); err != nil {
			return nil, fmt.Errorf("failed to write frame data: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// Deserialize converts binary data to a frame
func DeserializeFrame(data []byte) (*Frame, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("frame too small: %d bytes (min %d)", len(data), HeaderSize)
	}

	buf := bytes.NewReader(data)
	var header FrameHeader

	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	// Validate magic number
	if header.Magic != FrameMagic {
		return nil, fmt.Errorf("invalid frame magic: 0x%08X (expected 0x%08X)", header.Magic, FrameMagic)
	}

	// Validate frame size
	expectedSize := HeaderSize + int(header.Length)
	if len(data) != expectedSize {
		return nil, fmt.Errorf("frame size mismatch: got %d bytes, expected %d", len(data), expectedSize)
	}

	frame := &Frame{
		Type:      header.Type,
		SessionID: header.SessionID,
		Sequence:  header.Sequence,
		Timestamp: header.Timestamp,
	}

	// Read data payload if present
	if header.Length > 0 {
		frame.Data = make([]byte, header.Length)
		if _, err := io.ReadFull(buf, frame.Data); err != nil {
			return nil, fmt.Errorf("failed to read frame data: %w", err)
		}
	}

	return frame, nil
}

// parseFrameHeader parses just the header portion of frame data
// This is used when reading frames incrementally (header first, then data)
func parseFrameHeader(headerData []byte) (*FrameHeader, error) {
	if len(headerData) != HeaderSize {
		return nil, fmt.Errorf("invalid header size: %d bytes (expected %d)", len(headerData), HeaderSize)
	}

	buf := bytes.NewReader(headerData)
	var header FrameHeader

	if err := binary.Read(buf, binary.BigEndian, &header); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	// Validate magic number
	if header.Magic != FrameMagic {
		return nil, fmt.Errorf("invalid frame magic: 0x%08X (expected 0x%08X)", header.Magic, FrameMagic)
	}

	// Validate data length doesn't exceed maximum
	if header.Length > MaxDataSize {
		return nil, fmt.Errorf("frame data too large: %d bytes (max %d)", header.Length, MaxDataSize)
	}

	return &header, nil
}

// NewFrame creates a new frame with the specified parameters
func NewFrame(frameType FrameType, sessionID, sequence uint32, timestamp uint64, data []byte) *Frame {
	return &Frame{
		Type:      frameType,
		SessionID: sessionID,
		Sequence:  sequence,
		Timestamp: timestamp,
		Data:      data,
	}
}

// IsValid checks if the frame is structurally valid
func (f *Frame) IsValid() bool {
	return len(f.Data) <= MaxDataSize
}

// Size returns the total serialized size of the frame
func (f *Frame) Size() int {
	return HeaderSize + len(f.Data)
}
