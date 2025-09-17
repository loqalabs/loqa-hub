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
	"io"
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	grpcservice "github.com/loqalabs/loqa-hub/internal/grpc"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/storage"
	pb "github.com/loqalabs/loqa-proto/go/audio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// Test-specific mock implementations
type TestTranscriber struct {
	responses map[string]*llm.TranscriptionResult
}

func NewTestTranscriber() *TestTranscriber {
	return &TestTranscriber{
		responses: map[string]*llm.TranscriptionResult{
			"turn_on_lights": {
				Text:               "turn on the kitchen lights",
				ConfidenceEstimate: 0.95,
				WakeWordDetected:   false,
				NeedsConfirmation:  false,
			},
			"turn_off_lights": {
				Text:               "turn off the bedroom lights",
				ConfidenceEstimate: 0.92,
				WakeWordDetected:   false,
				NeedsConfirmation:  false,
			},
			"low_confidence": {
				Text:               "mumbled command",
				ConfidenceEstimate: 0.3,
				WakeWordDetected:   false,
				NeedsConfirmation:  true,
			},
			"empty": {
				Text:               "",
				ConfidenceEstimate: 0.0,
				WakeWordDetected:   false,
				NeedsConfirmation:  false,
			},
		},
	}
}

func (t *TestTranscriber) Transcribe(audioData []float32, sampleRate int) (string, error) {
	result, err := t.TranscribeWithConfidence(audioData, sampleRate)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func (t *TestTranscriber) TranscribeWithConfidence(audioData []float32, sampleRate int) (*llm.TranscriptionResult, error) {
	// Simple heuristic: use length of audio data to determine response type
	switch {
	case len(audioData) == 0:
		return t.responses["empty"], nil
	case len(audioData) < 1000:
		return t.responses["low_confidence"], nil
	case len(audioData) < 2000:
		return t.responses["turn_on_lights"], nil
	default:
		return t.responses["turn_off_lights"], nil
	}
}

type TestTTSClient struct {
	responses map[string][]byte
}

func NewTestTTSClient() *TestTTSClient {
	return &TestTTSClient{
		responses: map[string][]byte{
			"Turning on the kitchen lights now":      []byte("audio_data_turn_on"),
			"The kitchen lights are now on.":         []byte("audio_data_lights_on"),
			"Turning off the bedroom lights":         []byte("audio_data_turn_off"),
			"I didn't hear anything. Please try again.": []byte("audio_data_no_speech"),
			"I'm not sure I heard you correctly":     []byte("audio_data_confirmation"),
		},
	}
}

func (t *TestTTSClient) Synthesize(text string, options *llm.TTSOptions) (*llm.TTSResult, error) {
	// Find matching response based on text content
	for key, audioData := range t.responses {
		if strings.Contains(text, key) || strings.Contains(key, text) {
			return &llm.TTSResult{
				Audio:  strings.NewReader(string(audioData)),
				Length: len(audioData),
			}, nil
		}
	}

	// Default response for unmatched text
	defaultAudio := []byte("default_audio_response")
	return &llm.TTSResult{
		Audio:  strings.NewReader(string(defaultAudio)),
		Length: len(defaultAudio),
	}, nil
}

func (t *TestTTSClient) GetVoices() ([]llm.TTSVoice, error) {
	return []llm.TTSVoice{{ID: "test", Name: "Test Voice"}}, nil
}

func (t *TestTTSClient) HealthCheck() error {
	return nil
}

// Test setup helper
func setupTestServer(t *testing.T) (*grpc.Server, *bufconn.Listener, *grpcservice.AudioService) {
	// Create in-memory database
	db, err := storage.NewDatabase(storage.DatabaseConfig{Path: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	eventsStore := storage.NewVoiceEventsStore(db)

	// Create test audio service with mock transcriber and TTS
	transcriber := NewTestTranscriber()
	ttsClient := NewTestTTSClient()

	audioService, err := grpcservice.NewAudioServiceWithTTSAndOptions(
		"", "", config.TTSConfig{}, eventsStore, false) // Disable health checks for testing
	if err != nil {
		t.Fatalf("Failed to create audio service: %v", err)
	}

	// Replace the transcriber and TTS with our test mocks
	// Note: This would require the AudioService to expose setters or be refactored for better testability
	// For now, we'll create a custom audio service

	// Create gRPC server with buffer
	lis := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	pb.RegisterAudioServiceServer(server, audioService)

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	return server, lis, audioService
}

func bufDialer(listener *bufconn.Listener) func(context.Context, string) (grpc.ClientConnInterface, error) {
	return func(ctx context.Context, url string) (grpc.ClientConnInterface, error) {
		return grpc.DialContext(ctx, "", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
}

func TestAudioPipeline_EndToEnd(t *testing.T) {
	server, lis, _ := setupTestServer(t)
	defer server.Stop()

	// Create client connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}
	defer conn.Close()

	client := pb.NewAudioServiceClient(conn)

	// Test successful voice command processing
	t.Run("successful_voice_command", func(t *testing.T) {
		stream, err := client.StreamAudio(ctx)
		if err != nil {
			t.Fatalf("Failed to create stream: %v", err)
		}

		// Send wake word chunk
		wakeWordChunk := &pb.AudioChunk{
			RelayId:       "test-relay-001",
			AudioData:     generateTestAudioBytes(500), // Small for wake word
			SampleRate:    16000,
			IsWakeWord:    true,
			IsEndOfSpeech: false,
			Timestamp:     time.Now().UnixNano(),
		}

		if err := stream.Send(wakeWordChunk); err != nil {
			t.Fatalf("Failed to send wake word chunk: %v", err)
		}

		// Send speech chunk
		speechChunk := &pb.AudioChunk{
			RelayId:       "test-relay-001",
			AudioData:     generateTestAudioBytes(1500), // Medium size for turn_on_lights
			SampleRate:    16000,
			IsWakeWord:    false,
			IsEndOfSpeech: true,
			Timestamp:     time.Now().UnixNano(),
		}

		if err := stream.Send(speechChunk); err != nil {
			t.Fatalf("Failed to send speech chunk: %v", err)
		}

		// Receive response
		response, err := stream.Recv()
		if err != nil {
			t.Fatalf("Failed to receive response: %v", err)
		}

		// Verify response
		if !response.Success {
			t.Error("Expected successful response")
		}

		if response.Transcription != "turn on the kitchen lights" {
			t.Errorf("Expected transcription 'turn on the kitchen lights', got '%s'", response.Transcription)
		}

		if response.Command == "" {
			t.Error("Expected non-empty command")
		}

		if response.ResponseText == "" {
			t.Error("Expected non-empty response text")
		}

		// Verify TTS audio is included
		if len(response.ResponseAudio) == 0 {
			t.Error("Expected TTS audio data in response")
		}

		if response.AudioFormat == "" {
			t.Error("Expected audio format to be set")
		}

		t.Logf("Received response: %+v", response)
		t.Logf("Audio data length: %d", len(response.ResponseAudio))

		if err := stream.CloseSend(); err != nil {
			t.Errorf("Failed to close send: %v", err)
		}
	})

	// Test low confidence handling
	t.Run("low_confidence_command", func(t *testing.T) {
		stream, err := client.StreamAudio(ctx)
		if err != nil {
			t.Fatalf("Failed to create stream: %v", err)
		}

		// Send chunk that triggers low confidence
		chunk := &pb.AudioChunk{
			RelayId:       "test-relay-002",
			AudioData:     generateTestAudioBytes(500), // Small size triggers low confidence
			SampleRate:    16000,
			IsWakeWord:    false,
			IsEndOfSpeech: true,
			Timestamp:     time.Now().UnixNano(),
		}

		if err := stream.Send(chunk); err != nil {
			t.Fatalf("Failed to send chunk: %v", err)
		}

		// Receive response
		response, err := stream.Recv()
		if err != nil {
			t.Fatalf("Failed to receive response: %v", err)
		}

		// Verify confirmation request
		if response.Command != "confirmation_needed" {
			t.Errorf("Expected confirmation_needed command, got '%s'", response.Command)
		}

		if !strings.Contains(response.ResponseText, "not sure I heard") {
			t.Errorf("Expected confirmation message, got '%s'", response.ResponseText)
		}

		if err := stream.CloseSend(); err != nil {
			t.Errorf("Failed to close send: %v", err)
		}
	})

	// Test no speech detected
	t.Run("no_speech_detected", func(t *testing.T) {
		stream, err := client.StreamAudio(ctx)
		if err != nil {
			t.Fatalf("Failed to create stream: %v", err)
		}

		// Send empty audio chunk
		chunk := &pb.AudioChunk{
			RelayId:       "test-relay-003",
			AudioData:     []byte{}, // Empty triggers no speech
			SampleRate:    16000,
			IsWakeWord:    false,
			IsEndOfSpeech: true,
			Timestamp:     time.Now().UnixNano(),
		}

		if err := stream.Send(chunk); err != nil {
			t.Fatalf("Failed to send chunk: %v", err)
		}

		// Receive response
		response, err := stream.Recv()
		if err != nil {
			t.Fatalf("Failed to receive response: %v", err)
		}

		// Verify no speech response
		if response.Command != "no_speech" {
			t.Errorf("Expected no_speech command, got '%s'", response.Command)
		}

		if !strings.Contains(response.ResponseText, "didn't hear anything") {
			t.Errorf("Expected no speech message, got '%s'", response.ResponseText)
		}

		if err := stream.CloseSend(); err != nil {
			t.Errorf("Failed to close send: %v", err)
		}
	})
}

func TestAudioPipeline_MultipleConcurrentStreams(t *testing.T) {
	server, lis, _ := setupTestServer(t)
	defer server.Stop()

	ctx := context.Background()
	numClients := 3

	// Create multiple concurrent streams
	for i := 0; i < numClients; i++ {
		t.Run(fmt.Sprintf("client_%d", i), func(t *testing.T) {
			t.Parallel()

			// Create client connection
			conn, err := grpc.DialContext(ctx, "",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return lis.Dial()
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("Failed to dial server: %v", err)
			}
			defer conn.Close()

			client := pb.NewAudioServiceClient(conn)
			stream, err := client.StreamAudio(ctx)
			if err != nil {
				t.Fatalf("Failed to create stream: %v", err)
			}

			// Send audio chunk
			chunk := &pb.AudioChunk{
				RelayId:       fmt.Sprintf("test-relay-%03d", i),
				AudioData:     generateTestAudioBytes(1500),
				SampleRate:    16000,
				IsWakeWord:    false,
				IsEndOfSpeech: true,
				Timestamp:     time.Now().UnixNano(),
			}

			if err := stream.Send(chunk); err != nil {
				t.Fatalf("Failed to send chunk: %v", err)
			}

			// Receive response
			response, err := stream.Recv()
			if err != nil {
				t.Fatalf("Failed to receive response: %v", err)
			}

			if !response.Success {
				t.Error("Expected successful response")
			}

			if err := stream.CloseSend(); err != nil {
				t.Errorf("Failed to close send: %v", err)
			}
		})
	}
}

func TestAudioPipeline_StreamingPredictiveBridge_Performance(t *testing.T) {
	server, lis, _ := setupTestServer(t)
	defer server.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial server: %v", err)
	}
	defer conn.Close()

	client := pb.NewAudioServiceClient(conn)

	// Measure response time for predictive vs traditional processing
	numRequests := 10
	responseTimes := make([]time.Duration, numRequests)

	for i := 0; i < numRequests; i++ {
		start := time.Now()

		stream, err := client.StreamAudio(ctx)
		if err != nil {
			t.Fatalf("Failed to create stream: %v", err)
		}

		chunk := &pb.AudioChunk{
			RelayId:       fmt.Sprintf("perf-test-%d", i),
			AudioData:     generateTestAudioBytes(1500),
			SampleRate:    16000,
			IsWakeWord:    false,
			IsEndOfSpeech: true,
			Timestamp:     time.Now().UnixNano(),
		}

		if err := stream.Send(chunk); err != nil {
			t.Fatalf("Failed to send chunk: %v", err)
		}

		_, err = stream.Recv()
		if err != nil {
			t.Fatalf("Failed to receive response: %v", err)
		}

		responseTimes[i] = time.Since(start)

		if err := stream.CloseSend(); err != nil {
			t.Errorf("Failed to close send: %v", err)
		}
	}

	// Calculate average response time
	var total time.Duration
	for _, rt := range responseTimes {
		total += rt
	}
	avgResponseTime := total / time.Duration(numRequests)

	t.Logf("Average response time over %d requests: %v", numRequests, avgResponseTime)

	// With predictive responses, we expect sub-second response times
	maxExpectedTime := 5 * time.Second // Generous for test environment
	if avgResponseTime > maxExpectedTime {
		t.Errorf("Average response time %v exceeds expected maximum %v", avgResponseTime, maxExpectedTime)
	}
}

// Helper function to generate test audio bytes
func generateTestAudioBytes(numSamples int) []byte {
	// Generate 16-bit PCM audio data
	audioBytes := make([]byte, numSamples*2)
	for i := 0; i < numSamples; i++ {
		// Generate simple sine wave pattern
		sample := int16(1000.0 * math.Sin(2*math.Pi*float64(i)*440.0/16000.0))
		audioBytes[i*2] = byte(sample)
		audioBytes[i*2+1] = byte(sample >> 8)
	}
	return audioBytes
}

// Need to import these for the helper function
import (
	"fmt"
	"math"
	"net"
)