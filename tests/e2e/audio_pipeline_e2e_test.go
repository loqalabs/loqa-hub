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

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/messaging"
)

// E2E Audio Pipeline Test Suite
// Tests the complete flow: STT → LLM → TTS → NATS → Relay

// MockSTTServer simulates an OpenAI-compatible STT service
type MockSTTServer struct {
	server    *httptest.Server
	mu        sync.RWMutex
	responses map[string]string
	callCount int
	lastAudio []byte
}

func NewMockSTTServer() *MockSTTServer {
	mock := &MockSTTServer{
		responses: make(map[string]string),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/audio/transcriptions", mock.handleTranscription)
	mux.HandleFunc("/health", mock.handleHealth)
	mock.server = httptest.NewServer(mux)

	// Set default responses
	mock.responses["default"] = "turn on the kitchen lights"

	return mock
}

func (m *MockSTTServer) handleTranscription(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	// Read the audio data
	err := r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No audio file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.lastAudio = audioData

	// Return transcription
	response := struct {
		Text string `json:"text"`
	}{
		Text: m.responses["default"],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *MockSTTServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "mock-stt",
	})
}

func (m *MockSTTServer) SetResponse(key, text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[key] = text
}

func (m *MockSTTServer) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

func (m *MockSTTServer) GetLastAudio() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastAudio
}

func (m *MockSTTServer) Close() {
	m.server.Close()
}

func (m *MockSTTServer) URL() string {
	return m.server.URL
}

// MockTTSServer simulates an OpenAI-compatible TTS service
type MockTTSServer struct {
	server       *httptest.Server
	mu           sync.RWMutex
	audioContent []byte
	callCount    int
	lastText     string
	lastVoice    string
	lastFormat   string
}

