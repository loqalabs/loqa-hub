package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loqalabs/loqa-hub/internal/config"
)

// TestSTTLanguageParameterEndToEndIntegration verifies that the language parameter
// flows correctly from config through to HTTP requests
func TestSTTLanguageParameterEndToEndIntegration(t *testing.T) {
	tests := []struct {
		name           string
		configLanguage string
		expectedInHTTP string
	}{
		{
			name:           "Config English language flows to HTTP",
			configLanguage: "en",
			expectedInHTTP: "en",
		},
		{
			name:           "Config Spanish language flows to HTTP",
			configLanguage: "es",
			expectedInHTTP: "es",
		},
		{
			name:           "Config French language flows to HTTP",
			configLanguage: "fr",
			expectedInHTTP: "fr",
		},
		{
			name:           "Empty config language defaults in STT client",
			configLanguage: "",
			expectedInHTTP: "en", // STT client defaults empty string to "en"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedLanguage string

			// Create mock STT server that captures the language parameter
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}

				if strings.HasPrefix(r.URL.Path, "/v1/audio/transcriptions") {
					// Parse multipart form to capture language parameter
					err := r.ParseMultipartForm(32 << 20)
					if err != nil {
						t.Errorf("Failed to parse multipart form: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}

					capturedLanguage = r.FormValue("language")

					// Return successful response
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"text": "integration test response"}`))
					return
				}

				w.WriteHeader(http.StatusNotFound)
			}))
			defer mockServer.Close()

			// Create a test configuration (simulates global config loading)
			cfg := &config.Config{
				STT: config.STTConfig{
					URL:      mockServer.URL,
					Language: tt.configLanguage,
				},
			}

			// Create STT client using config (simulates server.go flow)
			client, err := NewSTTClient(cfg.STT.URL, cfg.STT.Language)
			if err != nil {
				t.Fatalf("NewSTTClient() error = %v", err)
			}

			// Perform transcription request (simulates actual usage)
			audioData := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
			result, err := client.Transcribe(audioData, 16000)
			if err != nil {
				t.Fatalf("Transcribe() error = %v", err)
			}

			// Verify transcription worked
			if result != "integration test response" {
				t.Errorf("Expected transcription result 'integration test response', got %q", result)
			}

			// Verify language parameter was transmitted correctly
			if capturedLanguage != tt.expectedInHTTP {
				t.Errorf("Language parameter in HTTP request = %q, want %q", capturedLanguage, tt.expectedInHTTP)
			}

			// Clean up
			_ = client.Close()
		})
	}
}

// TestSTTLanguageParameterWithConfidenceIntegration tests the language parameter
// with the TranscribeWithConfidence method
func TestSTTLanguageParameterWithConfidenceIntegration(t *testing.T) {
	var capturedLanguage string

	// Create mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/audio/transcriptions") {
			_ = r.ParseMultipartForm(32 << 20)
			capturedLanguage = r.FormValue("language")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"text": "Hey Loqa turn on the lights"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Test with Spanish language configuration
	cfg := &config.Config{
		STT: config.STTConfig{
			URL:      mockServer.URL,
			Language: "es",
		},
	}

	client, err := NewSTTClient(cfg.STT.URL, cfg.STT.Language)
	if err != nil {
		t.Fatalf("NewSTTClient() error = %v", err)
	}

	// Use TranscribeWithConfidence method
	audioData := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	result, err := client.TranscribeWithConfidence(audioData, 16000)
	if err != nil {
		t.Fatalf("TranscribeWithConfidence() error = %v", err)
	}

	// Verify the result
	if result.Text != "turn on the lights" { // Wake word should be stripped
		t.Errorf("Expected cleaned text 'turn on the lights', got %q", result.Text)
	}

	if !result.WakeWordDetected {
		t.Error("Expected wake word to be detected")
	}

	// Verify language parameter was sent correctly
	if capturedLanguage != "es" {
		t.Errorf("Language parameter in HTTP request = %q, want %q", capturedLanguage, "es")
	}

	_ = client.Close()
}
