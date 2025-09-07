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
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// STTClient implements the Transcriber interface using REST API calls
// to any OpenAI-compatible Speech-to-Text service
type STTClient struct {
	baseURL    string
	httpClient *http.Client
}

// OpenAI-compatible response struct
type transcriptionResponse struct {
	Text string `json:"text"`
}

// NewSTTClient creates a new OpenAI-compatible STT client
func NewSTTClient(baseURL string) (*STTClient, error) {
	if baseURL == "" {
		baseURL = "http://localhost:8000" // Default STT service address
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	s := &STTClient{
		baseURL:    baseURL,
		httpClient: client,
	}

	// Test connection with health check
	if err := s.healthCheck(); err != nil {
		return nil, fmt.Errorf("STT service health check failed: %w", err)
	}

	logging.Sugar.Infow("Connected to STT REST service", "base_url", baseURL)

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

	logging.Sugar.Infow("Sending transcription request",
		"request_id", requestID,
		"samples", len(audioData),
		"sample_rate", sampleRate,
	)

	// Convert float32 audio data to WAV bytes
	wavData, err := s.float32ToWAV(audioData, sampleRate)
	if err != nil {
		return "", fmt.Errorf("failed to convert audio to WAV: %w", err)
	}

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
	_ = writer.WriteField("language", "")  // Auto-detect
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
	logging.Sugar.Infow("Transcription completed",
		"request_id", requestID,
		"processing_time_ms", processingTime.Milliseconds(),
		"text_length", len(transcriptionResp.Text),
		"text", transcriptionResp.Text,
	)

	return transcriptionResp.Text, nil
}

// float32ToWAV converts float32 audio samples to WAV format bytes
func (s *STTClient) float32ToWAV(samples []float32, sampleRate int) ([]byte, error) {
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

	return buf.Bytes(), nil
}

// Close cleans up resources
func (s *STTClient) Close() error {
	logging.Sugar.Infow("Closing STT client", "base_url", s.baseURL)
	// HTTP client doesn't need explicit cleanup
	return nil
}
