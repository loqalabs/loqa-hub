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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StreamingCommandParser handles streaming command classification using LLM
type StreamingCommandParser struct {
	ollamaURL   string
	model       string
	client      *http.Client
	enabled     bool
	fallbackParser *CommandParser
}

// StreamingResult represents a streaming command parsing result
type StreamingResult struct {
	TokenStream    chan string           // Stream of tokens as they arrive
	FinalCommand   chan *Command         // Final parsed command
	ErrorChan      chan error            // Error notifications
	Cancel         context.CancelFunc    // Cancellation function
	VisualTokens   chan string           // Immediate visual feedback tokens
	AudioPhrases   chan string           // Buffered phrases for TTS
	Metrics        *StreamingMetrics     // Performance metrics
}

// StreamingMetrics tracks performance of streaming operations
type StreamingMetrics struct {
	StartTime        time.Time
	FirstTokenTime   time.Time
	FirstPhraseTime  time.Time
	CompletionTime   time.Time
	TokenCount       int
	PhraseCount      int
	BufferOverflows  int
	InterruptCount   int
}

// PhraseBuffer manages intelligent buffering for natural speech boundaries
type PhraseBuffer struct {
	tokens          []string
	lastFlushTime   time.Time
	boundaries      []string
	maxBufferTime   time.Duration
	maxTokens       int
	mu              sync.RWMutex
}

// StreamingToken represents a token from the streaming response
type StreamingToken struct {
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsLast    bool      `json:"is_last"`
}

// NewStreamingCommandParser creates a new streaming command parser
func NewStreamingCommandParser(ollamaURL, model string, enabled bool) *StreamingCommandParser {
	fallbackParser := NewCommandParser(ollamaURL, model)

	return &StreamingCommandParser{
		ollamaURL:      ollamaURL,
		model:          model,
		client:         &http.Client{Timeout: 30 * time.Second},
		enabled:        enabled,
		fallbackParser: fallbackParser,
	}
}

// NewPhraseBuffer creates a new phrase buffer with intelligent boundaries
func NewPhraseBuffer() *PhraseBuffer {
	return &PhraseBuffer{
		tokens: make([]string, 0, 100),
		boundaries: []string{
			".", "!", "?",                       // Sentence endings
			", and", ", then", ", so",           // Natural pauses
			", but", ", however", ", because",   // Connectors
			"\n",                                // Paragraph breaks
		},
		maxBufferTime: 2 * time.Second,
		maxTokens:     50, // Prevent runaway buffering
	}
}

// ParseCommandStreaming parses a command with streaming response
func (scp *StreamingCommandParser) ParseCommandStreaming(ctx context.Context, transcription string) (*StreamingResult, error) {
	if !scp.enabled {
		// Fall back to non-streaming parser
		cmd, err := scp.fallbackParser.ParseCommand(transcription)
		if err != nil {
			return nil, err
		}
		return scp.createFallbackResult(cmd), nil
	}

	if transcription == "" {
		return scp.createEmptyResult(), nil
	}

	// Create cancellable context
	streamCtx, cancel := context.WithCancel(ctx)

	// Create result channels
	result := &StreamingResult{
		TokenStream:   make(chan string, 100),
		FinalCommand:  make(chan *Command, 1),
		ErrorChan:     make(chan error, 1),
		Cancel:        cancel,
		VisualTokens:  make(chan string, 100),
		AudioPhrases:  make(chan string, 10),
		Metrics:       &StreamingMetrics{StartTime: time.Now()},
	}

	// Start streaming processing
	go scp.processStreamingResponse(streamCtx, transcription, result)

	return result, nil
}

