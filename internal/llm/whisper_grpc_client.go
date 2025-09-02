/*
Copyright (c) 2024 Loqa Labs

Licensed under the AGPLv3 License.
This file is part of the loqa-hub.
*/

package llm

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/loqalabs/loqa-hub/internal/logging"
	whisperpb "github.com/loqalabs/loqa-proto/go/whisper"
)

// WhisperGRPCClient implements the Transcriber interface using a gRPC connection
// to a faster-whisper service
type WhisperGRPCClient struct {
	conn   *grpc.ClientConn
	client whisperpb.WhisperServiceClient
	addr   string
}

// NewWhisperGRPCClient creates a new gRPC-based transcriber
func NewWhisperGRPCClient(addr string) (*WhisperGRPCClient, error) {
	if addr == "" {
		addr = "localhost:50052" // Default whisper service address
	}

	logging.Sugar.Infow("Connecting to Whisper gRPC service", "address", addr)

	// Create connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // Wait for connection to be ready
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to whisper service at %s: %w", addr, err)
	}

	client := whisperpb.NewWhisperServiceClient(conn)

	// Test connection with health check
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer healthCancel()

	healthResp, err := client.Health(healthCtx, &whisperpb.HealthRequest{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("whisper service health check failed: %w", err)
	}

	logging.Sugar.Infow("Connected to Whisper gRPC service",
		"address", addr,
		"status", healthResp.Status,
		"model", healthResp.ModelName,
		"version", healthResp.Version,
	)

	return &WhisperGRPCClient{
		conn:   conn,
		client: client,
		addr:   addr,
	}, nil
}

// Transcribe implements the Transcriber interface
func (w *WhisperGRPCClient) Transcribe(audioData []float32, sampleRate int) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("empty audio data")
	}

	if sampleRate <= 0 {
		return "", fmt.Errorf("invalid sample rate: %d", sampleRate)
	}

	// Create unique request ID for tracking
	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())

	logging.Sugar.Infow("Sending transcription request",
		"request_id", requestID,
		"samples", len(audioData),
		"sample_rate", sampleRate,
	)

	// Create request
	req := &whisperpb.TranscribeRequest{
		AudioData:  audioData,
		SampleRate: int32(sampleRate),
		Language:   "", // Auto-detect
		RequestId:  requestID,
	}

	// Set timeout for transcription
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Call transcription service
	resp, err := w.client.Transcribe(ctx, req)
	if err != nil {
		return "", fmt.Errorf("transcription gRPC call failed: %w", err)
	}

	// Check if transcription was successful
	if !resp.Success {
		return "", fmt.Errorf("transcription failed: %s", resp.ErrorMessage)
	}

	logging.Sugar.Infow("Transcription completed",
		"request_id", requestID,
		"language", resp.Language,
		"processing_time_ms", resp.ProcessingTimeMs,
		"text_length", len(resp.Text),
	)

	return resp.Text, nil
}

// Close closes the gRPC connection
func (w *WhisperGRPCClient) Close() error {
	if w.conn != nil {
		logging.Sugar.Infow("Closing Whisper gRPC connection", "address", w.addr)
		return w.conn.Close()
	}
	return nil
}