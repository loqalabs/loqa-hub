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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/security"
)

var (
	ErrSkillNotFound      = errors.New("skill not found")
	ErrSkillAlreadyLoaded = errors.New("skill already loaded")
	ErrInvalidManifest    = errors.New("invalid skill manifest")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrSkillInitFailed    = errors.New("skill initialization failed")
	ErrNoSkillCanHandle   = errors.New("no skill can handle this intent")
	ErrInvalidSkillID     = security.ErrInvalidSkillID
)

// safeConfigPath constructs a safe config file path within the config store
func (sm *SkillManager) safeConfigPath(skillID string) (string, error) {
	// Validate skill ID using shared security function
	if err := security.ValidateSkillID(skillID); err != nil {
		return "", fmt.Errorf("invalid skill ID: %w", err)
	}

	// Get absolute path of safe directory first
	safeDir, err := filepath.Abs(sm.config.ConfigStore)
	if err != nil {
		return "", fmt.Errorf("failed to resolve config store path: %w", err)
	}

	// Construct path by joining safe directory with user input
	configPath := filepath.Join(safeDir, skillID+".json")

	// Get absolute path of the result (CodeQL recommended pattern)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve config path: %w", err)
	}

	// Ensure the resolved path is within the safe directory (CodeQL recommended check)
	if !strings.HasPrefix(absConfigPath, safeDir+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file path: path traversal detected")
	}

	return absConfigPath, nil
}

// SkillManagerConfig holds configuration for the skill manager
type SkillManagerConfig struct {
	SkillsDir    string        `json:"skills_dir"`
	AutoLoad     bool          `json:"auto_load"`
	MaxSkills    int           `json:"max_skills"`
	LoadTimeout  time.Duration `json:"load_timeout"`
	DefaultTrust TrustLevel    `json:"default_trust"`
	AllowedModes []SandboxMode `json:"allowed_sandbox_modes"`
	ConfigStore  string        `json:"config_store"`
}

// SkillManager manages the lifecycle of skills
type SkillManager struct {
	config SkillManagerConfig
	skills map[string]*LoadedSkill
	mutex  sync.RWMutex
	loader SkillLoader
}

// LoadedSkill wraps a SkillPlugin with additional runtime information
type LoadedSkill struct {
	Plugin SkillPlugin
	Info   *SkillInfo
	mutex  sync.RWMutex
}

// SkillLoader defines the interface for loading skills
type SkillLoader interface {
	LoadSkill(ctx context.Context, skillPath string) (SkillPlugin, error)
	UnloadSkill(ctx context.Context, plugin SkillPlugin) error
	SupportedModes() []SandboxMode
}

// NewSkillManager creates a new skill manager
func NewSkillManager(config SkillManagerConfig, loader SkillLoader) *SkillManager {
	if config.MaxSkills <= 0 {
		config.MaxSkills = 50
	}
	if config.LoadTimeout <= 0 {
		config.LoadTimeout = 30 * time.Second
	}

	return &SkillManager{
		config: config,
		skills: make(map[string]*LoadedSkill),
		loader: loader,
	}
}

// Start initializes the skill manager and loads skills
func (sm *SkillManager) Start(ctx context.Context) error {
	logging.Sugar.Infow("Starting skill manager", "skills_dir", security.SanitizeLogInput(sm.config.SkillsDir))

	if sm.config.AutoLoad {
		if err := sm.loadAllSkills(ctx); err != nil {
			logging.Sugar.Warnw("Failed to load some skills during startup", "error", err)
		}
	}

	return nil
}

