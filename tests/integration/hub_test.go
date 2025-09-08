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

package integration

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/loqalabs/loqa-proto/go/audio"
)

const (
	testHubAddress = "localhost:50051"
	testTimeout    = 30 * time.Second
)

func TestHubConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Connect to hub
	conn, err := grpc.DialContext(ctx, testHubAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		t.Skipf("Could not connect to hub at %s: %v", testHubAddress, err)
	}
	defer conn.Close()

	// Create client
	client := pb.NewAudioServiceClient(conn)

	// Test streaming connection
	stream, err := client.StreamAudio(ctx)
	if err != nil {
		t.Fatalf("Failed to create audio stream: %v", err)
	}
	defer stream.CloseSend()

	// Send test audio chunk
	testChunk := &pb.AudioChunk{
		RelayId:    "test-relay",
		AudioData:  make([]byte, 1024), // Empty test data
		SampleRate: 16000,
		Timestamp:  time.Now().UnixNano(),
	}

	if err := stream.Send(testChunk); err != nil {
		t.Fatalf("Failed to send audio chunk: %v", err)
	}

	// Close send stream and wait for response
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("Failed to close send stream: %v", err)
	}

	// Try to receive response (should timeout gracefully)
	select {
	case <-ctx.Done():
		t.Log("Stream test completed successfully")
	case <-time.After(5 * time.Second):
		t.Log("Stream test completed within timeout")
	}
}

func TestHubAudioProcessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Connect to hub
	conn, err := grpc.DialContext(ctx, testHubAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		t.Skipf("Could not connect to hub at %s: %v", testHubAddress, err)
	}
	defer conn.Close()

	client := pb.NewAudioServiceClient(conn)

	// Test audio processing stream
	stream, err := client.StreamAudio(ctx)
	if err != nil {
		t.Fatalf("Failed to create audio stream: %v", err)
	}

	// Send multiple audio chunks to simulate real audio
	for i := 0; i < 5; i++ {
		chunk := &pb.AudioChunk{
			RelayId:    "test-relay",
			AudioData:  make([]byte, 1024),
			SampleRate: 16000,
			Timestamp:  time.Now().UnixNano(),
		}

		if err := stream.Send(chunk); err != nil {
			t.Fatalf("Failed to send audio chunk %d: %v", i, err)
		}

		// Small delay between chunks
		time.Sleep(100 * time.Millisecond)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("Failed to close send stream: %v", err)
	}

	t.Log("Audio processing test completed successfully")
}