func NewMockTTSServer() *MockTTSServer {
	mock := &MockTTSServer{
		audioContent: generateMockWAVData(22050, 2), // 2 seconds of 22.05kHz audio
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/audio/speech", mock.handleSpeech)
	mux.HandleFunc("/audio/speech", mock.handleSpeech)
	mux.HandleFunc("/health", mock.handleHealth)
	mux.HandleFunc("/audio/voices", mock.handleVoices)
	mock.server = httptest.NewServer(mux)

	return mock
}

func (m *MockTTSServer) handleSpeech(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callCount++

	// Parse request body
	var request struct {
		Model  string  `json:"model"`
		Input  string  `json:"input"`
		Voice  string  `json:"voice"`
		Format string  `json:"response_format"`
		Speed  float64 `json:"speed"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.lastText = request.Input
	m.lastVoice = request.Voice
	m.lastFormat = request.Format

	// Return mock audio based on format
	w.Header().Set("Content-Type", "audio/wav")
	w.Write(m.audioContent)
}

func (m *MockTTSServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "mock-tts",
	})
}

func (m *MockTTSServer) handleVoices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": []map[string]string{
			{
				"id":          "af_bella",
				"name":        "Bella",
				"preview_url": "https://api.openai.com/v1/audio/speech/preview/af_bella",
			},
			{
				"id":          "af_sarah",
				"name":        "Sarah",
				"preview_url": "https://api.openai.com/v1/audio/speech/preview/af_sarah",
			},
		},
	})
}

func (m *MockTTSServer) SetAudioContent(content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioContent = content
}

func (m *MockTTSServer) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

func (m *MockTTSServer) GetLastRequest() (text, voice, format string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastText, m.lastVoice, m.lastFormat
}

func (m *MockTTSServer) Close() {
	m.server.Close()
}

func (m *MockTTSServer) URL() string {
	return m.server.URL
}

// generateMockWAVData creates a simple WAV file with sine wave data
func generateMockWAVData(sampleRate int, durationSeconds int) []byte {
	numSamples := sampleRate * durationSeconds

	// WAV header (44 bytes)
	header := []byte{
		// RIFF header
		0x52, 0x49, 0x46, 0x46, // "RIFF"
		0x00, 0x00, 0x00, 0x00, // Chunk size (will be filled later)
		0x57, 0x41, 0x56, 0x45, // "WAVE"

		// fmt subchunk
		0x66, 0x6D, 0x74, 0x20, // "fmt "
		0x10, 0x00, 0x00, 0x00, // Subchunk size (16)
		0x01, 0x00, // Audio format (PCM)
		0x01, 0x00, // Number of channels (1)
		0x22, 0x56, 0x00, 0x00, // Sample rate (22050)
		0x44, 0xAC, 0x00, 0x00, // Byte rate
		0x02, 0x00, // Block align
		0x10, 0x00, // Bits per sample (16)

		// data subchunk
		0x64, 0x61, 0x74, 0x61, // "data"
		0x00, 0x00, 0x00, 0x00, // Subchunk size (will be filled later)
	}

	// Generate sine wave data (16-bit samples)
	audioData := make([]byte, numSamples*2) // 2 bytes per sample
	for i := 0; i < numSamples; i++ {
		// Simple sine wave
		sample := int16(32767 / 2) // Half volume
		audioData[i*2] = byte(sample & 0xFF)
		audioData[i*2+1] = byte((sample >> 8) & 0xFF)
	}

	// Update chunk sizes in header
	totalSize := len(header) + len(audioData) - 8
	header[4] = byte(totalSize & 0xFF)
	header[5] = byte((totalSize >> 8) & 0xFF)
	header[6] = byte((totalSize >> 16) & 0xFF)
	header[7] = byte((totalSize >> 24) & 0xFF)

	dataSize := len(audioData)
	header[40] = byte(dataSize & 0xFF)
	header[41] = byte((dataSize >> 8) & 0xFF)
	header[42] = byte((dataSize >> 16) & 0xFF)
	header[43] = byte((dataSize >> 24) & 0xFF)

	return append(header, audioData...)
}

// MockNATSServer simulates NATS message delivery
type MockNATSServer struct {
	mu       sync.RWMutex
	messages map[string][]messaging.AudioStreamMessage
	relayID  string
}

func NewMockNATSServer() *MockNATSServer {
	return &MockNATSServer{
		messages: make(map[string][]messaging.AudioStreamMessage),
	}
}

func (m *MockNATSServer) PublishAudio(subject string, message messaging.AudioStreamMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.messages[subject] == nil {
		m.messages[subject] = make([]messaging.AudioStreamMessage, 0)
	}
	m.messages[subject] = append(m.messages[subject], message)

	return nil
}

func (m *MockNATSServer) GetMessages(subject string) []messaging.AudioStreamMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.messages[subject]
}

func (m *MockNATSServer) GetMessageCount(subject string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages[subject])
}

func (m *MockNATSServer) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make(map[string][]messaging.AudioStreamMessage)
}

// TestE2EAudioPipeline tests the complete audio processing pipeline
func TestE2EAudioPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Setup mock services
	sttServer := NewMockSTTServer()
	defer sttServer.Close()

	ttsServer := NewMockTTSServer()
	defer ttsServer.Close()

	mockNATS := NewMockNATSServer()

	// Configure STT client
	sttConfig := &config.STTConfig{
		URL:      sttServer.URL(),
		Language: "en",
	}

	sttClient, err := llm.NewSTTClient(sttConfig.URL, sttConfig.Language)
	if err != nil {
		t.Fatalf("Failed to create STT client: %v", err)
	}

	// Configure TTS client
	ttsConfig := config.TTSConfig{
		URL:            ttsServer.URL(),
		Voice:          "test-voice",
		Speed:          1.0,
		ResponseFormat: "wav",
		Timeout:        10 * time.Second,
		MaxConcurrent:  10,
		Normalize:      true,
	}

	ttsClient, err := llm.NewOpenAITTSClient(ttsConfig)
	if err != nil {
		t.Fatalf("Failed to create TTS client: %v", err)
	}

	tests := []struct {
		name          string
		inputAudio    []byte
		expectedText  string
		expectedAudio bool
		relayID       string
		messageType   string
		priority      int
	}{
		{
			name:          "basic_voice_command",
			inputAudio:    generateMockWAVData(22050, 1),
			expectedText:  "turn on the kitchen lights",
			expectedAudio: true,
			relayID:       "relay-001",
			messageType:   "response",
			priority:      1,
		},
		{
			name:          "timer_notification",
			inputAudio:    generateMockWAVData(22050, 2),
			expectedText:  "timer notification",
			expectedAudio: true,
			relayID:       "relay-002",
			messageType:   "timer",
			priority:      2,
		},
		{
			name:          "system_message",
			inputAudio:    generateMockWAVData(22050, 1),
			expectedText:  "system ready",
			expectedAudio: true,
			relayID:       "relay-003",
			messageType:   "system",
			priority:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous messages
			mockNATS.Clear()

			// Set expected STT response
			sttServer.SetResponse("default", tt.expectedText)

			// Step 1: Speech-to-Text
			transcriptionResult, err := sttClient.TranscribeWithConfidence(
				bytesToFloat32(tt.inputAudio),
				22050,
			)
			if err != nil {
				t.Fatalf("STT failed: %v", err)
			}

			if transcriptionResult.Text != tt.expectedText {
				t.Errorf("STT text mismatch: got %s, want %s",
					transcriptionResult.Text, tt.expectedText)
			}

			// Step 2: Text-to-Speech (simulate LLM response)
			responseText := "Understood, " + tt.expectedText
			ttsResult, err := ttsClient.Synthesize(responseText, nil)
			if err != nil {
				t.Fatalf("TTS failed: %v", err)
			}
			defer func() {
				if ttsResult.Cleanup != nil {
					ttsResult.Cleanup()
				}
			}()

			// Read audio data from the result
			audioData, err := io.ReadAll(ttsResult.Audio)
			if err != nil {
				t.Fatalf("Failed to read TTS audio: %v", err)
			}

			if len(audioData) == 0 {
				t.Error("TTS returned empty audio data")
			}

			// Step 3: NATS Audio Streaming
			audioMessage := messaging.AudioStreamMessage{
				StreamID:    "e2e-test-" + tt.name,
				AudioData:   audioData,
				AudioFormat: "wav",
				SampleRate:  22050,
				MessageType: tt.messageType,
				Priority:    tt.priority,
			}

			subject := "audio.stream." + tt.relayID
			err = mockNATS.PublishAudio(subject, audioMessage)
			if err != nil {
				t.Fatalf("NATS publish failed: %v", err)
			}

			// Step 4: Verify end-to-end pipeline
			if tt.expectedAudio {
				messages := mockNATS.GetMessages(subject)
				if len(messages) != 1 {
					t.Errorf("Expected 1 NATS message, got %d", len(messages))
				} else {
					msg := messages[0]
					if len(msg.AudioData) != len(audioData) {
						t.Errorf("Audio data length mismatch: got %d, want %d",
							len(msg.AudioData), len(audioData))
					}
					if msg.AudioFormat != "wav" {
						t.Errorf("Audio format mismatch: got %s, want wav", msg.AudioFormat)
					}
					if msg.SampleRate != 22050 {
						t.Errorf("Sample rate mismatch: got %d, want 22050", msg.SampleRate)
					}
					if msg.MessageType != tt.messageType {
						t.Errorf("Message type mismatch: got %s, want %s",
							msg.MessageType, tt.messageType)
					}
					if msg.Priority != tt.priority {
						t.Errorf("Priority mismatch: got %d, want %d",
							msg.Priority, tt.priority)
					}
				}
			}

			// Verify service call counts
			if sttServer.GetCallCount() == 0 {
				t.Error("STT server was not called")
			}

			if ttsServer.GetCallCount() == 0 {
				t.Error("TTS server was not called")
			}

			// Verify TTS request parameters
			lastText, lastVoice, lastFormat := ttsServer.GetLastRequest()
			if lastText != responseText {
				t.Errorf("TTS text mismatch: got %s, want %s", lastText, responseText)
			}
			if lastVoice != "test-voice" {
				t.Errorf("TTS voice mismatch: got %s, want test-voice", lastVoice)
			}
			if lastFormat != "wav" {
				t.Errorf("TTS format mismatch: got %s, want wav", lastFormat)
			}
		})
	}
}

func TestE2EAudioPipeline_ErrorHandling(t *testing.T) {
	// Test STT service failure
	t.Run("stt_service_failure", func(t *testing.T) {
		// Create server that handles health checks but fails on transcription
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
				return
			}
			http.Error(w, "STT service unavailable", http.StatusInternalServerError)
		}))
		defer errorServer.Close()

		sttClient, err := llm.NewSTTClientWithOptions(errorServer.URL, "en", true)
		if err != nil {
			t.Fatalf("Failed to create STT client: %v", err)
		}
		audioData := generateMockWAVData(22050, 1)

		_, err = sttClient.TranscribeWithConfidence(bytesToFloat32(audioData), 22050)
		if err == nil {
			t.Error("Expected STT error but got none")
		}
	})

	// Test TTS service failure
	t.Run("tts_service_failure", func(t *testing.T) {
		// Create server that handles health checks but fails on synthesis
		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/audio/voices" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"data": []map[string]string{
						{"id": "test-voice", "name": "Test Voice"},
					},
				})
				return
			}
			http.Error(w, "TTS service unavailable", http.StatusInternalServerError)
		}))
		defer errorServer.Close()

		ttsConfig := config.TTSConfig{
			URL:           errorServer.URL,
			Timeout:       1 * time.Second,
			MaxConcurrent: 1,
		}

		ttsClient, err := llm.NewOpenAITTSClient(ttsConfig)
		if err != nil {
			t.Fatalf("Failed to create TTS client: %v", err)
		}

		_, err = ttsClient.Synthesize("test text", nil)
		if err == nil {
			t.Error("Expected TTS error but got none")
		}
	})
}

func TestE2EAudioPipeline_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	sttServer := NewMockSTTServer()
	defer sttServer.Close()

	ttsServer := NewMockTTSServer()
	defer ttsServer.Close()

	sttConfig := &config.STTConfig{
		URL:      sttServer.URL(),
		Language: "en",
	}

	ttsConfig := config.TTSConfig{
		URL:            ttsServer.URL(),
		Voice:          "test-voice",
		ResponseFormat: "wav",
		Timeout:        10 * time.Second,
		MaxConcurrent:  10,
	}

	sttClient, err := llm.NewSTTClient(sttConfig.URL, sttConfig.Language)
	if err != nil {
		t.Fatalf("Failed to create STT client: %v", err)
	}
	ttsClient, err := llm.NewOpenAITTSClient(ttsConfig)
	if err != nil {
		t.Fatalf("Failed to create TTS client: %v", err)
	}

	// Test concurrent pipeline processing
	numConcurrent := 5
	var wg sync.WaitGroup
	errors := make(chan error, numConcurrent)

	start := time.Now()

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			audioData := generateMockWAVData(22050, 1)

			// STT
			transcription, err := sttClient.TranscribeWithConfidence(
				bytesToFloat32(audioData),
				22050,
			)
			if err != nil {
				errors <- err
				return
			}

			// TTS
			responseText := "Response " + transcription.Text
			_, err = ttsClient.Synthesize(responseText, nil)
			if err != nil {
				errors <- err
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent processing error: %v", err)
	}

	// Performance expectations (these are generous for mock services)
	maxExpectedDuration := 5 * time.Second
	if duration > maxExpectedDuration {
		t.Errorf("Pipeline too slow: %v > %v", duration, maxExpectedDuration)
	}

	t.Logf("Processed %d concurrent pipelines in %v", numConcurrent, duration)
}

func TestE2EAudioPipeline_SampleRateConversion(t *testing.T) {
	sttServer := NewMockSTTServer()
	defer sttServer.Close()

	ttsServer := NewMockTTSServer()
	defer ttsServer.Close()

	sttConfig := &config.STTConfig{
		URL:      sttServer.URL(),
		Language: "en",
	}

	ttsConfig := config.TTSConfig{
		URL:            ttsServer.URL(),
		ResponseFormat: "wav",
		Timeout:        10 * time.Second,
		MaxConcurrent:  10,
	}

	sttClient, err := llm.NewSTTClient(sttConfig.URL, sttConfig.Language)
	if err != nil {
		t.Fatalf("Failed to create STT client: %v", err)
	}
	ttsClient, err := llm.NewOpenAITTSClient(ttsConfig)
	if err != nil {
		t.Fatalf("Failed to create TTS client: %v", err)
	}

	// Test different sample rates
	sampleRates := []int{16000, 22050, 44100, 48000}

	for _, sampleRate := range sampleRates {
		t.Run(fmt.Sprintf("sample_rate_%d", sampleRate), func(t *testing.T) {
			audioData := generateMockWAVData(sampleRate, 1)

			// Test STT with various sample rates
			_, err := sttClient.TranscribeWithConfidence(
				bytesToFloat32(audioData),
				sampleRate,
			)
			if err != nil {
				t.Errorf("STT failed for sample rate %d: %v", sampleRate, err)
			}

			// Test TTS (should always output at configured rate)
			_, err = ttsClient.Synthesize("test text", nil)
			if err != nil {
				t.Errorf("TTS failed for sample rate %d context: %v", sampleRate, err)
			}
		})
	}
}

// Helper function to convert bytes to float32 array (simplified)
func bytesToFloat32(data []byte) []float32 {
	// Skip WAV header (44 bytes) and convert 16-bit samples to float32
	if len(data) < 44 {
		return []float32{}
	}

	audioBytes := data[44:] // Skip WAV header
	numSamples := len(audioBytes) / 2
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		// Convert 16-bit little-endian to float32
		sample := int16(audioBytes[i*2]) | (int16(audioBytes[i*2+1]) << 8)
		samples[i] = float32(sample) / 32768.0
	}

	return samples
}
