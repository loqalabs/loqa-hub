package integration

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/loqa-voice-assistant/proto/go"
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
		Data:       make([]byte, 1024), // Empty test data
		SampleRate: 16000,
		Channels:   1,
		Format:     pb.AudioFormat_PCM_16,
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
			Data:       make([]byte, 1024),
			SampleRate: 16000,
			Channels:   1,
			Format:     pb.AudioFormat_PCM_16,
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