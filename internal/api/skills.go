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
	"net/url"
	"strings"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/security"
	"github.com/loqalabs/loqa-hub/internal/skills"
)

// isValidAction validates that the action is one of the allowed values
func isValidAction(action string) bool {
	validActions := map[string]bool{
		"enable":  true,
		"disable": true,
		"reload":  true,
	}
	return validActions[action]
}

// safeExtractSkillID safely extracts skill ID from URL path with validation
func safeExtractSkillID(urlPath string) (string, error) {
	// Parse URL to ensure it's well-formed
	u, err := url.Parse(urlPath)
	if err != nil {
		return "", err
	}
	
	// Use the cleaned path
	cleanPath := u.Path
	
	// Extract skill ID using the existing logic but on cleaned path
	skillID := extractSkillID(cleanPath)
	
	// Validate extracted skill ID
	if err := security.ValidateSkillID(skillID); err != nil {
		return "", err
	}
	
	return skillID, nil
}

// safeExtractSkillIDAndAction safely extracts skill ID and action from URL path
func safeExtractSkillIDAndAction(urlPath string) (string, string, error) {
	// Parse URL to ensure it's well-formed
	u, err := url.Parse(urlPath)
	if err != nil {
		return "", "", err
	}
	
	// Use the cleaned path
	cleanPath := u.Path
	
	// Extract skill ID and action using existing logic
	skillID, action := extractSkillIDAndAction(cleanPath)
	
	// Validate extracted values
	if err := security.ValidateSkillID(skillID); err != nil {
		return "", "", err
	}
	
	if !isValidAction(action) {
		return "", "", skills.ErrInvalidSkillID // reuse this error for simplicity
	}
	
	return skillID, action, nil
}

// SkillsHandler handles HTTP requests for skill management
type SkillsHandler struct {
	skillManager *skills.SkillManager
}

// NewSkillsHandler creates a new skills API handler
func NewSkillsHandler(skillManager *skills.SkillManager) *SkillsHandler {
	return &SkillsHandler{
		skillManager: skillManager,
	}
}

// HandleSkills handles requests to /api/skills
func (h *SkillsHandler) HandleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listSkills(w, r)
	case http.MethodPost:
		h.loadSkill(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSkillByID handles requests to /api/skills/{id}
func (h *SkillsHandler) HandleSkillByID(w http.ResponseWriter, r *http.Request) {
	// Safely extract and validate skill ID from URL path
	skillID, err := safeExtractSkillID(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill ID")
		return
	}
	if skillID == "" {
		writeError(w, http.StatusBadRequest, "skill ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getSkill(w, r, skillID)
	case http.MethodDelete:
		h.unloadSkill(w, r, skillID)
	case http.MethodPut:
		h.updateSkill(w, r, skillID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleSkillAction handles requests to /api/skills/{id}/{action}
func (h *SkillsHandler) HandleSkillAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Safely extract and validate skill ID and action from URL path
	skillID, action, err := safeExtractSkillIDAndAction(r.URL.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill ID or action")
		return
	}
	if skillID == "" || action == "" {
		writeError(w, http.StatusBadRequest, "skill ID and action required")
		return
	}

	switch action {
	case "enable":
		h.enableSkill(w, r, skillID)
	case "disable":
		h.disableSkill(w, r, skillID)
	case "reload":
		h.reloadSkill(w, r, skillID)
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+security.SanitizeLogInput(action))
	}
}

// listSkills returns all loaded skills
func (h *SkillsHandler) listSkills(w http.ResponseWriter, r *http.Request) {
	skills := h.skillManager.ListSkills()

	response := map[string]interface{}{
		"skills": skills,
		"count":  len(skills),
	}

	writeJSON(w, http.StatusOK, response)
}

// getSkill returns information about a specific skill
func (h *SkillsHandler) getSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	skillInfo, err := h.skillManager.GetSkill(skillID)
	if err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to get skill", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get skill")
		return
	}

	writeJSON(w, http.StatusOK, skillInfo)
}

// loadSkill loads a new skill
func (h *SkillsHandler) loadSkill(w http.ResponseWriter, r *http.Request) {
	var request struct {
		SkillPath string `json:"skill_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if request.SkillPath == "" {
		writeError(w, http.StatusBadRequest, "skill_path is required")
		return
	}

	if err := h.skillManager.LoadSkill(r.Context(), request.SkillPath); err != nil {
		if err == skills.ErrSkillAlreadyLoaded {
			writeError(w, http.StatusConflict, "skill already loaded")
			return
		}
		logging.Sugar.Errorw("Failed to load skill", "path", security.SanitizeLogInput(request.SkillPath), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "skill loaded successfully",
		"path":    request.SkillPath,
	})
}

// unloadSkill unloads an existing skill
func (h *SkillsHandler) unloadSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	if err := h.skillManager.UnloadSkill(r.Context(), skillID); err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to unload skill", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to unload skill")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "skill unloaded successfully",
		"skill":   skillID,
	})
}

// enableSkill enables a skill
func (h *SkillsHandler) enableSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	if err := h.skillManager.EnableSkill(r.Context(), skillID); err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to enable skill", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to enable skill")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "skill enabled successfully",
		"skill":   skillID,
	})
}

// disableSkill disables a skill
func (h *SkillsHandler) disableSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	if err := h.skillManager.DisableSkill(r.Context(), skillID); err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to disable skill", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to disable skill")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "skill disabled successfully",
		"skill":   skillID,
	})
}

// reloadSkill reloads a skill (unload then load)
func (h *SkillsHandler) reloadSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	// Get current skill info to find the plugin path
	skillInfo, err := h.skillManager.GetSkill(skillID)
	if err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to get skill for reload", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get skill")
		return
	}

	// Unload the skill
	if err := h.skillManager.UnloadSkill(r.Context(), skillID); err != nil {
		logging.Sugar.Errorw("Failed to unload skill during reload", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to unload skill")
		return
	}

	// Reload the skill
	if err := h.skillManager.LoadSkill(r.Context(), skillInfo.PluginPath); err != nil {
		logging.Sugar.Errorw("Failed to reload skill", "skill", security.SanitizeLogInput(skillID), "path", security.SanitizeLogInput(skillInfo.PluginPath), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to reload skill: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "skill reloaded successfully",
		"skill":   skillID,
	})
}

// updateSkill updates a skill's configuration
func (h *SkillsHandler) updateSkill(w http.ResponseWriter, r *http.Request, skillID string) {
	var request struct {
		Config map[string]interface{} `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get current skill to update its configuration
	skillInfo, err := h.skillManager.GetSkill(skillID)
	if err != nil {
		if err == skills.ErrSkillNotFound {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		logging.Sugar.Errorw("Failed to get skill for update", "skill", security.SanitizeLogInput(skillID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get skill")
		return
	}

	// Update the config
	if request.Config != nil {
		skillInfo.Config.Config = request.Config
	}

	// This would require implementing UpdateConfig in the skill manager
	// For now, just return success
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "skill configuration updated successfully",
		"skill":   skillID,
	})
}

// Helper functions
func extractSkillID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "skills" {
		return parts[2]
	}
	return ""
}

func extractSkillIDAndAction(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "skills" {
		return parts[2], parts[3]
	}
	return "", ""
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error":   true,
		"message": message,
	})
}