// Stop gracefully shuts down the skill manager
func (sm *SkillManager) Stop(ctx context.Context) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	logging.Sugar.Infow("Stopping skill manager", "loaded_skills", len(sm.skills))

	var errors []error
	for skillID := range sm.skills {
		if err := sm.unloadSkillUnsafe(ctx, skillID); err != nil {
			errors = append(errors, fmt.Errorf("failed to unload skill %s: %w", skillID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// LoadSkill loads a skill from the specified path
func (sm *SkillManager) LoadSkill(ctx context.Context, skillPath string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Validate skill path to prevent path traversal attacks
	if skillPath == "" {
		return fmt.Errorf("skill path cannot be empty")
	}

	// Additional path validation - ensure no path traversal attempts
	if strings.Contains(skillPath, "..") || strings.Contains(skillPath, "\\") {
		return fmt.Errorf("invalid skill path: path traversal detected")
	}

	// Load the manifest first to get skill ID
	manifest, err := sm.loadManifest(skillPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Check if already loaded
	if _, exists := sm.skills[manifest.ID]; exists {
		return ErrSkillAlreadyLoaded
	}

	// Check skill limits
	if len(sm.skills) >= sm.config.MaxSkills {
		return fmt.Errorf("maximum number of skills reached: %d", sm.config.MaxSkills)
	}

	// Validate permissions and trust level
	if err := sm.validateSkill(manifest); err != nil {
		return fmt.Errorf("skill validation failed: %w", err)
	}

	// Load the plugin
	ctx, cancel := context.WithTimeout(ctx, sm.config.LoadTimeout)
	defer cancel()

	plugin, err := sm.loader.LoadSkill(ctx, skillPath)
	if err != nil {
		return fmt.Errorf("failed to load skill plugin: %w", err)
	}

	// Load or create skill configuration
	config, err := sm.loadSkillConfig(manifest.ID)
	if err != nil {
		logging.Sugar.Warnw("Failed to load skill config, using defaults", "skill", security.SanitizeLogInput(manifest.ID), "error", err)
		config = &SkillConfig{
			SkillID:     manifest.ID,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Config:      make(map[string]interface{}),
			Permissions: manifest.Permissions,
			Enabled:     true,
			Timeout:     30 * time.Second,
			MaxRetries:  3,
		}
	}

	// Initialize the skill
	if err := plugin.Initialize(ctx, config); err != nil {
		if unloadErr := sm.loader.UnloadSkill(ctx, plugin); unloadErr != nil {
			logging.Sugar.Warnw("Failed to unload skill after initialization failure", "error", unloadErr)
		}
		return fmt.Errorf("skill initialization failed: %w", err)
	}

	// Create loaded skill info
	skillInfo := &SkillInfo{
		Manifest:   manifest,
		Config:     config,
		Status:     SkillStatus{State: SkillStateReady, Healthy: true},
		LoadedAt:   time.Now(),
		PluginPath: skillPath,
	}

	loadedSkill := &LoadedSkill{
		Plugin: plugin,
		Info:   skillInfo,
	}

	sm.skills[manifest.ID] = loadedSkill

	logging.Sugar.Infow("Skill loaded successfully",
		"skill_id", security.SanitizeLogInput(manifest.ID),
		"name", security.SanitizeLogInput(manifest.Name),
		"version", security.SanitizeLogInput(manifest.Version))

	return nil
}

// UnloadSkill unloads a skill
func (sm *SkillManager) UnloadSkill(ctx context.Context, skillID string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	return sm.unloadSkillUnsafe(ctx, skillID)
}

func (sm *SkillManager) unloadSkillUnsafe(ctx context.Context, skillID string) error {
	loadedSkill, exists := sm.skills[skillID]
	if !exists {
		return ErrSkillNotFound
	}

	// Update status
	loadedSkill.mutex.Lock()
	loadedSkill.Info.Status.State = SkillStateShutdown
	loadedSkill.mutex.Unlock()

	// Teardown the skill
	if err := loadedSkill.Plugin.Teardown(ctx); err != nil {
		logging.Sugar.Warnw("Skill teardown failed", "skill", security.SanitizeLogInput(skillID), "error", err)
	}

	// Unload from loader
	if err := sm.loader.UnloadSkill(ctx, loadedSkill.Plugin); err != nil {
		logging.Sugar.Warnw("Failed to unload skill from loader", "skill", security.SanitizeLogInput(skillID), "error", err)
		// Continue with unloading even if loader fails
	}

	delete(sm.skills, skillID)

	logging.Sugar.Infow("Skill unloaded", "skill_id", security.SanitizeLogInput(skillID))
	return nil
}

// HandleIntent routes an intent to the appropriate skill
func (sm *SkillManager) HandleIntent(ctx context.Context, intent *VoiceIntent) (*SkillResponse, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	// Find skills that can handle this intent
	var candidates []*LoadedSkill
	for _, loadedSkill := range sm.skills {
		loadedSkill.mutex.RLock()
		if loadedSkill.Info.Status.State == SkillStateReady &&
			loadedSkill.Info.Status.Healthy &&
			loadedSkill.Info.Config.Enabled &&
			loadedSkill.Plugin.CanHandle(*intent) {
			candidates = append(candidates, loadedSkill)
		}
		loadedSkill.mutex.RUnlock()
	}

	if len(candidates) == 0 {
		return nil, ErrNoSkillCanHandle
	}

	// Sort by priority (implement priority logic based on manifest)
	sort.Slice(candidates, func(i, j int) bool {
		// For now, use first match
		return true
	})

	// Try each candidate until one succeeds
	var lastError error
	for _, candidate := range candidates {
		response, err := sm.executeSkill(ctx, candidate, intent)
		if err != nil {
			lastError = err
			candidate.mutex.Lock()
			candidate.Info.ErrorCount++
			candidate.Info.LastError = err.Error()
			candidate.mutex.Unlock()

			logging.Sugar.Warnw("Skill execution failed",
				"skill", security.SanitizeLogInput(candidate.Info.Manifest.ID),
				"error", err)
			continue
		}

		// Update usage statistics
		candidate.mutex.Lock()
		now := time.Now()
		candidate.Info.LastUsed = &now
		candidate.Info.Status.LastUsed = now
		candidate.Info.Status.UsageCount++
		candidate.mutex.Unlock()

		return response, nil
	}

	return nil, fmt.Errorf("all candidate skills failed, last error: %w", lastError)
}

func (sm *SkillManager) executeSkill(ctx context.Context, loadedSkill *LoadedSkill, intent *VoiceIntent) (*SkillResponse, error) {
	// Set a reasonable timeout for skill execution
	ctx, cancel := context.WithTimeout(ctx, loadedSkill.Info.Config.Timeout)
	defer cancel()

	return loadedSkill.Plugin.HandleIntent(ctx, intent)
}

// GetSkill returns information about a loaded skill
func (sm *SkillManager) GetSkill(skillID string) (*SkillInfo, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	loadedSkill, exists := sm.skills[skillID]
	if !exists {
		return nil, ErrSkillNotFound
	}

	loadedSkill.mutex.RLock()
	defer loadedSkill.mutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	info := *loadedSkill.Info
	return &info, nil
}

// ListSkills returns information about all loaded skills
func (sm *SkillManager) ListSkills() []*SkillInfo {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	skills := make([]*SkillInfo, 0, len(sm.skills))
	for _, loadedSkill := range sm.skills {
		loadedSkill.mutex.RLock()
		info := *loadedSkill.Info
		loadedSkill.mutex.RUnlock()
		skills = append(skills, &info)
	}

	// Sort by name for consistent ordering
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Manifest.Name < skills[j].Manifest.Name
	})

	return skills
}

// EnableSkill enables a skill
func (sm *SkillManager) EnableSkill(ctx context.Context, skillID string) error {
	return sm.updateSkillEnabled(ctx, skillID, true)
}

// DisableSkill disables a skill
func (sm *SkillManager) DisableSkill(ctx context.Context, skillID string) error {
	return sm.updateSkillEnabled(ctx, skillID, false)
}

func (sm *SkillManager) updateSkillEnabled(ctx context.Context, skillID string, enabled bool) error {
	sm.mutex.RLock()
	loadedSkill, exists := sm.skills[skillID]
	sm.mutex.RUnlock()

	if !exists {
		return ErrSkillNotFound
	}

	loadedSkill.mutex.Lock()
	defer loadedSkill.mutex.Unlock()

	loadedSkill.Info.Config.Enabled = enabled
	if enabled {
		loadedSkill.Info.Status.State = SkillStateReady
	} else {
		loadedSkill.Info.Status.State = SkillStateDisabled
	}

	// Update the plugin configuration
	if err := loadedSkill.Plugin.UpdateConfig(ctx, loadedSkill.Info.Config); err != nil {
		return fmt.Errorf("failed to update skill config: %w", err)
	}

	// Save configuration
	if err := sm.saveSkillConfig(skillID, loadedSkill.Info.Config); err != nil {
		logging.Sugar.Warnw("Failed to save skill config", "skill", security.SanitizeLogInput(skillID), "error", err)
	}

	return nil
}

// loadAllSkills loads all skills from the skills directory
func (sm *SkillManager) loadAllSkills(ctx context.Context) error {
	if sm.config.SkillsDir == "" {
		return nil
	}

	return filepath.WalkDir(sm.config.SkillsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != sm.config.SkillsDir {
			// Check if this directory contains a manifest
			manifestPath := filepath.Join(path, "skill.json")
			if _, err := os.Stat(manifestPath); err == nil {
				if loadErr := sm.LoadSkill(ctx, path); loadErr != nil {
					logging.Sugar.Warnw("Failed to load skill", "path", security.SanitizeLogInput(path), "error", loadErr)
				}
			}
			return filepath.SkipDir // Don't recurse into subdirectories
		}

		return nil
	})
}

// loadManifest loads and validates a skill manifest
func (sm *SkillManager) loadManifest(skillPath string) (*SkillManifest, error) {
	// Validate the skill path to prevent directory traversal
	if skillPath == "" {
		return nil, fmt.Errorf("empty skill path")
	}

	// Check for path traversal attempts
	if strings.Contains(skillPath, "..") {
		return nil, fmt.Errorf("invalid skill path: path traversal detected")
	}

	manifestPath := filepath.Clean(filepath.Join(skillPath, "skill.json"))
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Basic validation
	if manifest.ID == "" || manifest.Name == "" || manifest.Version == "" {
		return nil, ErrInvalidManifest
	}

	// Validate the skill ID from the manifest
	if err := security.ValidateSkillID(manifest.ID); err != nil {
		return nil, fmt.Errorf("invalid skill ID in manifest: %w", err)
	}

	return &manifest, nil
}

// validateSkill validates a skill's manifest and permissions
func (sm *SkillManager) validateSkill(manifest *SkillManifest) error {
	// Check sandbox mode support
	supported := false
	for _, mode := range sm.config.AllowedModes {
		if mode == manifest.SandboxMode {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("sandbox mode %s not supported", manifest.SandboxMode)
	}

	// Check trust level (implement additional checks as needed)
	if manifest.TrustLevel == "" {
		manifest.TrustLevel = sm.config.DefaultTrust
	}

	return nil
}

// loadSkillConfig loads configuration for a skill
func (sm *SkillManager) loadSkillConfig(skillID string) (*SkillConfig, error) {
	// Get safe config path
	configPath, err := sm.safeConfigPath(skillID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filepath.Clean(configPath))
	if err != nil {
		return nil, err
	}

	var config SkillConfig
	err = json.Unmarshal(data, &config)
	return &config, err
}

// saveSkillConfig saves configuration for a skill
func (sm *SkillManager) saveSkillConfig(skillID string, config *SkillConfig) error {
	if sm.config.ConfigStore == "" {
		return nil
	}

	// Ensure config directory exists
	if err := os.MkdirAll(sm.config.ConfigStore, 0750); err != nil {
		return err
	}

	// Get safe config path
	configPath, err := sm.safeConfigPath(skillID)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}
