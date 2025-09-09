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
	"time"
)

// SkillPlugin defines the interface that all Loqa skills must implement
type SkillPlugin interface {
	// Lifecycle hooks
	Initialize(ctx context.Context, config *SkillConfig) error
	Teardown(ctx context.Context) error

	// Core functionality
	CanHandle(intent VoiceIntent) bool
	HandleIntent(ctx context.Context, intent *VoiceIntent) (*SkillResponse, error)

	// Metadata
	GetManifest() (*SkillManifest, error)
	GetStatus() SkillStatus

	// Configuration
	GetConfig() (*SkillConfig, error)
	UpdateConfig(ctx context.Context, config *SkillConfig) error

	// Health checking
	HealthCheck(ctx context.Context) error
}

// VoiceIntent represents a parsed voice command intent
type VoiceIntent struct {
	// Core intent data
	ID         string                 `json:"id"`
	Transcript string                 `json:"transcript"`
	Intent     string                 `json:"intent"`
	Confidence float64                `json:"confidence"`
	Entities   map[string]interface{} `json:"entities"`

	// Metadata
	UserID    string    `json:"user_id,omitempty"`
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"timestamp"`

	// Context
	SessionID string                 `json:"session_id,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// SkillResponse represents the response from a skill
type SkillResponse struct {
	// Response data
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	SpeechText string `json:"speech_text,omitempty"`
	AudioURL   string `json:"audio_url,omitempty"`

	// Actions performed
	Actions []SkillAction `json:"actions,omitempty"`

	// Response metadata
	ResponseTime time.Duration          `json:"response_time"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`

	// Error information
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

// SkillAction represents an action taken by a skill
type SkillAction struct {
	Type       string                 `json:"type"`
	Target     string                 `json:"target"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Success    bool                   `json:"success"`
	Error      string                 `json:"error,omitempty"`
}

// SkillConfig holds configuration for a skill instance
type SkillConfig struct {
	// Skill identification
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Version string `json:"version"`

	// Configuration
	Config map[string]interface{} `json:"config"`

	// Permissions and security
	Permissions []Permission `json:"permissions"`
	Enabled     bool         `json:"enabled"`

	// Runtime settings
	Timeout    time.Duration `json:"timeout"`
	MaxRetries int           `json:"max_retries"`
}

// SkillManifest describes a skill's metadata and capabilities
type SkillManifest struct {
	// Basic information
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	License     string `json:"license"`

	// Capabilities
	IntentPatterns []IntentPattern `json:"intent_patterns"`
	Languages      []string        `json:"languages"`
	Categories     []string        `json:"categories"`

	// Requirements
	Permissions  []Permission  `json:"permissions"`
	Dependencies []string      `json:"dependencies,omitempty"`
	MinVersion   string        `json:"min_loqa_version"`
	ConfigSchema *ConfigSchema `json:"config_schema,omitempty"`

	// Runtime behavior
	LoadOnStartup bool        `json:"load_on_startup"`
	Singleton     bool        `json:"singleton"`
	Timeout       string      `json:"timeout"`
	SandboxMode   SandboxMode `json:"sandbox_mode"`
	TrustLevel    TrustLevel  `json:"trust_level"`

	// Metadata
	Homepage   string   `json:"homepage,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// Permission defines what a skill is allowed to access
type Permission struct {
	Type        PermissionType `json:"type"`
	Resource    string         `json:"resource,omitempty"`
	Actions     []string       `json:"actions,omitempty"`
	Description string         `json:"description"`
}

// PermissionType defines the types of permissions
type PermissionType string

const (
	PermissionMicrophone    PermissionType = "microphone"
	PermissionSpeaker       PermissionType = "speaker"
	PermissionNetwork       PermissionType = "network"
	PermissionFileSystem    PermissionType = "filesystem"
	PermissionDeviceControl PermissionType = "device_control"
	PermissionUserData      PermissionType = "user_data"
	PermissionSystemInfo    PermissionType = "system_info"
)

// SkillStatus represents the current status of a skill
type SkillStatus struct {
	State      SkillState `json:"state"`
	Healthy    bool       `json:"healthy"`
	LastError  string     `json:"last_error,omitempty"`
	LastUsed   time.Time  `json:"last_used"`
	UsageCount int64      `json:"usage_count"`
}

// SkillState defines the possible states of a skill
type SkillState string

const (
	SkillStateLoading  SkillState = "loading"
	SkillStateReady    SkillState = "ready"
	SkillStateError    SkillState = "error"
	SkillStateDisabled SkillState = "disabled"
	SkillStateShutdown SkillState = "shutdown"
)

// ConfigSchema defines the configuration schema for a skill
type ConfigSchema struct {
	Properties map[string]ConfigProperty `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

// ConfigProperty defines a single configuration property
type ConfigProperty struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Default     interface{} `json:"default,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Format      string      `json:"format,omitempty"`
	Sensitive   bool        `json:"sensitive,omitempty"`
}

// IntentPattern defines patterns this skill can handle
type IntentPattern struct {
	Name       string   `json:"name"`
	Examples   []string `json:"examples"`
	Confidence float64  `json:"min_confidence"`
	Priority   int      `json:"priority"`
	Enabled    bool     `json:"enabled"`
	Categories []string `json:"categories,omitempty"`
	Languages  []string `json:"languages,omitempty"`
}

// SandboxMode defines how the skill should be executed
type SandboxMode string

const (
	SandboxNone    SandboxMode = "none"    // No sandboxing
	SandboxProcess SandboxMode = "process" // Run in separate process
	SandboxWASM    SandboxMode = "wasm"    // Run in WASM runtime
	SandboxDocker  SandboxMode = "docker"  // Run in Docker container
)

// TrustLevel defines the trust level of the skill
type TrustLevel string

const (
	TrustSystem    TrustLevel = "system"    // System/built-in skills
	TrustVerified  TrustLevel = "verified"  // Verified by Loqa Labs
	TrustCommunity TrustLevel = "community" // Community contributions
	TrustUnknown   TrustLevel = "unknown"   // Unverified/local skills
)

// SkillInfo contains information about a loaded skill
type SkillInfo struct {
	Manifest   *SkillManifest `json:"manifest"`
	Config     *SkillConfig   `json:"config"`
	Status     SkillStatus    `json:"status"`
	LoadedAt   time.Time      `json:"loaded_at"`
	LastUsed   *time.Time     `json:"last_used,omitempty"`
	ErrorCount int            `json:"error_count"`
	LastError  string         `json:"last_error,omitempty"`
	PluginPath string         `json:"plugin_path"`
}

// SkillExecutor defines how skills are executed (for different plugin types)
type SkillExecutor interface {
	LoadSkill(manifest *SkillManifest, config *SkillConfig) (SkillPlugin, error)
	UnloadSkill(skillID string) error
	ListLoadedSkills() []string
}