// processStreamingResponse handles the streaming LLM response
func (scp *StreamingCommandParser) processStreamingResponse(ctx context.Context, transcription string, result *StreamingResult) {
	defer close(result.TokenStream)
	defer close(result.FinalCommand)
	defer close(result.VisualTokens)
	defer close(result.AudioPhrases)

	// Create phrase buffer for intelligent TTS synthesis
	phraseBuffer := NewPhraseBuffer()

	// Create streaming request
	prompt := scp.buildStreamingPrompt(transcription)
	streamResp, err := scp.createStreamingRequest(ctx, prompt)
	if err != nil {
		result.ErrorChan <- fmt.Errorf("failed to create streaming request: %w", err)
		return
	}
	defer streamResp.Body.Close()

	// Process streaming response
	scanner := bufio.NewScanner(streamResp.Body)
	var fullResponse strings.Builder

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			result.Metrics.InterruptCount++
			return
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		// Parse streaming response line
		token, err := scp.parseStreamingLine(line)
		if err != nil {
			log.Printf("Warning: failed to parse streaming line: %v", err)
			continue
		}

		if token.Content == "" {
			continue
		}

		// Record first token timing
		if result.Metrics.FirstTokenTime.IsZero() {
			result.Metrics.FirstTokenTime = time.Now()
		}
		result.Metrics.TokenCount++

		// Send token for immediate visual feedback
		select {
		case result.VisualTokens <- token.Content:
		case <-ctx.Done():
			return
		}

		// Send to main token stream
		select {
		case result.TokenStream <- token.Content:
		case <-ctx.Done():
			return
		}

		// Add to full response for final parsing
		fullResponse.WriteString(token.Content)

		// Process through phrase buffer for audio synthesis
		phrase := phraseBuffer.AddToken(token.Content)
		if phrase != "" {
			if result.Metrics.FirstPhraseTime.IsZero() {
				result.Metrics.FirstPhraseTime = time.Now()
			}
			result.Metrics.PhraseCount++

			select {
			case result.AudioPhrases <- phrase:
			case <-ctx.Done():
				return
			}
		}

		// Check if this is the last token
		if token.IsLast {
			break
		}
	}

	// Flush any remaining tokens in buffer
	if remaining := phraseBuffer.Flush(); remaining != "" {
		result.Metrics.PhraseCount++
		select {
		case result.AudioPhrases <- remaining:
		case <-ctx.Done():
			return
		}
	}

	result.Metrics.CompletionTime = time.Now()

	// Parse final command from complete response
	if err := scanner.Err(); err != nil {
		result.ErrorChan <- fmt.Errorf("error reading streaming response: %w", err)
		return
	}

	// Parse the complete response into a command
	finalCommand, err := scp.parseFinalResponse(fullResponse.String())
	if err != nil {
		result.ErrorChan <- fmt.Errorf("error parsing final command: %w", err)
		return
	}

	// Send final command
	select {
	case result.FinalCommand <- finalCommand:
	case <-ctx.Done():
		return
	}

	log.Printf("🌊 Streaming command completed - Tokens: %d, Phrases: %d, Duration: %v",
		result.Metrics.TokenCount,
		result.Metrics.PhraseCount,
		result.Metrics.CompletionTime.Sub(result.Metrics.StartTime))
}

// AddToken adds a token to the buffer and returns a phrase if boundary is reached
func (pb *PhraseBuffer) AddToken(token string) string {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.tokens = append(pb.tokens, token)

	// Check for natural boundaries
	currentText := strings.Join(pb.tokens, "")
	for _, boundary := range pb.boundaries {
		if strings.HasSuffix(currentText, boundary) {
			return pb.flushLocked()
		}
	}

	// Check for buffer limits (only if we have some content and time has passed)
	if len(pb.tokens) >= pb.maxTokens ||
	   (len(pb.tokens) > 0 && !pb.lastFlushTime.IsZero() && time.Since(pb.lastFlushTime) >= pb.maxBufferTime) {
		return pb.flushLocked()
	}

	return ""
}

// Flush returns all buffered tokens as a phrase
func (pb *PhraseBuffer) Flush() string {
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.flushLocked()
}

// flushLocked flushes the buffer (must be called with lock held)
func (pb *PhraseBuffer) flushLocked() string {
	if len(pb.tokens) == 0 {
		return ""
	}

	phrase := strings.Join(pb.tokens, "")
	pb.tokens = pb.tokens[:0] // Reset slice but keep capacity
	pb.lastFlushTime = time.Now()

	return strings.TrimSpace(phrase)
}

