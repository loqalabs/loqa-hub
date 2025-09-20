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

package intent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/loqalabs/loqa-hub/internal/logging"
)

// Intent represents a parsed voice command intent
type Intent struct {
	Type           IntentType             `json:"type"`
	Confidence     float64                `json:"confidence"`
	Entities       map[string]interface{} `json:"entities"`
	RawText        string                 `json:"raw_text"`
	Source         ProcessorType          `json:"source"`
	ProcessingTime time.Duration          `json:"processing_time"`
}

// IntentType represents different categories of intents
type IntentType string

const (
	IntentUnknown       IntentType = "unknown"
	IntentLightControl  IntentType = "light_control"
	IntentMediaControl  IntentType = "media_control"
	IntentWeatherQuery  IntentType = "weather_query"
	IntentTimeQuery     IntentType = "time_query"
	IntentVolumeControl IntentType = "volume_control"
	IntentSystemControl IntentType = "system_control"
	IntentSmartHome     IntentType = "smart_home"
	IntentConversation  IntentType = "conversation"
)

// ProcessorType identifies which processor handled the intent
type ProcessorType string

const (
	ProcessorReflex ProcessorType = "reflex"
	ProcessorLLM    ProcessorType = "llm"
	ProcessorCloud  ProcessorType = "cloud"
)

// IntentProcessor interface for different processing stages
type IntentProcessor interface {
	ProcessIntent(ctx context.Context, text string) (*Intent, error)
	CanHandle(text string) bool
	GetConfidenceThreshold() float64
}

// CascadeProcessor implements the Reflex → LLM → Cloud cascade
type CascadeProcessor struct {
	reflexProcessor ReflexProcessor
	llmProcessor    LLMProcessor
	cloudProcessor  CloudProcessor

	// Configuration
	reflexThreshold float64
	llmThreshold    float64
	enableCloud     bool
	cascadeTimeout  time.Duration
}

// ReflexProcessor handles immediate pattern-based responses
type ReflexProcessor struct {
	patterns map[IntentType][]*regexp.Regexp
}

// LLMProcessor handles local LLM-based intent parsing
type LLMProcessor struct {
	ollamaURL string
	model     string
	timeout   time.Duration
}

// CloudProcessor handles cloud-based intent parsing (fallback)
type CloudProcessor struct {
	enabled bool
	timeout time.Duration
}

// NewCascadeProcessor creates a new intent cascade processor
func NewCascadeProcessor(ollamaURL, model string) *CascadeProcessor {
	cp := &CascadeProcessor{
		reflexProcessor: *NewReflexProcessor(),
		llmProcessor: LLMProcessor{
			ollamaURL: ollamaURL,
			model:     model,
			timeout:   5 * time.Second,
		},
		cloudProcessor: CloudProcessor{
			enabled: false, // Disabled by default for privacy
			timeout: 10 * time.Second,
		},
		reflexThreshold: 0.8,
		llmThreshold:    0.7,
		enableCloud:     false,
		cascadeTimeout:  15 * time.Second,
	}

	return cp
}

// ProcessIntent executes the full cascade to parse intent
func (cp *CascadeProcessor) ProcessIntent(ctx context.Context, text string) (*Intent, error) {
	startTime := time.Now()

	// Create context with timeout
	cascadeCtx, cancel := context.WithTimeout(ctx, cp.cascadeTimeout)
	defer cancel()

	logging.Sugar.Infow("Starting intent cascade", "text", text)

	// Stage 1: Reflex Processing (immediate patterns)
	if intent, err := cp.reflexProcessor.ProcessIntent(cascadeCtx, text); err == nil && intent.Confidence >= cp.reflexThreshold {
		intent.ProcessingTime = time.Since(startTime)
		logging.Sugar.Infow("Intent resolved by reflex processor",
			"type", intent.Type,
			"confidence", intent.Confidence,
			"processing_time", intent.ProcessingTime)
		return intent, nil
	}

	// Stage 2: LLM Processing (local AI)
	if intent, err := cp.llmProcessor.ProcessIntent(cascadeCtx, text); err == nil && intent.Confidence >= cp.llmThreshold {
		intent.ProcessingTime = time.Since(startTime)
		logging.Sugar.Infow("Intent resolved by LLM processor",
			"type", intent.Type,
			"confidence", intent.Confidence,
			"processing_time", intent.ProcessingTime)
		return intent, nil
	}

	// Stage 3: Cloud Processing (if enabled)
	if cp.enableCloud && cp.cloudProcessor.enabled {
		if intent, err := cp.cloudProcessor.ProcessIntent(cascadeCtx, text); err == nil {
			intent.ProcessingTime = time.Since(startTime)
			logging.Sugar.Infow("Intent resolved by cloud processor",
				"type", intent.Type,
				"confidence", intent.Confidence,
				"processing_time", intent.ProcessingTime)
			return intent, nil
		}
	}

	// Fallback: Unknown intent
	fallbackIntent := &Intent{
		Type:           IntentUnknown,
		Confidence:     0.0,
		Entities:       make(map[string]interface{}),
		RawText:        text,
		Source:         ProcessorReflex,
		ProcessingTime: time.Since(startTime),
	}

	logging.Sugar.Warnw("Intent cascade failed to classify",
		"text", text,
		"processing_time", fallbackIntent.ProcessingTime)

	return fallbackIntent, nil
}

