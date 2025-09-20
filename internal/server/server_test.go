package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loqalabs/loqa-hub/internal/arbitration"
	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/tiers"
	"github.com/loqalabs/loqa-hub/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// timeToMicros safely converts time to microseconds
func timeToMicros(t time.Time) uint64 {
	micros := t.UnixNano() / 1000
	if micros < 0 {
		return 0
	}
	return uint64(micros)
}

func TestNew_WithHTTPStreamingConfig(t *testing.T) {
	// Initialize logging for test
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	// Create a test configuration for new HTTP/1.1 streaming architecture
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "0.0.0.0",
			Port:         3000,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
		STT: config.STTConfig{
			URL:         "http://localhost:8000",
			Language:    "en",
			Temperature: 0.0,
		},
		TTS: config.TTSConfig{
			URL:             "http://localhost:8880/v1",
			Voice:           "af_bella",
			Speed:           1.0,
			ResponseFormat:  "mp3",
			Normalize:       true,
			MaxConcurrent:   10,
			Timeout:         10 * time.Second,
			FallbackEnabled: true,
		},
		Streaming: config.StreamingConfig{
			Enabled:             false, // Disabled for testing
			OllamaURL:           "http://localhost:11434",
			Model:               "llama3.2:3b",
			MaxBufferTime:       2 * time.Second,
			MaxTokensPerPhrase:  50,
			AudioConcurrency:    3,
			VisualFeedbackDelay: 50 * time.Millisecond,
			InterruptTimeout:    500 * time.Millisecond,
			FallbackEnabled:     true,
			MetricsEnabled:      false, // Disabled for testing
		},
		NATS: config.NATSConfig{
			URL:           "nats://localhost:4222",
			Subject:       "loqa.test",
			MaxReconnect:  10,
			ReconnectWait: 2 * time.Second,
		},
	}

	// Test that server creation doesn't panic and returns a valid server
	// Use NewWithOptions to skip health checks during testing
	server := NewWithOptions(cfg, false)
	if server == nil {
		t.Fatal("New() returned nil server")
	}

	// Verify server has the expected configuration
	if server.cfg != cfg {
		t.Error("Server configuration not set correctly")
	}

	// Verify server components are initialized
	if server.mux == nil {
		t.Error("Server mux not initialized")
	}

	if server.streamTransport == nil {
		t.Error("Server HTTP streaming transport not initialized")
	}

	if server.arbitrator == nil {
		t.Error("Server arbitrator not initialized")
	}

	if server.intentProcessor == nil {
		t.Error("Server intent processor not initialized")
	}

	if server.tierDetector == nil {
		t.Error("Server tier detector not initialized")
	}

	// Note: audioService may be nil if STT/TTS services are not available during tests
	// This is acceptable for unit testing server initialization
}

func TestNew_STTLanguageConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		language string
		expected string
	}{
		{
			name:     "English language",
			language: "en",
			expected: "en",
		},
		{
			name:     "Spanish language",
			language: "es",
			expected: "es",
		},
		{
			name:     "French language",
			language: "fr",
			expected: "fr",
		},
		{
			name:     "Empty language passed through as-is",
			language: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize logging for test
			if err := logging.Initialize(); err != nil {
				t.Fatalf("Failed to initialize logging: %v", err)
			}
			defer logging.Close()
			cfg := &config.Config{
				Server: config.ServerConfig{
					Host:         "0.0.0.0",
					Port:         3000,
					ReadTimeout:  15 * time.Second,
					WriteTimeout: 15 * time.Second,
				},
				STT: config.STTConfig{
					URL:         "http://localhost:8000",
					Language:    tt.language,
					Temperature: 0.0,
				},
				TTS: config.TTSConfig{
					URL:             "http://localhost:8880/v1",
					Voice:           "af_bella",
					Speed:           1.0,
					ResponseFormat:  "mp3",
					Normalize:       true,
					MaxConcurrent:   10,
					Timeout:         10 * time.Second,
					FallbackEnabled: true,
				},
			}

			// Test that server creation works with different language configurations
			// Use NewWithOptions to skip health checks during testing
			server := NewWithOptions(cfg, false)
			if server == nil {
				t.Fatal("New() returned nil server")
			}

			// Verify the language configuration is passed through
			if server.cfg.STT.Language != tt.expected {
				t.Errorf("Expected STT language %q, got %q", tt.expected, server.cfg.STT.Language)
			}
		})
	}
}

