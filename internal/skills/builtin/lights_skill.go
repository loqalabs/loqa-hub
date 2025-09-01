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

package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"github.com/loqalabs/loqa-hub/internal/skills"
)

// LightsSkill handles lighting control commands
type LightsSkill struct {
	logger *zap.Logger
	config *skills.SkillConfig
	status skills.SkillStatus
}

// NewLightsSkill creates a new lights skill instance
func NewLightsSkill(logger *zap.Logger) skills.SkillPlugin {
	return &LightsSkill{
		logger: logger,
		status: skills.SkillStatus{
			State:   skills.SkillStateLoading,
			Healthy: false,
		},
	}
}

// Initialize initializes the lights skill
func (s *LightsSkill) Initialize(ctx context.Context, config *skills.SkillConfig) error {
	s.config = config
	s.status.State = skills.SkillStateReady
	s.status.Healthy = true
	
	s.logger.Info("Initialized lights skill", 
		zap.String("skill", config.SkillID),
		zap.String("version", config.Version))
	
	return nil
}

// Teardown shuts down the lights skill
func (s *LightsSkill) Teardown(ctx context.Context) error {
	s.status.State = skills.SkillStateShutdown
	s.status.Healthy = false
	
	s.logger.Info("Shutdown lights skill")
	return nil
}

// CanHandle determines if this skill can handle the given intent
func (s *LightsSkill) CanHandle(intent skills.VoiceIntent) bool {
	transcript := strings.ToLower(intent.Transcript)
	
	// Check for lighting-related keywords
	lightingKeywords := []string{
		"light", "lights", "lighting",
		"turn on", "turn off", "switch on", "switch off",
		"dim", "brighten", "bright", "dark",
		"lamp", "lamps",
	}
	
	for _, keyword := range lightingKeywords {
		if strings.Contains(transcript, keyword) {
			return true
		}
	}
	
	return false
}

// HandleIntent processes a voice intent for lighting control
func (s *LightsSkill) HandleIntent(ctx context.Context, intent *skills.VoiceIntent) (*skills.SkillResponse, error) {
	s.logger.Info("Handling lights intent", 
		zap.String("intent", intent.Intent),
		zap.String("transcript", intent.Transcript))
	
	// Update usage stats
	s.status.LastUsed = time.Now()
	s.status.UsageCount++
	
	// Parse the intent
	action, location := s.parseIntent(intent)
	
	// Perform the action
	success, message := s.performLightingAction(action, location)
	
	var speechText string
	if success {
		speechText = fmt.Sprintf("%s in the %s", message, location)
	} else {
		speechText = fmt.Sprintf("Sorry, I couldn't %s", message)
	}
	
	return &skills.SkillResponse{
		Success:    success,
		Message:    message,
		SpeechText: speechText,
		Actions: []skills.SkillAction{
			{
				Type:       "lighting_control",
				Target:     fmt.Sprintf("lights.%s", location),
				Parameters: map[string]interface{}{
					"action": action,
				},
				Success: success,
			},
		},
	}, nil
}

// parseIntent extracts action and location from the voice intent
func (s *LightsSkill) parseIntent(intent *skills.VoiceIntent) (string, string) {
	transcript := strings.ToLower(intent.Transcript)
	
	// Extract action
	var action string
	switch {
	case strings.Contains(transcript, "turn on") || strings.Contains(transcript, "switch on"):
		action = "on"
	case strings.Contains(transcript, "turn off") || strings.Contains(transcript, "switch off"):
		action = "off"
	case strings.Contains(transcript, "dim") || strings.Contains(transcript, "lower"):
		action = "dim"
	case strings.Contains(transcript, "brighten") || strings.Contains(transcript, "bright"):
		action = "brighten"
	default:
		action = "toggle"
	}
	
	// Extract location
	var location string
	switch {
	case strings.Contains(transcript, "kitchen"):
		location = "kitchen"
	case strings.Contains(transcript, "living room") || strings.Contains(transcript, "lounge"):
		location = "living_room"
	case strings.Contains(transcript, "bedroom"):
		location = "bedroom"
	case strings.Contains(transcript, "bathroom"):
		location = "bathroom"
	case strings.Contains(transcript, "all") || strings.Contains(transcript, "everywhere"):
		location = "all"
	default:
		location = "main"
	}
	
	return action, location
}