// NewReflexProcessor creates a new reflex processor with pattern matching
func NewReflexProcessor() *ReflexProcessor {
	rp := &ReflexProcessor{
		patterns: make(map[IntentType][]*regexp.Regexp),
	}

	// Light Control Patterns
	rp.addPatterns(IntentLightControl, []string{
		`(?i)turn\s+(on|off)\s+(?:the\s+)?lights?`,
		`(?i)(dim|brighten)\s+(?:the\s+)?lights?`,
		`(?i)lights?\s+(on|off)`,
		`(?i)set\s+lights?\s+to\s+(\d+)%?`,
	})

	// Media Control Patterns
	rp.addPatterns(IntentMediaControl, []string{
		`(?i)(play|pause|stop)\s+music`,
		`(?i)(next|previous)\s+song`,
		`(?i)(skip|back)`,
		`(?i)play\s+(.+)`,
	})

	// Volume Control Patterns
	rp.addPatterns(IntentVolumeControl, []string{
		`(?i)volume\s+(up|down)`,
		`(?i)(increase|decrease)\s+volume`,
		`(?i)set\s+volume\s+to\s+(\d+)`,
		`(?i)(mute|unmute)`,
	})

	// Time Query Patterns
	rp.addPatterns(IntentTimeQuery, []string{
		`(?i)what\s+time\s+is\s+it`,
		`(?i)current\s+time`,
		`(?i)tell\s+me\s+the\s+time`,
	})

	// Weather Query Patterns
	rp.addPatterns(IntentWeatherQuery, []string{
		`(?i)what'?s\s+the\s+weather`,
		`(?i)weather\s+forecast`,
		`(?i)how'?s\s+the\s+weather`,
		`(?i)is\s+it\s+(raining|sunny|cloudy)`,
	})

	return rp
}

// addPatterns compiles and adds regex patterns for an intent type
func (rp *ReflexProcessor) addPatterns(intentType IntentType, patterns []string) {
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		compiled[i] = regexp.MustCompile(pattern)
	}
	rp.patterns[intentType] = compiled
}

// ProcessIntent attempts to match text against reflex patterns
func (rp *ReflexProcessor) ProcessIntent(ctx context.Context, text string) (*Intent, error) {
	text = strings.TrimSpace(text)

	for intentType, patterns := range rp.patterns {
		for _, pattern := range patterns {
			if matches := pattern.FindStringSubmatch(text); matches != nil {
				entities := make(map[string]interface{})

				// Extract entities from capture groups
				if len(matches) > 1 {
					switch intentType {
					case IntentLightControl:
						if len(matches) > 1 {
							entities["action"] = matches[1]
						}
					case IntentMediaControl:
						if len(matches) > 1 {
							entities["action"] = matches[1]
						}
						if len(matches) > 2 && matches[2] != "" {
							entities["target"] = matches[2]
						}
					case IntentVolumeControl:
						if len(matches) > 1 {
							entities["action"] = matches[1]
						}
					}
				}

				return &Intent{
					Type:       intentType,
					Confidence: 0.95, // High confidence for exact pattern matches
					Entities:   entities,
					RawText:    text,
					Source:     ProcessorReflex,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no reflex pattern matched")
}

// CanHandle checks if reflex processor can handle the text
func (rp *ReflexProcessor) CanHandle(text string) bool {
	intent, _ := rp.ProcessIntent(context.Background(), text)
	return intent != nil
}

// GetConfidenceThreshold returns the confidence threshold
func (rp *ReflexProcessor) GetConfidenceThreshold() float64 {
	return 0.8
}

// ProcessIntent handles LLM-based intent processing
func (lp *LLMProcessor) ProcessIntent(ctx context.Context, text string) (*Intent, error) {
	// LLM-based intent classification via Ollama API
	// In a complete implementation, this would:
	// 1. Send text to Ollama API with intent classification prompt
	// 2. Parse structured response for intent type and entities
	// 3. Return high-confidence intent with extracted entities

	// Simplified implementation for architecture demonstration
	return &Intent{
		Type:       IntentConversation,
		Confidence: 0.5,
		Entities:   make(map[string]interface{}),
		RawText:    text,
		Source:     ProcessorLLM,
	}, nil
}

// CanHandle checks if LLM processor can handle the text
func (lp *LLMProcessor) CanHandle(text string) bool {
	return true // LLM can attempt to handle any text
}

// GetConfidenceThreshold returns the confidence threshold
func (lp *LLMProcessor) GetConfidenceThreshold() float64 {
	return 0.7
}

// ProcessIntent handles cloud-based intent processing
func (cp *CloudProcessor) ProcessIntent(ctx context.Context, text string) (*Intent, error) {
	if !cp.enabled {
		return nil, fmt.Errorf("cloud processor disabled")
	}

	// Cloud-based intent processing (disabled by default for privacy)
	// In a complete implementation, this would make secure API calls to
	// approved cloud services only when explicitly enabled by user

	return nil, fmt.Errorf("cloud processing not implemented")
}

// CanHandle checks if cloud processor can handle the text
func (cp *CloudProcessor) CanHandle(text string) bool {
	return cp.enabled
}

// GetConfidenceThreshold returns the confidence threshold
func (cp *CloudProcessor) GetConfidenceThreshold() float64 {
	return 0.6
}

// SetCloudEnabled enables or disables cloud processing
func (cp *CascadeProcessor) SetCloudEnabled(enabled bool) {
	cp.enableCloud = enabled
	cp.cloudProcessor.enabled = enabled

	if enabled {
		logging.Sugar.Warnw("Cloud processing enabled - data may leave local network")
	} else {
		logging.Sugar.Infow("Cloud processing disabled - maintaining local-first privacy")
	}
}