// createStreamingRequest creates an HTTP request for streaming
func (scp *StreamingCommandParser) createStreamingRequest(ctx context.Context, prompt string) (*http.Response, error) {
	reqBody := OllamaRequest{
		Model:  scp.model,
		Prompt: prompt,
		Stream: true, // Enable streaming
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling streaming request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", scp.ollamaURL+"/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("error creating streaming request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")

	resp, err := scp.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making streaming request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("streaming request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// parseStreamingLine parses a single line from streaming response
func (scp *StreamingCommandParser) parseStreamingLine(line string) (*StreamingToken, error) {
	var ollamaResp OllamaResponse
	if err := json.Unmarshal([]byte(line), &ollamaResp); err != nil {
		return nil, fmt.Errorf("error unmarshaling streaming line: %w", err)
	}

	return &StreamingToken{
		Content:   ollamaResp.Response,
		Timestamp: time.Now(),
		IsLast:    ollamaResp.Done,
	}, nil
}

// buildStreamingPrompt creates a streaming-optimized prompt
func (scp *StreamingCommandParser) buildStreamingPrompt(transcription string) string {
	return fmt.Sprintf(`You are a voice assistant command parser. Analyze the following voice command and respond with a JSON object. Respond progressively as you process the command.

Voice command: "%s"

Classify this command and respond with ONLY a JSON object in this exact format:
{
  "intent": "one of: turn_on, turn_off, greeting, question, unknown",
  "entities": {
    "device": "lights, music, tv, etc. or empty string if none",
    "location": "bedroom, kitchen, living room, etc. or empty string if none"
  },
  "confidence": 0.95,
  "response": "A natural response to the user"
}

Rules:
- turn_on: user wants to turn something on (lights, music, etc.)
- turn_off: user wants to turn something off
- greeting: user is saying hello, hi, good morning, etc.
- question: user is asking a question
- unknown: unclear or unrecognized command
- confidence should be 0.0-1.0 based on how clear the intent is
- response should be natural and conversational
- Only respond with the JSON object, no other text`, transcription)
}

// parseFinalResponse parses the complete streaming response into a command
func (scp *StreamingCommandParser) parseFinalResponse(response string) (*Command, error) {
	// Use the existing parse logic from the original parser
	return scp.fallbackParser.parseResponse(response)
}

// createFallbackResult creates a result for non-streaming fallback
func (scp *StreamingCommandParser) createFallbackResult(cmd *Command) *StreamingResult {
	result := &StreamingResult{
		TokenStream:   make(chan string, 1),
		FinalCommand:  make(chan *Command, 1),
		ErrorChan:     make(chan error, 1),
		Cancel:        func() {}, // No-op cancel
		VisualTokens:  make(chan string, 1),
		AudioPhrases:  make(chan string, 1),
		Metrics:       &StreamingMetrics{StartTime: time.Now()},
	}

	// Send fallback data
	go func() {
		defer close(result.TokenStream)
		defer close(result.FinalCommand)
		defer close(result.VisualTokens)
		defer close(result.AudioPhrases)

		// Send complete response as single token
		result.VisualTokens <- cmd.Response
		result.AudioPhrases <- cmd.Response
		result.TokenStream <- cmd.Response
		result.FinalCommand <- cmd

		result.Metrics.FirstTokenTime = time.Now()
		result.Metrics.FirstPhraseTime = time.Now()
		result.Metrics.CompletionTime = time.Now()
		result.Metrics.TokenCount = 1
		result.Metrics.PhraseCount = 1
	}()

	return result
}

// createEmptyResult creates a result for empty input
func (scp *StreamingCommandParser) createEmptyResult() *StreamingResult {
	emptyCmd := &Command{
		Intent:     "unknown",
		Entities:   make(map[string]string),
		Confidence: 0.0,
		Response:   "I didn't hear anything.",
	}
	return scp.createFallbackResult(emptyCmd)
}

// TestStreamingConnection tests if Ollama streaming is working
func (scp *StreamingCommandParser) TestStreamingConnection() error {
	if !scp.enabled {
		return fmt.Errorf("streaming is disabled, using fallback parser")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with a simple prompt
	result, err := scp.ParseCommandStreaming(ctx, "hello")
	if err != nil {
		return fmt.Errorf("streaming test failed: %w", err)
	}

	// Wait for first token or error
	select {
	case <-result.TokenStream:
		result.Cancel()
		log.Printf("✅ Streaming Command Parser connected and working")
		return nil
	case err := <-result.ErrorChan:
		return fmt.Errorf("streaming test error: %w", err)
	case <-ctx.Done():
		result.Cancel()
		return fmt.Errorf("streaming test timeout")
	}
}