// Helper function to create a test configuration
func createTestConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:         "0.0.0.0",
			Port:         3000,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
		STT: config.STTConfig{
			URL:         "http://localhost:8000",
			Language:    "en",
			Temperature: 0.0,
		},
		TTS: config.TTSConfig{
			URL:             "http://localhost:8880/v1",
			Voice:           "af_bella",
			Speed:           1.0,
			ResponseFormat:  "mp3",
			Normalize:       true,
			MaxConcurrent:   10,
			Timeout:         10 * time.Second,
			FallbackEnabled: true,
		},
		Streaming: config.StreamingConfig{
			Enabled:             false,
			OllamaURL:           "http://localhost:11434",
			Model:               "llama3.2:3b",
			MaxBufferTime:       2 * time.Second,
			MaxTokensPerPhrase:  50,
			AudioConcurrency:    3,
			VisualFeedbackDelay: 50 * time.Millisecond,
			InterruptTimeout:    500 * time.Millisecond,
			FallbackEnabled:     true,
			MetricsEnabled:      false,
		},
		NATS: config.NATSConfig{
			URL:           "nats://localhost:4222",
			Subject:       "loqa.test",
			MaxReconnect:  10,
			ReconnectWait: 2 * time.Second,
		},
	}
}

// Test health endpoint
func TestHandleHealth(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var health map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &health)
	require.NoError(t, err)

	// Verify health response structure
	assert.Equal(t, "ok", health["status"])
	assert.Contains(t, health, "timestamp")
	assert.Equal(t, "HTTP/1.1 Binary Streaming", health["architecture"])
	assert.Contains(t, health, "tier")
	assert.Contains(t, health, "services")
	assert.Contains(t, health, "degraded")
}

// Test capabilities endpoint
func TestHandleCapabilities(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET request succeeds",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST request fails",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT request fails",
			method:         "PUT",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/capabilities", nil)
			w := httptest.NewRecorder()

			server.handleCapabilities(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var capabilities map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &capabilities)
				require.NoError(t, err)

				// Verify capabilities response structure
				assert.Contains(t, capabilities, "tier")
				assert.Contains(t, capabilities, "services")
				assert.Contains(t, capabilities, "degraded")
			}
		})
	}
}

// Test arbitration stats endpoint
func TestHandleArbitrationStats(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET request succeeds",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST request fails",
			method:         "POST",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/arbitration/stats", nil)
			w := httptest.NewRecorder()

			server.handleArbitrationStats(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var stats map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &stats)
				require.NoError(t, err)

				// Verify stats response structure (arbitrator provides stats)
				assert.NotNil(t, stats)
			}
		})
	}
}

// Test tier info endpoint
func TestHandleTierInfo(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET request succeeds",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "DELETE request fails",
			method:         "DELETE",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/tier", nil)
			w := httptest.NewRecorder()

			server.handleTierInfo(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var tierInfo map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &tierInfo)
				require.NoError(t, err)

				// Verify tier info response structure
				assert.Contains(t, tierInfo, "current_tier")
				assert.Contains(t, tierInfo, "features")

				features := tierInfo["features"].(map[string]interface{})
				assert.Contains(t, features, "local_llm")
				assert.Contains(t, features, "streaming_responses")
				assert.Contains(t, features, "reflex_only")
			}
		})
	}
}

