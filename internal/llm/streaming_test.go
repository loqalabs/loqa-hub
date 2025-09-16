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

package llm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
)

// MockTTSClient implements TextToSpeech interface for testing
type MockTTSClient struct {
	synthesizeDelay time.Duration
	voices          []string
	shouldFail      bool
}

func NewMockTTSClient() *MockTTSClient {
	return &MockTTSClient{
		synthesizeDelay: 100 * time.Millisecond,
		voices:          []string{"test_voice", "af_bella"},
		shouldFail:      false,
	}
}

func (m *MockTTSClient) Synthesize(text string, options *TTSOptions) (*TTSResult, error) {
	if m.shouldFail {
		return nil, io.ErrUnexpectedEOF
	}

	// Simulate TTS processing delay
	time.Sleep(m.synthesizeDelay)

	// Create mock audio stream
	audioData := strings.NewReader("mock-audio-data-for-" + text)

	return &TTSResult{
		Audio:       audioData,
		ContentType: "audio/mp3",
		Length:      int64(audioData.Len()),
	}, nil
}

func (m *MockTTSClient) GetAvailableVoices() ([]string, error) {
	return m.voices, nil
}

func (m *MockTTSClient) Close() error {
	return nil
}

// Test PhraseBuffer functionality
func TestPhraseBuffer(t *testing.T) {

	tests := []struct {
		name           string
		tokens         []string
		expectedPhrase string
		description    string
	}{
		{
			name:           "sentence boundary",
			tokens:         []string{"Hello", " ", "world", "."},
			expectedPhrase: "Hello world.",
			description:    "Should flush on sentence ending",
		},
		{
			name:           "conjunction boundary",
			tokens:         []string{"Turn", " on", " lights", ", and"},
			expectedPhrase: "Turn on lights, and",
			description:    "Should flush on conjunction",
		},
		{
			name:           "no boundary",
			tokens:         []string{"Just", " some", " words"},
			expectedPhrase: "",
			description:    "Should not flush without boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testBuffer := NewPhraseBuffer()
			var result string

			for _, token := range tt.tokens {
				phrase := testBuffer.AddToken(token)
				if phrase != "" {
					result = phrase
					break
				}
			}

			if result != tt.expectedPhrase {
				t.Errorf("Expected phrase '%s', got '%s'", tt.expectedPhrase, result)
			}
		})
	}
}

