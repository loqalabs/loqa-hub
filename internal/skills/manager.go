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

package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Manager handles the loading, lifecycle, and execution of skills
type Manager struct {
	logger     *zap.Logger
	skillsPath string
	
	// Skill management
	skills     map[string]SkillPlugin
	configs    map[string]*SkillConfig
	executors  map[string]SkillExecutor
	
	// Synchronization
	mu         sync.RWMutex
	
	// Context for lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager creates a new skill manager
func NewManager(logger *zap.Logger, skillsPath string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Manager{
		logger:     logger,
		skillsPath: skillsPath,
		skills:     make(map[string]SkillPlugin),
		configs:    make(map[string]*SkillConfig),
		executors:  make(map[string]SkillExecutor),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// RegisterExecutor registers a skill executor for a specific skill type
func (m *Manager) RegisterExecutor(skillType string, executor SkillExecutor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.executors[skillType] = executor
	m.logger.Info("Registered skill executor", zap.String("type", skillType))
}

// LoadSkillsFromDirectory scans the skills directory and loads all available skills
func (m *Manager) LoadSkillsFromDirectory() error {
	if m.skillsPath == "" {
		m.logger.Warn("No skills directory configured, skipping skill loading")
		return nil
	}
	
	// Ensure skills directory exists
	if err := os.MkdirAll(m.skillsPath, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}
	
	// Scan for skill manifests
	return filepath.Walk(m.skillsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.Name() == "skill.json" || info.Name() == "manifest.json" {
			if err := m.loadSkillFromManifest(path); err != nil {
				m.logger.Error("Failed to load skill", 
					zap.String("manifest", path), 
					zap.Error(err))
			}
		}
		
		return nil
	})
}

// loadSkillFromManifest loads a skill from its manifest file
func (m *Manager) loadSkillFromManifest(manifestPath string) error {
	// Read manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}
	
	// Parse manifest
	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}
	
	// Load skill configuration
	config, err := m.loadSkillConfig(manifest.ID)
	if err != nil {
		// Create default config if none exists
		config = &SkillConfig{
			SkillID:    manifest.ID,
			Name:       manifest.Name,
			Version:    manifest.Version,
			Config:     make(map[string]interface{}),
			Permissions: manifest.Permissions,
			Enabled:    manifest.LoadOnStartup,
			Timeout:    30 * time.Second,
			MaxRetries: 3,
		}
	}
	
	// Only load if enabled
	if !config.Enabled {
		m.logger.Info("Skill disabled, skipping load", zap.String("skill", manifest.ID))
		return nil
	}
	
	return m.LoadSkill(&manifest, config)
}

// LoadSkill loads a specific skill with the given configuration
func (m *Manager) LoadSkill(manifest *SkillManifest, config *SkillConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	skillID := manifest.ID
	
	// Check if skill is already loaded
	if _, exists := m.skills[skillID]; exists {
		return fmt.Errorf("skill %s is already loaded", skillID)
	}
	
	// Find appropriate executor (for now, assume built-in)
	executor, exists := m.executors["builtin"]
	if !exists {
		return fmt.Errorf("no executor available for skill type: builtin")
	}
	
	// Load the skill
	skill, err := executor.LoadSkill(manifest, config)
	if err != nil {
		return fmt.Errorf("failed to load skill %s: %w", skillID, err)
	}
	
	// Initialize the skill
	initCtx, cancel := context.WithTimeout(m.ctx, config.Timeout)
	defer cancel()
	
	if err := skill.Init(initCtx, config); err != nil {
		return fmt.Errorf("failed to initialize skill %s: %w", skillID, err)
	}
	
	// Store skill and config
	m.skills[skillID] = skill
	m.configs[skillID] = config
	
	m.logger.Info("Successfully loaded skill", 
		zap.String("skill", skillID), 
		zap.String("version", manifest.Version))
	
	return nil
}

// UnloadSkill unloads a specific skill
func (m *Manager) UnloadSkill(skillID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	skill, exists := m.skills[skillID]
	if !exists {
		return fmt.Errorf("skill %s is not loaded", skillID)
	}
	
	// Shutdown the skill
	shutdownCtx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()
	
	if err := skill.Shutdown(shutdownCtx); err != nil {
		m.logger.Warn("Error shutting down skill", 
			zap.String("skill", skillID), 
			zap.Error(err))
	}
	
	// Remove from maps
	delete(m.skills, skillID)
	delete(m.configs, skillID)
	
	m.logger.Info("Unloaded skill", zap.String("skill", skillID))
	return nil
}

// ExecuteIntent finds a matching skill and executes the intent
func (m *Manager) ExecuteIntent(ctx context.Context, intent *VoiceIntent) (*SkillResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Find matching skills
	var matchingSkill SkillPlugin
	var highestConfidence float64
	
	for skillID, skill := range m.skills {
		manifest := skill.GetManifest()
		status := skill.GetStatus()
		
		// Skip unhealthy skills
		if !status.Healthy || status.State != SkillStateReady {
			continue
		}
		
		// Check if skill can handle this intent
		if m.skillMatches(manifest, intent) {
			// For now, use the first match
			// TODO: Implement proper intent routing with confidence scoring
			matchingSkill = skill
			break
		}
	}
	
	if matchingSkill == nil {
		return &SkillResponse{
			Success: false,
			Error:   "No skill found to handle intent",
			Message: "I don't know how to handle that command.",
		}, nil
	}
	
	// Execute the intent with timeout
	config := m.configs[matchingSkill.GetManifest().ID]
	execCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	
	startTime := time.Now()
	response, err := matchingSkill.HandleIntent(execCtx, intent)
	duration := time.Since(startTime)
	
	if response != nil {
		response.ResponseTime = duration
	}
	
	if err != nil {
		return &SkillResponse{
			Success:      false,
			Error:        err.Error(),
			ResponseTime: duration,
		}, err
	}
	
	return response, nil
}

// skillMatches checks if a skill can handle the given intent
func (m *Manager) skillMatches(manifest *SkillManifest, intent *VoiceIntent) bool {
	// Simple pattern matching for now
	// TODO: Implement proper intent classification
	
	for _, pattern := range manifest.IntentPatterns {
		if intent.Intent == pattern {
			return true
		}
	}
	
	return false
}

// ListSkills returns information about all loaded skills
func (m *Manager) ListSkills() map[string]SkillInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	skills := make(map[string]SkillInfo)
	
	for skillID, skill := range m.skills {
		manifest := skill.GetManifest()
		status := skill.GetStatus()
		config := m.configs[skillID]
		
		skills[skillID] = SkillInfo{
			Manifest: manifest,
			Status:   status,
			Config:   config,
		}
	}
	
	return skills
}