// Test intent processing endpoint
func TestHandleIntentProcessing(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
	}{
		{
			name:           "Valid POST request",
			method:         "POST",
			body:           `{"text": "turn on the lights"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET request fails",
			method:         "GET",
			body:           "",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid JSON",
			method:         "POST",
			body:           `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Empty text",
			method:         "POST",
			body:           `{"text": ""}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/intent/process", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleIntentProcessing(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				// Verify intent processing response structure
				assert.Contains(t, response, "raw_text")
				assert.Contains(t, response, "type")
				assert.Contains(t, response, "confidence")
			}
		})
	}
}

// Test server start and graceful shutdown
func TestServerStartStop(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	cfg.Server.Port = 0 // Use any available port for testing
	server := NewWithOptions(cfg, false)

	// Test that Start method can be called without error
	startErrChan := make(chan error, 1)
	go func() {
		startErrChan <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that Stop method works
	stopErr := server.Stop()
	assert.NoError(t, stopErr)

	// Verify Start returns after Stop
	select {
	case startErr := <-startErrChan:
		// Server should shut down gracefully
		assert.NoError(t, startErr)
	case <-time.After(5 * time.Second):
		t.Fatal("Server did not shut down within timeout")
	}
}

// Test route registration
func TestRoutes(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Test that routes are properly registered by making test requests
	endpoints := []struct {
		path   string
		method string
	}{
		{"/health", "GET"},
		{"/api/capabilities", "GET"},
		{"/api/arbitration/stats", "GET"},
		{"/api/tier", "GET"},
		{"/api/intent/process", "POST"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			var body string
			if endpoint.path == "/api/intent/process" {
				body = `{"text": "test"}`
			}

			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(body))
			if endpoint.path == "/api/intent/process" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			server.mux.ServeHTTP(w, req)

			// Should not get 404 (routes should be registered)
			assert.NotEqual(t, http.StatusNotFound, w.Code, "Route %s should be registered", endpoint.path)
		})
	}
}

// Test server configuration validation
func TestServerConfiguration(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Verify server configuration is set correctly
	assert.Equal(t, cfg, server.cfg)
	assert.Equal(t, ":3000", server.server.Addr)
	assert.Equal(t, 15*time.Second, server.server.ReadTimeout)
	assert.Equal(t, 15*time.Second, server.server.WriteTimeout)
	assert.Equal(t, 60*time.Second, server.server.IdleTimeout)
}

// Test configureComponents method
func TestConfigureComponents(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Verify all components are properly configured
	assert.NotNil(t, server.streamTransport)
	assert.NotNil(t, server.arbitrator)
	assert.NotNil(t, server.intentProcessor)
	assert.NotNil(t, server.tierDetector)
	assert.NotNil(t, server.ctx)
	assert.NotNil(t, server.cancel)
}

// Test writeJSON helper function
func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name        string
		data        interface{}
		expectError bool
	}{
		{
			name:        "Valid object",
			data:        map[string]string{"key": "value"},
			expectError: false,
		},
		{
			name:        "Valid slice",
			data:        []string{"item1", "item2"},
			expectError: false,
		},
		{
			name:        "Nil data",
			data:        nil,
			expectError: false,
		},
		{
			name:        "Invalid data (channel)",
			data:        make(chan int),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			w.Header().Set("Content-Type", "application/json") // Set before calling writeJSON
			err := writeJSON(w, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Verify JSON was written (response body should not be empty for valid data)
				if tt.data != nil {
					assert.Greater(t, w.Body.Len(), 0)
				}
			}
		})
	}
}

// Test readJSON helper function
func TestReadJSON(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		target      interface{}
		expectError bool
	}{
		{
			name:        "Valid JSON object",
			body:        `{"text": "hello"}`,
			target:      &map[string]string{},
			expectError: false,
		},
		{
			name:        "Invalid JSON",
			body:        `{invalid}`,
			target:      &map[string]string{},
			expectError: true,
		},
		{
			name:        "Empty body",
			body:        "",
			target:      &map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(tt.body))
			err := readJSON(req, tt.target)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Frame Handler Tests

func TestHandleWakeWordFrame(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	tests := []struct {
		name           string
		frameData      []byte
		expectError    bool
		expectedError  string
	}{
		{
			name: "Valid wake word detection",
			frameData: func() []byte {
				detection := arbitration.WakeWordDetection{
					SessionID:   "test-session",
					PuckID:      "test-puck",
					Confidence:  0.95,
					Timestamp:   time.Now(),
					AudioLevel:  0.8,
					NoiseLevel:  0.2,
				}
				data, _ := arbitration.SerializeWakeWordDetection(detection)
				return data
			}(),
			expectError: false,
		},
		{
			name:          "Invalid frame data",
			frameData:     []byte("invalid data"),
			expectError:   true,
			expectedError: "failed to deserialize wake word detection",
		},
		{
			name:          "Empty frame data",
			frameData:     []byte{},
			expectError:   true,
			expectedError: "failed to deserialize wake word detection",
		},
		{
			name: "Timestamp overflow handling",
			frameData: func() []byte {
				detection := arbitration.WakeWordDetection{
					SessionID:   "test-session",
					PuckID:      "test-puck",
					Confidence:  0.95,
					Timestamp:   time.Now(),
					AudioLevel:  0.8,
					NoiseLevel:  0.2,
				}
				data, _ := arbitration.SerializeWakeWordDetection(detection)
				return data
			}(),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createTestConfig()
			server := NewWithOptions(cfg, false)

			// Create test session
			session := &transport.StreamSession{
				ID:     "test-session-123",
				PuckID: "test-puck-456",
			}

			// Create test frame with large timestamp to test overflow handling
			timestamp := uint64(9999999999999999) // This should trigger overflow protection
			if tt.name == "Valid wake word detection" {
				timestamp = uint64(1234567890) // Normal timestamp
			}

			frame := &transport.Frame{
				Type:      transport.FrameTypeWakeWord,
				Data:      tt.frameData,
				Timestamp: timestamp,
			}

			err := server.handleWakeWordFrame(session, frame)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleAudioFrame(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Create test session
	session := &transport.StreamSession{
		ID:     "test-session-123",
		PuckID: "test-puck-456",
	}

	tests := []struct {
		name      string
		audioData []byte
	}{
		{
			name:      "Valid audio data",
			audioData: make([]byte, 1024), // 1KB of audio data
		},
		{
			name:      "Empty audio data",
			audioData: []byte{},
		},
		{
			name:      "Large audio frame",
			audioData: make([]byte, 4096), // 4KB audio frame
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := &transport.Frame{
				Type:      transport.FrameTypeAudioData,
				Data:      tt.audioData,
				Timestamp: timeToMicros(time.Now()),
			}

			// Audio frame handler should not return error for any valid frame
			err := server.handleAudioFrame(session, frame)
			assert.NoError(t, err)
		})
	}
}

func TestHandleHeartbeatFrame(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Create test session
	session := &transport.StreamSession{
		ID:     "test-session-123",
		PuckID: "test-puck-456",
	}

	frame := &transport.Frame{
		Type:      transport.FrameTypeHeartbeat,
		Data:      []byte{},
		Timestamp: uint64(time.Now().UnixNano() / 1000),
	}

	// Heartbeat frame handler should always succeed
	err := server.handleHeartbeatFrame(session, frame)
	assert.NoError(t, err)
}

func TestHandleHandshakeFrame(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Create test session
	session := &transport.StreamSession{
		ID:     "test-session-123",
		PuckID: "test-puck-456",
	}

	tests := []struct {
		name          string
		handshakeData string
		expectError   bool
		expectedError string
	}{
		{
			name:          "Valid handshake data",
			handshakeData: "session:test-session-123;puck:test-puck-456",
			expectError:   false,
		},
		{
			name:          "Valid handshake with extra fields",
			handshakeData: "session:test-session-123;puck:test-puck-456;version:1.0",
			expectError:   false,
		},
		{
			name:          "Session ID mismatch",
			handshakeData: "session:different-session;puck:test-puck-456",
			expectError:   true,
			expectedError: "session ID mismatch in handshake",
		},
		{
			name:          "Puck ID mismatch",
			handshakeData: "session:test-session-123;puck:different-puck",
			expectError:   true,
			expectedError: "puck ID mismatch in handshake",
		},
		{
			name:          "Empty handshake data",
			handshakeData: "",
			expectError:   true,
			expectedError: "empty handshake data",
		},
		{
			name:          "Malformed handshake data",
			handshakeData: "invalid;format;data",
			expectError:   false, // Should not error for missing IDs, just log warning
		},
		{
			name:          "Partial handshake data (session only)",
			handshakeData: "session:test-session-123",
			expectError:   false,
		},
		{
			name:          "Partial handshake data (puck only)",
			handshakeData: "puck:test-puck-456",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := &transport.Frame{
				Type:      transport.FrameTypeHandshake,
				Data:      []byte(tt.handshakeData),
				Timestamp: timeToMicros(time.Now()),
			}

			err := server.handleHandshakeFrame(session, frame)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleArbitrationResult(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	// Create test arbitration result
	result := &arbitration.ArbitrationResult{
		WinnerPuckID:     "puck-001",
		WinnerScore:      0.95,
		AllDetections:    []arbitration.WakeWordDetection{},
		ArbitrationTime:  50 * time.Millisecond,
	}

	// Handler should not panic or error
	server.handleArbitrationResult(result)

	// Test with multiple detections
	result.AllDetections = []arbitration.WakeWordDetection{
		{PuckID: "puck-001", Confidence: 0.95},
		{PuckID: "puck-002", Confidence: 0.75},
	}

	server.handleArbitrationResult(result)
}

func TestHandleTierChange(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	tests := []struct {
		name    string
		oldTier tiers.PerformanceTier
		newTier tiers.PerformanceTier
	}{
		{
			name:    "Upgrade to pro",
			oldTier: tiers.TierBasic,
			newTier: tiers.TierPro,
		},
		{
			name:    "Downgrade to basic",
			oldTier: tiers.TierPro,
			newTier: tiers.TierBasic,
		},
		{
			name:    "Same tier",
			oldTier: tiers.TierStandard,
			newTier: tiers.TierStandard,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Handler should not panic or error
			server.handleTierChange(tt.oldTier, tt.newTier)
		})
	}
}

func TestHandleDegradation(t *testing.T) {
	if err := logging.Initialize(); err != nil {
		t.Fatalf("Failed to initialize logging: %v", err)
	}
	defer logging.Close()

	cfg := createTestConfig()
	server := NewWithOptions(cfg, false)

	reasons := []string{
		"STT service unavailable",
		"High latency detected",
		"Memory usage critical",
		"Network connectivity issues",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			// Handler should not panic or error
			server.handleDegradation(reason)
		})
	}
}
