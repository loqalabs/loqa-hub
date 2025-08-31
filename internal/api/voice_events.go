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

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"github.com/loqalabs/loqa-hub/internal/events"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/storage"
)

// VoiceEventsHandler handles HTTP requests for voice events
type VoiceEventsHandler struct {
	store *storage.VoiceEventsStore
}

// NewVoiceEventsHandler creates a new voice events handler
func NewVoiceEventsHandler(store *storage.VoiceEventsStore) *VoiceEventsHandler {
	return &VoiceEventsHandler{store: store}
}

// ListVoiceEventsResponse represents the response for listing voice events
type ListVoiceEventsResponse struct {
	Events     []*events.VoiceEvent `json:"events"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	TotalPages int                  `json:"total_pages"`
}

// CreateVoiceEventRequest represents the request for creating a voice event
type CreateVoiceEventRequest struct {
	PuckID           string            `json:"puck_id"`
	RequestID        string            `json:"request_id"`
	Transcription    string            `json:"transcription"`
	Intent           string            `json:"intent"`
	Entities         map[string]string `json:"entities"`
	Confidence       float64           `json:"confidence"`
	ResponseText     string            `json:"response_text"`
	AudioDuration    float64           `json:"audio_duration,omitempty"`
	SampleRate       int               `json:"sample_rate,omitempty"`
	WakeWordDetected bool              `json:"wake_word_detected,omitempty"`
}

// HandleVoiceEvents handles GET /api/voice-events and POST /api/voice-events
func (h *VoiceEventsHandler) HandleVoiceEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listVoiceEvents(w, r)
	case http.MethodPost:
		h.createVoiceEvent(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleVoiceEventByID handles GET /api/voice-events/{id}
func (h *VoiceEventsHandler) HandleVoiceEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract UUID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/voice-events/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Event ID is required", http.StatusBadRequest)
		return
	}

	uuid := pathParts[0]
	h.getVoiceEventByID(w, r, uuid)
}

// listVoiceEvents handles GET /api/voice-events
func (h *VoiceEventsHandler) listVoiceEvents(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	
	// Pagination
	page := parseIntParam(query.Get("page"), 1)
	pageSize := parseIntParam(query.Get("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100 // Limit maximum page size
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if page < 1 {
		page = 1
	}

	// Filtering
	options := storage.ListOptions{
		PuckID:    query.Get("puck_id"),
		Intent:    query.Get("intent"),
		Limit:     pageSize,
		Offset:    (page - 1) * pageSize,
		SortBy:    query.Get("sort_by"),
		SortOrder: strings.ToUpper(query.Get("sort_order")),
	}

	// Parse success filter
	if successStr := query.Get("success"); successStr != "" {
		if success, err := strconv.ParseBool(successStr); err == nil {
			options.Success = &success
		}
	}

	// Parse time filters
	if startTimeStr := query.Get("start_time"); startTimeStr != "" {
		if startTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			options.StartTime = &startTime
		}
	}
	if endTimeStr := query.Get("end_time"); endTimeStr != "" {
		if endTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			options.EndTime = &endTime
		}
	}

	// Get total count for pagination
	total, err := h.store.Count(options)
	if err != nil {
		logging.LogError(err, "Failed to count voice events")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get events
	events, err := h.store.List(options)
	if err != nil {
		logging.LogError(err, "Failed to list voice events")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build response
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	response := ListVoiceEventsResponse{
		Events:     events,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}

	// Log the API request
	logging.Sugar.Infow("Voice events API request",
		"endpoint", "list",
		"page", page,
		"page_size", pageSize,
		"total_results", total,
		"filters", map[string]interface{}{
			"puck_id": options.PuckID,
			"intent":  options.Intent,
			"success": options.Success,
		},
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// createVoiceEvent handles POST /api/voice-events
func (h *VoiceEventsHandler) createVoiceEvent(w http.ResponseWriter, r *http.Request) {
	var req CreateVoiceEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.PuckID == "" {
		http.Error(w, "puck_id is required", http.StatusBadRequest)
		return
	}
	if req.RequestID == "" {
		req.RequestID = req.PuckID // Default to puck ID
	}

	// Create voice event
	voiceEvent := events.NewVoiceEvent(req.PuckID, req.RequestID)
	
	// Set fields from request
	voiceEvent.SetTranscription(req.Transcription)
	voiceEvent.SetCommandResult(req.Intent, req.Entities, req.Confidence)
	voiceEvent.SetResponse(req.ResponseText)

	// Set optional audio metadata
	if req.AudioDuration > 0 || req.SampleRate > 0 {
		// Create dummy audio data for hash calculation if not provided
		dummyAudio := make([]float32, 1)
		voiceEvent.SetAudioMetadata(dummyAudio, req.SampleRate, req.WakeWordDetected)
		// Override duration if provided
		if req.AudioDuration > 0 {
			voiceEvent.AudioDuration = req.AudioDuration
		}
	}

	// Store in database
	if err := h.store.Insert(voiceEvent); err != nil {
		logging.LogError(err, "Failed to create voice event", 
			zap.String("puck_id", req.PuckID),
		)
		http.Error(w, "Failed to create voice event", http.StatusInternalServerError)
		return
	}

	logging.Sugar.Infow("Voice event created via API",
		"event_uuid", voiceEvent.UUID,
		"puck_id", req.PuckID,
		"intent", req.Intent,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(voiceEvent)
}

// getVoiceEventByID handles GET /api/voice-events/{id}
func (h *VoiceEventsHandler) getVoiceEventByID(w http.ResponseWriter, r *http.Request, uuid string) {
	event, err := h.store.GetByUUID(uuid)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Voice event not found", http.StatusNotFound)
			return
		}
		logging.LogError(err, "Failed to get voice event", 
			zap.String("uuid", uuid),
		)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	logging.Sugar.Infow("Voice event retrieved via API",
		"event_uuid", uuid,
		"puck_id", event.PuckID,
		"intent", event.Intent,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

// parseIntParam parses integer parameter with default value
func parseIntParam(param string, defaultValue int) int {
	if param == "" {
		return defaultValue
	}
	
	if value, err := strconv.Atoi(param); err == nil {
		return value
	}
	
	return defaultValue
}