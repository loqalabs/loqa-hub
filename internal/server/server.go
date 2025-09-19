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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/loqalabs/loqa-hub/internal/api"
	"github.com/loqalabs/loqa-hub/internal/arbitration"
	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/intent"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/storage"
	"github.com/loqalabs/loqa-hub/internal/tiers"
	"github.com/loqalabs/loqa-hub/internal/transport"
)

// Server represents the HTTP/1.1 streaming Loqa hub
type Server struct {
	cfg    *config.Config
	mux    *http.ServeMux
	server *http.Server

	// Core components
	database       *storage.Database
	eventsStore    *storage.VoiceEventsStore
	apiHandler     *api.VoiceEventsHandler

	// New architecture components
	streamTransport   *transport.StreamingTransport
	arbitrator        *arbitration.Arbitrator
	intentProcessor   *intent.CascadeProcessor
	tierDetector      *tiers.TierDetector

	// Server context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new server with the HTTP/1.1 streaming architecture
func New(cfg *config.Config) *Server {
	return NewWithOptions(cfg, true)
}

// NewWithOptions creates a new server with specified options
func NewWithOptions(cfg *config.Config, enableHealthChecks bool) *Server {
	mux := http.NewServeMux()

	// Create server context
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize database
	dbConfig := storage.DatabaseConfig{
		Path: cfg.Server.DBPath,
	}
	database, err := storage.NewDatabase(dbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create voice events store
	eventsStore := storage.NewVoiceEventsStore(database)

	// Create API handler
	apiHandler := api.NewVoiceEventsHandler(eventsStore)

	// Initialize new architecture components
	streamTransport := transport.NewStreamingTransport()
	arbitrator := arbitration.NewArbitrator()
	intentProcessor := intent.NewCascadeProcessor(cfg.Streaming.OllamaURL, cfg.Streaming.Model)
	tierDetector := tiers.NewTierDetector(cfg.STT.URL, cfg.TTS.URL, cfg.Streaming.OllamaURL, cfg.NATS.URL)

	s := &Server{
		cfg:             cfg,
		mux:             mux,
		database:        database,
		eventsStore:     eventsStore,
		apiHandler:      apiHandler,
		streamTransport: streamTransport,
		arbitrator:      arbitrator,
		intentProcessor: intentProcessor,
		tierDetector:    tierDetector,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Set up HTTP server
	s.server = &http.Server{
		Addr:         ":" + strconv.Itoa(s.cfg.Server.Port),
		Handler:      s.mux,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	// Configure components
	s.configureComponents()

	// Set up routes
	s.routes()

	return s
}

// configureComponents sets up integration between components
func (s *Server) configureComponents() {
	// Set up arbitration callbacks
	s.arbitrator.SetArbitrationCompleteCallback(s.handleArbitrationResult)

	// Set up tier detection callbacks
	s.tierDetector.SetTierChangeCallback(s.handleTierChange)
	s.tierDetector.SetDegradationCallback(s.handleDegradation)

	// Register frame handlers for streaming transport
	s.streamTransport.RegisterFrameHandler(transport.FrameTypeWakeWord, s.handleWakeWordFrame)
	s.streamTransport.RegisterFrameHandler(transport.FrameTypeAudioData, s.handleAudioFrame)
	s.streamTransport.RegisterFrameHandler(transport.FrameTypeHeartbeat, s.handleHeartbeatFrame)

	logging.Sugar.Infow("🔧 Components configured",
		"stt_url", s.cfg.STT.URL,
		"tts_url", s.cfg.TTS.URL,
		"llm_url", s.cfg.Streaming.OllamaURL,
		"nats_url", s.cfg.NATS.URL)
}

// Start starts the server and all background services
func (s *Server) Start() error {
	// Start tier detection
	go s.tierDetector.Start(s.ctx)

	logging.Sugar.Infow("🚀 Loqa Hub starting with HTTP/1.1 streaming architecture",
		"http_port", s.cfg.Server.Port,
		"db_path", s.cfg.Server.DBPath,
		"architecture", "HTTP/1.1 Binary Streaming")

	// Start HTTP server
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server failed: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	logging.Sugar.Infow("🛑 Shutting down Loqa Hub")

	// Cancel context to stop background services
	s.cancel()

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	// Close database
	if err := s.database.Close(); err != nil {
		logging.Sugar.Errorw("Failed to close database", "error", err)
	}

	logging.Sugar.Infow("✅ Loqa Hub shut down successfully")
	return nil
}

// routes sets up HTTP routing for the new architecture
func (s *Server) routes() {
	// Health check
	s.mux.HandleFunc("/health", s.handleHealth)

	// Legacy API endpoints
	s.mux.HandleFunc("/api/voice-events", s.apiHandler.HandleVoiceEvents)
	s.mux.HandleFunc("/api/voice-events/", s.apiHandler.HandleVoiceEventByID)

	// New HTTP/1.1 streaming endpoints
	s.mux.HandleFunc("/stream/puck", s.streamTransport.HandleStream)
	s.mux.HandleFunc("/api/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/api/arbitration/stats", s.handleArbitrationStats)
	s.mux.HandleFunc("/api/tier", s.handleTierInfo)

	// Intent processing endpoints
	s.mux.HandleFunc("/api/intent/process", s.handleIntentProcessing)

	logging.Sugar.Infow("🌐 HTTP routes configured",
		"streaming_endpoint", "/stream/puck",
		"capabilities_endpoint", "/api/capabilities",
		"arbitration_endpoint", "/api/arbitration/stats")
}

// handleHealth provides system health information
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	capabilities := s.tierDetector.GetCapabilities()

	health := map[string]interface{}{
		"status":       "ok",
		"timestamp":    time.Now(),
		"architecture": "HTTP/1.1 Binary Streaming",
		"tier":         capabilities.Tier,
		"services":     capabilities.Services,
		"degraded":     capabilities.Degraded,
	}

	if capabilities.Degraded {
		health["degradation_reason"] = capabilities.DegradationReason
	}

	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, health); err != nil {
		logging.Sugar.Errorw("Failed to write health response", "error", err)
	}
}

// handleCapabilities returns current system capabilities
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	capabilities := s.tierDetector.GetCapabilities()

	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, capabilities); err != nil {
		logging.Sugar.Errorw("Failed to write capabilities response", "error", err)
	}
}