// performLightingAction simulates performing a lighting action
func (s *LightsSkill) performLightingAction(action, location string) (bool, string) {
	// In a real implementation, this would interface with actual smart home systems
	// For now, we'll simulate the action
	
	switch action {
	case "on":
		return true, "Turned on the lights"
	case "off":
		return true, "Turned off the lights"
	case "dim":
		return true, "Dimmed the lights"
	case "brighten":
		return true, "Brightened the lights"
	case "toggle":
		return true, "Toggled the lights"
	default:
		return false, fmt.Sprintf("don't understand the action: %s", action)
	}
}

// GetManifest returns the skill manifest
func (s *LightsSkill) GetManifest() (*skills.SkillManifest, error) {
	return &skills.SkillManifest{
		ID:          "builtin.lights",
		Name:        "Lights Control",
		Version:     "1.0.0",
		Description: "Controls smart lighting systems",
		Author:      "Loqa Labs",
		License:     "AGPL-3.0",
		
		IntentPatterns: []skills.IntentPattern{
			{
				Name:       "lights_on",
				Examples:   []string{"turn on the lights", "switch on lights", "lights on"},
				Confidence: 0.8,
				Priority:   1,
				Enabled:    true,
			},
			{
				Name:       "lights_off",
				Examples:   []string{"turn off the lights", "switch off lights", "lights off"},
				Confidence: 0.8,
				Priority:   1,
				Enabled:    true,
			},
			{
				Name:       "lights_dim",
				Examples:   []string{"dim the lights", "lower the lights", "make it darker"},
				Confidence: 0.8,
				Priority:   1,
				Enabled:    true,
			},
			{
				Name:       "lights_brighten",
				Examples:   []string{"brighten the lights", "make it brighter", "lights up"},
				Confidence: 0.8,
				Priority:   1,
				Enabled:    true,
			},
		},
		
		Languages:   []string{"en"},
		Categories:  []string{"smart_home", "lighting"},
		
		Permissions: []skills.Permission{
			{
				Type:        skills.PermissionDeviceControl,
				Resource:    "lighting",
				Actions:     []string{"on", "off", "dim", "brighten"},
				Description: "Control smart lights",
			},
		},
		
		LoadOnStartup: true,
		Singleton:     true,
		Timeout:       "30s",
		SandboxMode:   skills.SandboxNone,
		TrustLevel:    skills.TrustSystem,
		
		Keywords: []string{"lights", "lighting", "smart home", "automation"},
	}, nil
}

// GetStatus returns the current skill status
func (s *LightsSkill) GetStatus() skills.SkillStatus {
	return s.status
}

// GetConfigSchema returns the configuration schema
func (s *LightsSkill) GetConfigSchema() *skills.ConfigSchema {
	return &skills.ConfigSchema{
		Properties: map[string]skills.ConfigProperty{
			"default_brightness": {
				Type:        "integer",
				Description: "Default brightness level (0-100)",
				Default:     80,
			},
			"fade_duration": {
				Type:        "integer", 
				Description: "Fade duration in milliseconds",
				Default:     500,
			},
			"supported_locations": {
				Type:        "array",
				Description: "List of supported room locations",
				Default:     []string{"kitchen", "living_room", "bedroom", "bathroom"},
			},
		},
		Required: []string{},
	}
}

// GetConfig returns the skill configuration
func (s *LightsSkill) GetConfig() (*skills.SkillConfig, error) {
	return s.config, nil
}

// UpdateConfig updates the skill configuration
func (s *LightsSkill) UpdateConfig(ctx context.Context, config *skills.SkillConfig) error {
	s.config = config
	// Validate and apply new configuration
	s.logger.Info("Updated lights skill configuration", 
		zap.String("skill_id", config.SkillID),
		zap.String("version", config.Version))
	return nil
}

// HealthCheck performs a health check
func (s *LightsSkill) HealthCheck(ctx context.Context) error {
	// Check if the skill is in a healthy state
	if s.status.State != skills.SkillStateReady {
		return fmt.Errorf("skill not ready, current state: %s", s.status.State)
	}
	
	return nil
}