// GetSkill returns a specific skill by ID
func (m *Manager) GetSkill(skillID string) (SkillPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	skill, exists := m.skills[skillID]
	return skill, exists
}

// EnableSkill enables a skill
func (m *Manager) EnableSkill(skillID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	config, exists := m.configs[skillID]
	if !exists {
		return fmt.Errorf("skill %s not found", skillID)
	}
	
	config.Enabled = true
	return m.saveSkillConfig(skillID, config)
}

// DisableSkill disables a skill
func (m *Manager) DisableSkill(skillID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	config, exists := m.configs[skillID]
	if !exists {
		return fmt.Errorf("skill %s not found", skillID)
	}
	
	config.Enabled = false
	if err := m.saveSkillConfig(skillID, config); err != nil {
		return err
	}
	
	// Unload if currently loaded
	if _, loaded := m.skills[skillID]; loaded {
		return m.UnloadSkill(skillID)
	}
	
	return nil
}

// Shutdown gracefully shuts down all loaded skills
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	var errors []error
	
	// Shutdown all skills
	for skillID := range m.skills {
		if err := m.UnloadSkill(skillID); err != nil {
			errors = append(errors, err)
		}
	}
	
	// Cancel context
	m.cancel()
	
	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}
	
	return nil
}

// loadSkillConfig loads configuration for a skill
func (m *Manager) loadSkillConfig(skillID string) (*SkillConfig, error) {
	configPath := filepath.Join(m.skillsPath, skillID, "config.json")
	
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found")
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	
	var config SkillConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	
	return &config, nil
}

// saveSkillConfig saves configuration for a skill
func (m *Manager) saveSkillConfig(skillID string, config *SkillConfig) error {
	skillDir := filepath.Join(m.skillsPath, skillID)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}
	
	configPath := filepath.Join(skillDir, "config.json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	
	return nil
}

// SkillInfo contains information about a loaded skill
type SkillInfo struct {
	Manifest *SkillManifest `json:"manifest"`
	Status   SkillStatus    `json:"status"`
	Config   *SkillConfig   `json:"config"`
}