/*
Copyright (c) 2024 Loqa Labs

Licensed under the AGPLv3 License.
This file is part of the loqa-hub.
*/

package llm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// STTClient implements the Transcriber interface using REST API calls
// to any OpenAI-compatible Speech-to-Text service
type STTClient struct {
	baseURL    string
	language   string
	httpClient *http.Client
}

// OpenAI-compatible response struct
type transcriptionResponse struct {
	Text string `json:"text"`
}

// PostProcessingResult contains the result of transcription post-processing
type PostProcessingResult struct {
	CleanedText        string
	OriginalText       string
	WakeWordDetected   bool
	WakeWordVariant    string
	ConfidenceEstimate float64
	NeedsConfirmation  bool
}

// NewSTTClient creates a new OpenAI-compatible STT client
func NewSTTClient(baseURL, language string) (*STTClient, error) {
	return NewSTTClientWithOptions(baseURL, language, true)
}

// NewSTTClientWithOptions creates a new OpenAI-compatible STT client with configurable health check
func NewSTTClientWithOptions(baseURL, language string, enableHealthCheck bool) (*STTClient, error) {
	if baseURL == "" {
		baseURL = "http://localhost:8000" // Default STT service address
	}

	if language == "" {
		language = "en" // Default language
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	s := &STTClient{
		baseURL:    baseURL,
		language:   language,
		httpClient: client,
	}

	// Test connection with health check (skip for testing)
	if enableHealthCheck {
		if err := s.healthCheck(); err != nil {
			return nil, fmt.Errorf("STT service health check failed: %w", err)
		}
	}

	if logging.Sugar != nil {
		logging.Sugar.Infow("Connected to STT REST service", "base_url", baseURL, "language", language)
	}

	return s, nil
}

// healthCheck verifies the service is running
func (s *STTClient) healthCheck() error {
	resp, err := s.httpClient.Get(s.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("failed to connect to STT service at %s: %w", s.baseURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("STT service health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Transcribe implements the Transcriber interface
func (s *STTClient) Transcribe(audioData []float32, sampleRate int) (string, error) {
	if len(audioData) == 0 {
		return "", fmt.Errorf("empty audio data")
	}

	if sampleRate <= 0 {
		return "", fmt.Errorf("invalid sample rate: %d", sampleRate)
	}

	startTime := time.Now()
	requestID := fmt.Sprintf("req_%d", startTime.UnixNano())

	if logging.Sugar != nil {
		logging.Sugar.Infow("Sending transcription request",
			"request_id", requestID,
			"samples", len(audioData),
			"sample_rate", sampleRate,
		)
	}

	// Convert float32 audio data to WAV bytes
	wavData := s.float32ToWAV(audioData, sampleRate)

	// Create multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add the audio file
	audioWriter, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := audioWriter.Write(wavData); err != nil {
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	// Add optional parameters
	_ = writer.WriteField("model", "tiny") // Use the model loaded in STT service
	_ = writer.WriteField("language", s.language)  // Use configured language
	_ = writer.WriteField("temperature", "0.0")
	_ = writer.WriteField("response_format", "json")

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make the request
	req, err := http.NewRequest("POST", s.baseURL+"/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription HTTP request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("transcription failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var transcriptionResp transcriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcriptionResp); err != nil {
		return "", fmt.Errorf("failed to parse transcription response: %w", err)
	}

	processingTime := time.Since(startTime)
	// Post-process the transcription
	processResult := s.postProcessTranscription(transcriptionResp.Text)

	if logging.Sugar != nil {
		logging.Sugar.Infow("Transcription completed",
			"request_id", requestID,
			"processing_time_ms", processingTime.Milliseconds(),
			"original_text", processResult.OriginalText,
			"cleaned_text", processResult.CleanedText,
			"wake_word_detected", processResult.WakeWordDetected,
			"wake_word_variant", processResult.WakeWordVariant,
			"confidence_estimate", processResult.ConfidenceEstimate,
			"needs_confirmation", processResult.NeedsConfirmation,
		)
	}

	// Return cleaned text for intent parsing
	return processResult.CleanedText, nil
}

// TranscribeWithConfidence implements the enhanced Transcriber interface
func (s *STTClient) TranscribeWithConfidence(audioData []float32, sampleRate int) (*TranscriptionResult, error) {
	if len(audioData) == 0 {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("empty audio data")
	}

	if sampleRate <= 0 {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("invalid sample rate: %d", sampleRate)
	}

	startTime := time.Now()
	requestID := fmt.Sprintf("req_%d", startTime.UnixNano())

	if logging.Sugar != nil {
		logging.Sugar.Infow("Sending transcription request with confidence",
			"request_id", requestID,
			"samples", len(audioData),
			"sample_rate", sampleRate,
		)
	}

	// Convert float32 audio data to WAV bytes
	wavData := s.float32ToWAV(audioData, sampleRate)

	// Create multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add the audio file
	audioWriter, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := audioWriter.Write(wavData); err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("failed to write audio data: %w", err)
	}

	// Add optional parameters
	_ = writer.WriteField("model", "tiny") // Use the model loaded in STT service
	_ = writer.WriteField("language", s.language)  // Use configured language
	_ = writer.WriteField("temperature", "0.0")
	_ = writer.WriteField("response_format", "json")

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Make the request
	req, err := http.NewRequest("POST", s.baseURL+"/v1/audio/transcriptions", &requestBody)
	if err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("transcription HTTP request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("transcription failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var transcriptionResp transcriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcriptionResp); err != nil {
		return &TranscriptionResult{
			Text:               "",
			ConfidenceEstimate: 0.0,
			WakeWordDetected:   false,
			WakeWordVariant:    "",
			NeedsConfirmation:  true,
		}, fmt.Errorf("failed to parse transcription response: %w", err)
	}

	// Post-process the transcription
	processResult := s.postProcessTranscription(transcriptionResp.Text)

	processingTime := time.Since(startTime)
	if logging.Sugar != nil {
		logging.Sugar.Infow("Transcription with confidence completed",
			"request_id", requestID,
			"processing_time_ms", processingTime.Milliseconds(),
			"original_text", processResult.OriginalText,
			"cleaned_text", processResult.CleanedText,
			"wake_word_detected", processResult.WakeWordDetected,
			"wake_word_variant", processResult.WakeWordVariant,
			"confidence_estimate", processResult.ConfidenceEstimate,
			"needs_confirmation", processResult.NeedsConfirmation,
		)
	}

	return &TranscriptionResult{
		Text:               processResult.CleanedText,
		ConfidenceEstimate: processResult.ConfidenceEstimate,
		WakeWordDetected:   processResult.WakeWordDetected,
		WakeWordVariant:    processResult.WakeWordVariant,
		NeedsConfirmation:  processResult.NeedsConfirmation,
	}, nil
}

// float32ToWAV converts float32 audio samples to WAV format bytes
func (s *STTClient) float32ToWAV(samples []float32, sampleRate int) []byte {
	// Simple WAV header for 32-bit float PCM
	numSamples := len(samples)
	dataSize := numSamples * 4 // 4 bytes per float32 sample
	fileSize := 36 + dataSize

	var buf bytes.Buffer

	// WAV header
	buf.WriteString("RIFF")                                                                                    // ChunkID
	buf.Write([]byte{byte(fileSize), byte(fileSize >> 8), byte(fileSize >> 16), byte(fileSize >> 24)})         // ChunkSize
	buf.WriteString("WAVE")                                                                                    // Format
	buf.WriteString("fmt ")                                                                                    // Subchunk1ID
	buf.Write([]byte{16, 0, 0, 0})                                                                             // Subchunk1Size (16 for PCM)
	buf.Write([]byte{3, 0})                                                                                    // AudioFormat (3 = IEEE float)
	buf.Write([]byte{1, 0})                                                                                    // NumChannels (1 = mono)
	buf.Write([]byte{byte(sampleRate), byte(sampleRate >> 8), byte(sampleRate >> 16), byte(sampleRate >> 24)}) // SampleRate
	byteRate := sampleRate * 4                                                                                 // SampleRate * NumChannels * BitsPerSample/8
	buf.Write([]byte{byte(byteRate), byte(byteRate >> 8), byte(byteRate >> 16), byte(byteRate >> 24)})         // ByteRate
	buf.Write([]byte{4, 0})                                                                                    // BlockAlign (NumChannels * BitsPerSample/8)
	buf.Write([]byte{32, 0})                                                                                   // BitsPerSample (32 for float32)
	buf.WriteString("data")                                                                                    // Subchunk2ID
	buf.Write([]byte{byte(dataSize), byte(dataSize >> 8), byte(dataSize >> 16), byte(dataSize >> 24)})         // Subchunk2Size

	// Audio data (float32 samples as little-endian bytes)
	for _, sample := range samples {
		bits := math.Float32bits(sample)
		binaryData := make([]byte, 4)
		binary.LittleEndian.PutUint32(binaryData, bits)
		buf.Write(binaryData)
	}

	return buf.Bytes()
}

// postProcessTranscription handles wake word stripping, normalization, and confidence estimation
func (s *STTClient) postProcessTranscription(rawText string) *PostProcessingResult {
	result := &PostProcessingResult{
		OriginalText:       rawText,
		CleanedText:        rawText,
		WakeWordDetected:   false,
		WakeWordVariant:    "",
		ConfidenceEstimate: s.estimateConfidence(rawText),
		NeedsConfirmation:  false,
	}

	// Normalize and detect wake word variants
	lowerText := strings.ToLower(strings.TrimSpace(rawText))

	// Define wake word patterns (from most to least specific)
	wakeWordPatterns := []struct {
		pattern string
		variant string
	}{
		{"hey loqa", "hey loqa"},
		{"hey loca", "hey loca"},
		{"hey luka", "hey luka"},
		{"hey luca", "hey luca"},
		{"hey logic", "hey logic"},
		{"hey local", "hey local"},
		{"loqa", "loqa"},
		{"loca", "loca"},
		{"luka", "luka"},
		{"luca", "luca"},
	}

	// Find and strip wake word
	for _, ww := range wakeWordPatterns {
		if strings.HasPrefix(lowerText, ww.pattern) {
			result.WakeWordDetected = true
			result.WakeWordVariant = ww.variant

			// Strip wake word from beginning
			remaining := strings.TrimSpace(rawText[len(ww.pattern):])

			// Remove common separators after wake word
			remaining = strings.TrimLeft(remaining, " ,.!?")

			result.CleanedText = remaining
			break
		}
	}

	// Check if confidence is too low (requires confirmation)
	result.NeedsConfirmation = result.ConfidenceEstimate < 0.6

	// Special case: if we stripped wake word but nothing remains
	if result.WakeWordDetected && strings.TrimSpace(result.CleanedText) == "" {
		result.CleanedText = ""
		result.NeedsConfirmation = true
	}

	return result
}

// estimateConfidence provides a rough confidence estimate based on text characteristics
func (s *STTClient) estimateConfidence(text string) float64 {
	if text == "" {
		return 0.0
	}

	confidence := 0.8 // Base confidence

	// Reduce confidence for very short utterances
	if len(text) < 3 {
		confidence -= 0.3
	}

	// Reduce confidence for nonsensical character patterns
	if strings.Contains(text, "...") || strings.Contains(text, "???") {
		confidence -= 0.2
	}

	// Reduce confidence for repeated characters (stammering indicators)
	words := strings.Fields(text)
	for _, word := range words {
		if len(word) > 2 {
			// Check if the word has excessive repetition (like "aaaaaah")
			charCounts := make(map[rune]int)
			for _, c := range word {
				charCounts[c]++
			}

			// If any character appears more than 60% of the word, it's likely repetition
			for _, count := range charCounts {
				if float64(count)/float64(len(word)) > 0.6 {
					confidence -= 0.3
					break
				}
			}
		}
	}

	// Increase confidence for common wake word patterns
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "hey") || strings.Contains(lowerText, "loqa") {
		confidence += 0.1
	}

	// Ensure confidence stays in valid range
	if confidence < 0.0 {
		confidence = 0.0
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// Close cleans up resources
func (s *STTClient) Close() error {
	if logging.Sugar != nil {
		logging.Sugar.Infow("Closing STT client", "base_url", s.baseURL)
	}
	// HTTP client doesn't need explicit cleanup
	return nil
}