// handleArbitrationStats returns arbitration statistics
func (s *Server) handleArbitrationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := s.arbitrator.GetStats()

	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, stats); err != nil {
		logging.Sugar.Errorw("Failed to write arbitration stats", "error", err)
	}
}

// handleTierInfo returns current tier information
func (s *Server) handleTierInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tier := s.tierDetector.GetTier()
	info := map[string]interface{}{
		"current_tier": tier,
		"features": map[string]bool{
			"local_llm":          s.tierDetector.IsFeatureAvailable("local_llm"),
			"streaming_responses": s.tierDetector.IsFeatureAvailable("streaming_responses"),
			"advanced_skills":    s.tierDetector.IsFeatureAvailable("advanced_skills"),
			"reflex_only":        s.tierDetector.IsFeatureAvailable("reflex_only"),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, info); err != nil {
		logging.Sugar.Errorw("Failed to write tier info", "error", err)
	}
}

// handleIntentProcessing processes text through the intent cascade
func (s *Server) handleIntentProcessing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Text string `json:"text"`
	}

	if err := readJSON(r, &request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if request.Text == "" {
		http.Error(w, "Text required", http.StatusBadRequest)
		return
	}

	// Process through intent cascade
	intent, err := s.intentProcessor.ProcessIntent(r.Context(), request.Text)
	if err != nil {
		logging.Sugar.Errorw("Intent processing failed", "text", request.Text, "error", err)
		http.Error(w, "Intent processing failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, intent); err != nil {
		logging.Sugar.Errorw("Failed to write intent response", "error", err)
	}
}

// Frame handlers for streaming transport

// handleWakeWordFrame processes wake word detection frames
func (s *Server) handleWakeWordFrame(session *transport.StreamSession, frame *transport.Frame) error {
	detection, err := arbitration.DeserializeWakeWordDetection(frame.Data)
	if err != nil {
		return fmt.Errorf("failed to deserialize wake word detection: %w", err)
	}

	// Update detection with session info
	detection.SessionID = session.ID
	detection.PuckID = session.PuckID
	detection.Timestamp = time.Unix(0, int64(frame.Timestamp)*1000) // Convert microseconds

	// Send to arbitrator
	return s.arbitrator.ProcessWakeWordDetection(*detection)
}

// handleAudioFrame processes audio data frames
func (s *Server) handleAudioFrame(session *transport.StreamSession, frame *transport.Frame) error {
	// TODO: Implement audio processing pipeline
	// This would integrate with STT service and speech processing

	logging.Sugar.Debugw("Audio frame received",
		"session_id", session.ID,
		"puck_id", session.PuckID,
		"data_size", len(frame.Data))

	return nil
}

// handleHeartbeatFrame processes heartbeat frames
func (s *Server) handleHeartbeatFrame(session *transport.StreamSession, frame *transport.Frame) error {
	// Heartbeat frames are handled automatically by the transport layer
	// This is just for logging/metrics
	logging.Sugar.Debugw("Heartbeat received",
		"session_id", session.ID,
		"puck_id", session.PuckID)

	return nil
}

// Event handlers for system events

// handleArbitrationResult processes completed arbitration
func (s *Server) handleArbitrationResult(result *arbitration.ArbitrationResult) {
	logging.Sugar.Infow("Arbitration result",
		"winner_puck", result.WinnerPuckID,
		"winner_score", result.WinnerScore,
		"total_detections", len(result.AllDetections),
		"decision_time", result.ArbitrationTime)

	// TODO: Trigger voice processing pipeline for winner puck
	// This would start STT processing for the winning puck's audio
}

// handleTierChange responds to performance tier changes
func (s *Server) handleTierChange(oldTier, newTier tiers.PerformanceTier) {
	logging.Sugar.Infow("Performance tier changed",
		"old_tier", oldTier,
		"new_tier", newTier)

	// Update intent processor configuration based on tier
	if newTier == tiers.TierBasic {
		s.intentProcessor.SetCloudEnabled(false)
		logging.Sugar.Infow("Disabled cloud processing for Basic tier")
	}
}

// handleDegradation responds to system degradation
func (s *Server) handleDegradation(reason string) {
	logging.Sugar.Warnw("System degradation detected",
		"reason", reason)

	// Could trigger alerts, fallback modes, etc.
}

// Helper functions

func writeJSON(w http.ResponseWriter, data interface{}) error {
	return json.NewEncoder(w).Encode(data)
}

func readJSON(r *http.Request, data interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.Unmarshal(body, data)
}