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
	"plugin"
	"strings"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// Directory containing all skills. Change this path as appropriate for your deployment!
const SkillsRootDir = "/opt/loqa/skills"

// DefaultSkillLoader is the default implementation of SkillLoader
type DefaultSkillLoader struct {
	supportedModes []SandboxMode
}

// NewDefaultSkillLoader creates a new default skill loader
func NewDefaultSkillLoader() *DefaultSkillLoader {
	return &DefaultSkillLoader{
		supportedModes: []SandboxMode{
			SandboxNone,
			SandboxProcess,
		},
	}
}

// SupportedModes returns the sandbox modes supported by this loader
func (l *DefaultSkillLoader) SupportedModes() []SandboxMode {
	return l.supportedModes
}

// LoadSkill loads a skill from the specified path
func (l *DefaultSkillLoader) LoadSkill(ctx context.Context, skillPath string) (SkillPlugin, error) {
	// Resolve absolute manifest path
	manifestPath := filepath.Join(skillPath, "skill.json")
	absManifestPath, err := filepath.Abs(manifestPath)
		return nil, fmt.Errorf("failed to resolve skill path: %w", err)
	}
	absSkillsRoot, err := filepath.Abs(SkillsRootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve skills root path: %w", err)
	}
	if !strings.HasPrefix(absManifestPath, absSkillsRoot + string(os.PathSeparator)) && absManifestPath != absSkillsRoot {
		return nil, fmt.Errorf("skill path escapes root directory")
	}
	manifestData, err := os.ReadFile(absManifestPath)
	if err != nil {
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest SkillManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Determine how to load the skill based on sandbox mode
	switch manifest.SandboxMode {
	case SandboxNone:
		return l.loadGoPlugin(ctx, skillPath, &manifest)
	case SandboxProcess:
		return l.loadProcessPlugin(ctx, skillPath, &manifest)
	case SandboxWASM:
		return nil, fmt.Errorf("wasm sandbox mode not implemented")
	case SandboxDocker:
		return nil, fmt.Errorf("docker sandbox mode not implemented")
	default:
		return nil, fmt.Errorf("unsupported sandbox mode: %s", manifest.SandboxMode)
	}
}

// UnloadSkill unloads a skill plugin
func (l *DefaultSkillLoader) UnloadSkill(ctx context.Context, plugin SkillPlugin) error {
	// For Go plugins and process plugins, there's not much we can do
	// The plugin will be garbage collected when no longer referenced
	logging.Sugar.Debugw("Unloading skill plugin", "type", fmt.Sprintf("%T", plugin))
	return nil
}

// loadGoPlugin loads a Go plugin (.so file)
func (l *DefaultSkillLoader) loadGoPlugin(ctx context.Context, skillPath string, manifest *SkillManifest) (SkillPlugin, error) {
	// Look for a .so file in the skill directory
	pluginPath := filepath.Join(skillPath, "skill.so")
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		// Try alternative names
		pluginPath = filepath.Join(skillPath, manifest.ID+".so")
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin file not found in %s", skillPath)
		}
	}

	// Load the Go plugin
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", pluginPath, err)
	}

	// Look for the NewSkill symbol
	newSkillSym, err := p.Lookup("NewSkill")
	if err != nil {
		return nil, fmt.Errorf("NewSkill symbol not found in plugin: %w", err)
	}

	// Cast to the expected function signature
	newSkillFunc, ok := newSkillSym.(func() SkillPlugin)
	if !ok {
		return nil, fmt.Errorf("NewSkill has incorrect signature")
	}

	// Create the skill instance
	skill := newSkillFunc()
	logging.Sugar.Infow("Loaded Go plugin skill", "skill", manifest.ID, "path", pluginPath)
	
	return skill, nil
}

// loadProcessPlugin loads a skill as a separate process
func (l *DefaultSkillLoader) loadProcessPlugin(ctx context.Context, skillPath string, manifest *SkillManifest) (SkillPlugin, error) {
	// Look for an executable in the skill directory
	var execPath string
	candidates := []string{
		filepath.Join(skillPath, "skill"),
		filepath.Join(skillPath, "skill.exe"),
		filepath.Join(skillPath, manifest.ID),
		filepath.Join(skillPath, manifest.ID+".exe"),
	}

	for _, candidate := range candidates {
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			execPath = candidate
			break
		}
	}

	if execPath == "" {
		return nil, fmt.Errorf("skill executable not found in %s", skillPath)
	}

	// Create a process-based skill wrapper
	processSkill := &ProcessSkill{
		manifest:   manifest,
		execPath:   execPath,
		skillPath:  skillPath,
	}

	logging.Sugar.Infow("Loaded process plugin skill", "skill", manifest.ID, "path", execPath)
	return processSkill, nil
}

// ProcessSkill wraps a skill running as a separate process
type ProcessSkill struct {
	manifest  *SkillManifest
	execPath  string
	skillPath string
	config    *SkillConfig
}

// Initialize initializes the process skill
func (p *ProcessSkill) Initialize(ctx context.Context, config *SkillConfig) error {
	p.config = config
	// For now, just store the config
	// In a full implementation, we would start the process and send it the config
	return nil
}

// Teardown shuts down the process skill
func (p *ProcessSkill) Teardown(ctx context.Context) error {
	// In a full implementation, we would terminate the process
	return nil
}

// CanHandle determines if this skill can handle the given intent
func (p *ProcessSkill) CanHandle(intent VoiceIntent) bool {
	// Simple pattern matching for demonstration
	for _, pattern := range p.manifest.IntentPatterns {
		for _, example := range pattern.Examples {
			if containsWords(intent.Transcript, example) {
				return true
			}
		}
	}
	return false
}

// HandleIntent processes an intent
func (p *ProcessSkill) HandleIntent(ctx context.Context, intent *VoiceIntent) (*SkillResponse, error) {
	// In a full implementation, we would communicate with the process
	// For now, return a simple response
	return &SkillResponse{
		Success:    true,
		Message:    "Hello from " + p.manifest.Name,
		SpeechText: "Hello from " + p.manifest.Name,
	}, nil
}

// GetManifest returns the skill manifest
func (p *ProcessSkill) GetManifest() (*SkillManifest, error) {
	return p.manifest, nil
}

// GetStatus returns the skill status
func (p *ProcessSkill) GetStatus() SkillStatus {
	return SkillStatus{
		State:   SkillStateReady,
		Healthy: true,
	}
}

// GetConfig returns the skill configuration
func (p *ProcessSkill) GetConfig() (*SkillConfig, error) {
	return p.config, nil
}

// UpdateConfig updates the skill configuration
func (p *ProcessSkill) UpdateConfig(ctx context.Context, config *SkillConfig) error {
	p.config = config
	// In a full implementation, we would send the new config to the process
	return nil
}

// HealthCheck verifies the skill is functioning properly
func (p *ProcessSkill) HealthCheck(ctx context.Context) error {
	// In a full implementation, we would check if the process is running
	return nil
}

// Helper function for simple pattern matching
func containsWords(text, pattern string) bool {
	// This is a very basic implementation
	// A real implementation would use proper NLP techniques
	return len(text) > 0 && len(pattern) > 0
}