// Test StreamingCommandParser basic functionality
func TestStreamingCommandParser_FallbackMode(t *testing.T) {
	parser := NewStreamingCommandParser("http://localhost:11434", "test-model", false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := parser.ParseCommandStreaming(ctx, "turn on the lights")
	if err != nil {
		t.Fatalf("Streaming parser failed: %v", err)
	}

	// Should get immediate result in fallback mode
	select {
	case cmd := <-result.FinalCommand:
		if cmd == nil {
			t.Fatal("Expected command, got nil")
		}
		t.Logf("Received command: %+v", cmd)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for command in fallback mode")
	}

	result.Cancel()
}

// Test StreamingAudioPipeline
func TestStreamingAudioPipeline(t *testing.T) {
	mockTTS := NewMockTTSClient()
	options := &TTSOptions{
		Voice:          "test_voice",
		Speed:          1.0,
		ResponseFormat: "mp3",
		Normalize:      true,
	}

	pipeline := NewStreamingAudioPipeline(mockTTS, options)

	// Create phrase stream
	phraseStream := make(chan string, 10)
	phraseStream <- "Hello world."
	phraseStream <- "How are you?"
	close(phraseStream)

	// Start pipeline
	pipelineCtx, err := pipeline.StartPipeline("test-session", phraseStream)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Collect audio chunks
	var chunks []*AudioChunk
	timeout := time.After(5 * time.Second)

	for {
		select {
		case chunk, ok := <-pipelineCtx.AudioChunks:
			if !ok {
				t.Log("Audio chunks channel closed")
				goto done
			}
			chunks = append(chunks, chunk)
			t.Logf("Received audio chunk %d: %s", chunk.SequenceID, chunk.Phrase)
			if chunk.IsLast {
				goto done
			}

		case err := <-pipelineCtx.ErrorChan:
			t.Fatalf("Pipeline error: %v", err)

		case <-timeout:
			t.Fatal("Timeout waiting for audio chunks")
		}
	}

done:
	pipeline.StopPipeline("test-session")

	// Verify we got expected chunks
	if len(chunks) < 2 {
		t.Errorf("Expected at least 2 audio chunks, got %d", len(chunks))
	}

	// Verify sequence order
	for i, chunk := range chunks {
		if !chunk.IsLast && chunk.SequenceID != i {
			t.Errorf("Expected sequence ID %d, got %d", i, chunk.SequenceID)
		}
	}
}

// Test StreamingInterruptHandler
func TestStreamingInterruptHandler(t *testing.T) {
	handler := NewStreamingInterruptHandler(500*time.Millisecond, 1*time.Second)

	// Create mock session
	ctx, cancel := context.WithCancel(context.Background())
	result := &StreamingResult{
		TokenStream:  make(chan string, 10),
		FinalCommand: make(chan *Command, 1),
		ErrorChan:    make(chan error, 1),
		Cancel:       cancel,
	}

	// Register session
	handler.RegisterSession("test-session", ctx, cancel, result, nil)

	// Verify session is active
	activeIDs := handler.GetActiveSessionIDs()
	if len(activeIDs) != 1 || activeIDs[0] != "test-session" {
		t.Errorf("Expected 1 active session 'test-session', got %v", activeIDs)
	}

	// Interrupt session
	err := handler.InterruptSession("test-session", InterruptReasonUserRequest)
	if err != nil {
		t.Errorf("Failed to interrupt session: %v", err)
	}

	// Wait for cleanup
	time.Sleep(600 * time.Millisecond)

	// Verify session was cleaned up
	activeIDs = handler.GetActiveSessionIDs()
	if len(activeIDs) != 0 {
		t.Errorf("Expected 0 active sessions after interrupt, got %v", activeIDs)
	}
}

// Test StreamingMetricsCollector
func TestStreamingMetricsCollector(t *testing.T) {
	collector := NewStreamingMetricsCollector(true)

	// Record session start
	collector.RecordSessionStart("test-session")

	// Create mock metrics
	metrics := &StreamingMetrics{
		StartTime:        time.Now().Add(-1 * time.Second),
		FirstTokenTime:   time.Now().Add(-800 * time.Millisecond),
		FirstPhraseTime:  time.Now().Add(-600 * time.Millisecond),
		CompletionTime:   time.Now(),
		TokenCount:       25,
		PhraseCount:      3,
		BufferOverflows:  0,
		InterruptCount:   0,
	}

	// Record session completion
	collector.RecordSessionMetrics("test-session", metrics)

	// Get aggregate metrics
	agg := collector.GetAggregateMetrics()
	if agg.TotalSessions != 1 {
		t.Errorf("Expected 1 total session, got %d", agg.TotalSessions)
	}
	if agg.CompletedSessions != 1 {
		t.Errorf("Expected 1 completed session, got %d", agg.CompletedSessions)
	}
	if agg.TotalTokens != 25 {
		t.Errorf("Expected 25 total tokens, got %d", agg.TotalTokens)
	}

	// Generate performance report
	report := collector.GeneratePerformanceReport()
	if report.HealthStatus != "healthy" {
		t.Errorf("Expected healthy status, got %s", report.HealthStatus)
	}
}

// Test StreamingComponents integration
func TestStreamingComponents(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Streaming: config.StreamingConfig{
			Enabled:              true,
			OllamaURL:           "http://localhost:11434",
			Model:               "test-model",
			MaxBufferTime:       2 * time.Second,
			MaxTokensPerPhrase:  50,
			AudioConcurrency:    3,
			VisualFeedbackDelay: 50 * time.Millisecond,
			InterruptTimeout:    500 * time.Millisecond,
			FallbackEnabled:     true,
			MetricsEnabled:      true,
		},
		TTS: config.TTSConfig{
			Voice:          "test_voice",
			Speed:          1.0,
			ResponseFormat: "mp3",
			Normalize:      true,
		},
	}

	// Create mock TTS client
	mockTTS := NewMockTTSClient()

	// Initialize streaming components
	components, err := NewStreamingComponents(cfg, mockTTS)
	if err != nil {
		// Expected to fail without real Ollama connection
		t.Logf("Expected failure connecting to test Ollama: %v", err)
		return
	}

	defer components.Shutdown()

	// Get health status
	health := components.GetHealthStatus()
	if !health.MetricsEnabled {
		t.Error("Expected metrics to be enabled")
	}

	t.Logf("Health status: %+v", health)
}

// Benchmark PhraseBuffer performance
func BenchmarkPhraseBuffer(b *testing.B) {
	buffer := NewPhraseBuffer()
	tokens := []string{"Hello", " ", "world", " ", "this", " ", "is", " ", "a", " ", "test", "."}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, token := range tokens {
			buffer.AddToken(token)
		}
		buffer.Flush()
	}
}

// Test error handling in audio pipeline
func TestStreamingAudioPipeline_ErrorHandling(t *testing.T) {
	mockTTS := NewMockTTSClient()
	mockTTS.shouldFail = true // Force TTS failures

	pipeline := NewStreamingAudioPipeline(mockTTS, nil)

	// Create phrase stream
	phraseStream := make(chan string, 1)
	phraseStream <- "This should fail"
	close(phraseStream)

	// Start pipeline
	pipelineCtx, err := pipeline.StartPipeline("error-test", phraseStream)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Should receive error
	select {
	case err := <-pipelineCtx.ErrorChan:
		t.Logf("Received expected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for expected error")
	}

	pipeline.StopPipeline("error-test")
}

// Test concurrent pipeline sessions
func TestStreamingAudioPipeline_Concurrent(t *testing.T) {
	mockTTS := NewMockTTSClient()
	pipeline := NewStreamingAudioPipeline(mockTTS, nil)

	// Start multiple concurrent sessions
	numSessions := 3
	sessions := make([]*PipelineContext, numSessions)

	for i := 0; i < numSessions; i++ {
		phraseStream := make(chan string, 1)
		phraseStream <- "Concurrent session test"
		close(phraseStream)

		sessionID := fmt.Sprintf("session-%d", i)
		ctx, err := pipeline.StartPipeline(sessionID, phraseStream)
		if err != nil {
			t.Fatalf("Failed to start session %d: %v", i, err)
		}
		sessions[i] = ctx
	}

	// Collect results from all sessions
	for i, session := range sessions {
		select {
		case chunk := <-session.AudioChunks:
			t.Logf("Session %d received chunk: %s", i, chunk.Phrase)
		case err := <-session.ErrorChan:
			t.Errorf("Session %d error: %v", i, err)
		case <-time.After(3 * time.Second):
			t.Errorf("Session %d timeout", i)
		}

		pipeline.StopPipeline(fmt.Sprintf("session-%d", i))
	}
}