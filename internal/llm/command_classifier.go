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

package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CommandClassifier enhances basic command parsing with predictive classification
type CommandClassifier struct {
	baseParser         *CommandParser
	reliabilityTracker *DeviceReliabilityTracker
	deviceTimings      map[string]time.Duration // Known execution times for different devices

	// Classification rules
	highConfidenceThreshold float64
	reliabilityThreshold    float64
	slowOperationThreshold  time.Duration
}

// IntentCategory represents different types of user intents and capabilities
type IntentCategory string

const (
	// Core system capabilities
	CategoryInformation   IntentCategory = "information"   // Web search, knowledge queries, calculations
	CategoryCommunication IntentCategory = "communication" // Email, messaging, calls, social media
	CategoryProductivity  IntentCategory = "productivity"  // Calendar, reminders, notes, tasks
	CategoryEntertainment IntentCategory = "entertainment" // Music, videos, games, stories
	CategoryNavigation    IntentCategory = "navigation"    // Maps, directions, traffic, travel
	CategoryWeather       IntentCategory = "weather"       // Weather forecasts, alerts, conditions
	CategoryNews          IntentCategory = "news"          // News updates, headlines, summaries
	CategoryShopping      IntentCategory = "shopping"      // Online shopping, price comparison, orders
	CategoryHealth        IntentCategory = "health"        // Health tracking, medical info, fitness
	CategoryEducation     IntentCategory = "education"     // Learning, tutorials, language, skills

	// Home automation (subset of broader capabilities)
	CategorySmartHome  IntentCategory = "smart_home" // IoT devices, lights, climate, security
	CategoryMultimedia IntentCategory = "multimedia" // Local media playback, streaming control

	// System and utility
	CategorySystem    IntentCategory = "system"    // System settings, configurations, diagnostics
	CategoryDeveloper IntentCategory = "developer" // Development tools, coding assistance, APIs
	CategoryGeneral   IntentCategory = "general"   // Conversational, misc requests
)

// OperationType represents the kind of operation being performed
type OperationType string

const (
	OperationControl     OperationType = "control"     // Basic on/off/adjust
	OperationQuery       OperationType = "query"       // Status requests
	OperationSequence    OperationType = "sequence"    // Multi-step operations
	OperationCritical    OperationType = "critical"    // Security/safety operations
	OperationMaintenance OperationType = "maintenance" // System operations
)

// NewCommandClassifier creates an enhanced command classifier
func NewCommandClassifier(baseParser *CommandParser, reliabilityTracker *DeviceReliabilityTracker) *CommandClassifier {
	return &CommandClassifier{
		baseParser:         baseParser,
		reliabilityTracker: reliabilityTracker,
		deviceTimings:      initializeDeviceTimings(),

		highConfidenceThreshold: 0.85,
		reliabilityThreshold:    0.80,
		slowOperationThreshold:  5 * time.Second,
	}
}

// ClassifyCommand performs enhanced classification for predictive responses
func (cc *CommandClassifier) ClassifyCommand(ctx context.Context, transcript string) (*CommandClassification, error) {
	// 1. Get basic command parsing from existing parser
	command, err := cc.baseParser.ParseCommand(transcript)
	if err != nil {
		return nil, fmt.Errorf("base command parsing failed: %w", err)
	}

	// 2. Extract intent category and operation details
	intentCategory := cc.extractIntentCategory(command.Intent, command.Entities)
	operationType := cc.extractOperationType(command.Intent, command.Entities)
	targetID := cc.extractTargetID(command.Entities)

	// 3. Get target reliability and execution time estimates
	targetReliability := cc.reliabilityTracker.GetReliabilityScore(targetID)
	estimatedTime := cc.getEstimatedExecutionTime(intentCategory, operationType)

	// 4. Determine response type based on classification
	responseType := cc.determineResponseType(command.Confidence, targetReliability, estimatedTime, operationType)

	// 5. Choose update strategy
	updateStrategy := cc.determineUpdateStrategy(responseType, intentCategory, operationType, estimatedTime)

	return &CommandClassification{
		Intent:            command.Intent,
		Entities:          command.Entities,
		Confidence:        command.Confidence,
		DeviceReliability: targetReliability, // Keep field name for compatibility
		ExecutionTime:     estimatedTime,
		ResponseType:      responseType,
		UpdateStrategy:    updateStrategy,
	}, nil
}

// extractIntentCategory identifies the broad category of user intent
func (cc *CommandClassifier) extractIntentCategory(intent string, entities map[string]string) IntentCategory {
	intentLower := strings.ToLower(intent)

	// Weather requests (check before general information)
	switch {
	case strings.Contains(intentLower, "weather") || strings.Contains(intentLower, "temperature") ||
		strings.Contains(intentLower, "forecast") || strings.Contains(intentLower, "rain") ||
		strings.Contains(intentLower, "snow") || strings.Contains(intentLower, "sunny"):
		return CategoryWeather

	// Information and knowledge requests
	case strings.Contains(intentLower, "what") || strings.Contains(intentLower, "how") ||
		strings.Contains(intentLower, "why") || strings.Contains(intentLower, "explain") ||
		strings.Contains(intentLower, "search") || strings.Contains(intentLower, "find") ||
		strings.Contains(intentLower, "calculate") || strings.Contains(intentLower, "convert"):
		return CategoryInformation

	// News and current events
	case strings.Contains(intentLower, "news") || strings.Contains(intentLower, "headlines") ||
		strings.Contains(intentLower, "current events") || strings.Contains(intentLower, "breaking"):
		return CategoryNews

	// Entertainment and media
	case strings.Contains(intentLower, "play") || strings.Contains(intentLower, "music") ||
		strings.Contains(intentLower, "song") || strings.Contains(intentLower, "video") ||
		strings.Contains(intentLower, "movie") || strings.Contains(intentLower, "podcast") ||
		strings.Contains(intentLower, "radio") || strings.Contains(intentLower, "stream"):
		return CategoryEntertainment

	// Productivity and organization
	case strings.Contains(intentLower, "reminder") || strings.Contains(intentLower, "schedule") ||
		strings.Contains(intentLower, "calendar") || strings.Contains(intentLower, "appointment") ||
		strings.Contains(intentLower, "note") || strings.Contains(intentLower, "task") ||
		strings.Contains(intentLower, "todo") || strings.Contains(intentLower, "meeting"):
		return CategoryProductivity

	// Communication
	case strings.Contains(intentLower, "call") || strings.Contains(intentLower, "message") ||
		strings.Contains(intentLower, "email") || strings.Contains(intentLower, "text") ||
		strings.Contains(intentLower, "send") || strings.Contains(intentLower, "contact"):
		return CategoryCommunication

	// Navigation and travel
	case strings.Contains(intentLower, "directions") || strings.Contains(intentLower, "navigate") ||
		strings.Contains(intentLower, "route") || strings.Contains(intentLower, "traffic") ||
		strings.Contains(intentLower, "map") || strings.Contains(intentLower, "location") ||
		strings.Contains(intentLower, "distance") || strings.Contains(intentLower, "travel"):
		return CategoryNavigation

	// Shopping and commerce
	case strings.Contains(intentLower, "buy") || strings.Contains(intentLower, "purchase") ||
		strings.Contains(intentLower, "order") || strings.Contains(intentLower, "shop") ||
		strings.Contains(intentLower, "price") || strings.Contains(intentLower, "deal") ||
		strings.Contains(intentLower, "compare") || strings.Contains(intentLower, "cart"):
		return CategoryShopping

	// Health and fitness
	case strings.Contains(intentLower, "health") || strings.Contains(intentLower, "fitness") ||
		strings.Contains(intentLower, "exercise") || strings.Contains(intentLower, "calories") ||
		strings.Contains(intentLower, "steps") || strings.Contains(intentLower, "sleep") ||
		strings.Contains(intentLower, "heart rate") || strings.Contains(intentLower, "medical"):
		return CategoryHealth

	// Education and learning
	case strings.Contains(intentLower, "learn") || strings.Contains(intentLower, "teach") ||
		strings.Contains(intentLower, "lesson") || strings.Contains(intentLower, "course") ||
		strings.Contains(intentLower, "tutorial") || strings.Contains(intentLower, "study") ||
		strings.Contains(intentLower, "language") || strings.Contains(intentLower, "practice"):
		return CategoryEducation

	// Smart home and IoT (subset of broader capabilities)
	case strings.Contains(intentLower, "light") || strings.Contains(intentLower, "heat") ||
		strings.Contains(intentLower, "cool") || strings.Contains(intentLower, "door") ||
		strings.Contains(intentLower, "lock") || strings.Contains(intentLower, "alarm") ||
		strings.Contains(intentLower, "security") || strings.Contains(intentLower, "thermostat") ||
		strings.Contains(intentLower, "garage") || strings.Contains(intentLower, "smart"):
		return CategorySmartHome

	// System and configuration
	case strings.Contains(intentLower, "setting") || strings.Contains(intentLower, "config") ||
		strings.Contains(intentLower, "system") || strings.Contains(intentLower, "restart") ||
		strings.Contains(intentLower, "update") || strings.Contains(intentLower, "install") ||
		strings.Contains(intentLower, "debug") || strings.Contains(intentLower, "status"):
		return CategorySystem

	// Developer tools and programming
	case strings.Contains(intentLower, "code") || strings.Contains(intentLower, "program") ||
		strings.Contains(intentLower, "debug") || strings.Contains(intentLower, "api") ||
		strings.Contains(intentLower, "deploy") || strings.Contains(intentLower, "build") ||
		strings.Contains(intentLower, "test") || strings.Contains(intentLower, "git"):
		return CategoryDeveloper
	}

	// Check entities for additional context
	if topic, ok := entities["topic"]; ok {
		topicLower := strings.ToLower(topic)
		switch {
		case strings.Contains(topicLower, "weather"):
			return CategoryWeather
		case strings.Contains(topicLower, "music") || strings.Contains(topicLower, "entertainment"):
			return CategoryEntertainment
		case strings.Contains(topicLower, "news"):
			return CategoryNews
		case strings.Contains(topicLower, "health"):
			return CategoryHealth
		}
	}

	return CategoryGeneral
}

// extractOperationType identifies the type of operation being performed
func (cc *CommandClassifier) extractOperationType(intent string, _ map[string]string) OperationType {
	intentLower := strings.ToLower(intent)

	// Check for query operations
	if strings.Contains(intentLower, "status") || strings.Contains(intentLower, "check") ||
		strings.Contains(intentLower, "what") || strings.Contains(intentLower, "is") {
		return OperationQuery
	}

	// Check for critical operations
	if strings.Contains(intentLower, "security") || strings.Contains(intentLower, "alarm") ||
		strings.Contains(intentLower, "lock") || strings.Contains(intentLower, "unlock") {
		return OperationCritical
	}

	// Check for sequence operations (multiple actions)
	if strings.Contains(intentLower, "and") || strings.Contains(intentLower, "then") ||
		strings.Contains(intentLower, "also") {
		return OperationSequence
	}

	// Check for maintenance operations
	if strings.Contains(intentLower, "restart") || strings.Contains(intentLower, "reset") ||
		strings.Contains(intentLower, "update") || strings.Contains(intentLower, "configure") {
		return OperationMaintenance
	}

	return OperationControl
}

// extractTargetID creates a unique identifier for the target being operated on
func (cc *CommandClassifier) extractTargetID(entities map[string]string) string {
	// For smart home devices
	location := entities["location"]
	device := entities["device"]

	// For other categories
	service := entities["service"]
	topic := entities["topic"]
	action := entities["action"]

	// Smart home devices
	if location != "" && device != "" {
		return fmt.Sprintf("device_%s_%s", location, device)
	}
	if location != "" {
		return fmt.Sprintf("location_%s", location)
	}
	if device != "" {
		return fmt.Sprintf("device_%s", device)
	}

	// Web services and APIs
	if service != "" {
		return fmt.Sprintf("service_%s", service)
	}

	// Information topics
	if topic != "" {
		return fmt.Sprintf("topic_%s", topic)
	}

	// Action-based
	if action != "" {
		return fmt.Sprintf("action_%s", action)
	}

	return "general_request"
}

// getEstimatedExecutionTime returns expected execution time based on intent category and operation
func (cc *CommandClassifier) getEstimatedExecutionTime(category IntentCategory, operation OperationType) time.Duration {
	baseTime, exists := cc.deviceTimings[string(category)]
	if !exists {
		baseTime = 2 * time.Second // Default
	}

	// Adjust based on operation type
	switch operation {
	case OperationQuery:
		return baseTime / 2 // Information queries are typically faster
	case OperationSequence:
		return baseTime * 2 // Multiple operations take longer
	case OperationCritical:
		return baseTime * 3 // Security/financial operations may need confirmations
	case OperationMaintenance:
		return baseTime * 5 // System operations can be slow
	default:
		return baseTime
	}
}

// determineResponseType classifies how confident we can be in the response
func (cc *CommandClassifier) determineResponseType(confidence, reliability float64, executionTime time.Duration, operation OperationType) PredictiveType {
	// Critical operations always require confirmation
	if operation == OperationCritical {
		return PredictiveConfirm
	}

	// Slow operations need progress updates
	if executionTime > cc.slowOperationThreshold {
		return PredictiveProgress
	}

	// High confidence + reliable device = optimistic response
	if confidence >= cc.highConfidenceThreshold && reliability >= cc.reliabilityThreshold {
		return PredictiveOptimistic
	}

	// Everything else gets cautious handling
	return PredictiveCautious
}

// determineUpdateStrategy decides when and how to send follow-up messages
func (cc *CommandClassifier) determineUpdateStrategy(responseType PredictiveType, category IntentCategory, _ OperationType, executionTime time.Duration) UpdateStrategy {
	switch responseType {
	case PredictiveOptimistic:
		// Reliable operations - only report failures
		if category == CategorySmartHome || category == CategoryMultimedia || category == CategoryInformation {
			return UpdateSilent // These are routine and reliable
		}
		return UpdateErrorOnly

	case PredictiveCautious:
		// Uncertain operations - report errors and unexpected successes
		return UpdateErrorOnly

	case PredictiveConfirm:
		// Critical operations - verbose updates
		return UpdateVerbose

	case PredictiveProgress:
		// Slow operations - progress updates
		return UpdateProgress

	default:
		return UpdateErrorOnly
	}
}

// initializeDeviceTimings sets up expected execution times for different intent categories
func initializeDeviceTimings() map[string]time.Duration {
	return map[string]time.Duration{
		// Fast response categories
		string(CategoryInformation):   500 * time.Millisecond, // Quick searches and calculations
		string(CategoryWeather):       1 * time.Second,        // Weather API calls
		string(CategoryNews):          1 * time.Second,        // News feed retrieval
		string(CategoryEntertainment): 2 * time.Second,        // Media streaming control

		// Medium response categories
		string(CategoryProductivity):  3 * time.Second, // Calendar/reminder operations
		string(CategoryCommunication): 2 * time.Second, // Send messages/calls
		string(CategoryShopping):      4 * time.Second, // Shopping API calls
		string(CategoryHealth):        2 * time.Second, // Health data retrieval
		string(CategoryEducation):     3 * time.Second, // Learning content access

		// Smart home (variable based on device)
		string(CategorySmartHome):  3 * time.Second, // Average smart device response
		string(CategoryMultimedia): 2 * time.Second, // Local media control

		// Slower categories
		string(CategoryNavigation): 5 * time.Second,  // Route calculation
		string(CategorySystem):     8 * time.Second,  // System operations
		string(CategoryDeveloper):  10 * time.Second, // Development operations

		// Default
		string(CategoryGeneral): 2 * time.Second, // Conversational responses
	}
}

// GetIntentTimings returns the current intent category timing estimates
func (cc *CommandClassifier) GetIntentTimings() map[string]time.Duration {
	timings := make(map[string]time.Duration)
	for k, v := range cc.deviceTimings {
		timings[k] = v
	}
	return timings
}

// UpdateIntentTiming adjusts the expected execution time for an intent category
func (cc *CommandClassifier) UpdateIntentTiming(category IntentCategory, actualTime time.Duration) {
	key := string(category)
	currentTime, exists := cc.deviceTimings[key]

	if !exists {
		cc.deviceTimings[key] = actualTime
	} else {
		// Use exponential moving average (70% old, 30% new)
		cc.deviceTimings[key] = time.Duration(float64(currentTime)*0.7 + float64(actualTime)*0.3)
	}
}

// SetThresholds allows tuning of classification parameters
func (cc *CommandClassifier) SetThresholds(confidence, reliability float64, slowOperation time.Duration) {
	cc.highConfidenceThreshold = confidence
	cc.reliabilityThreshold = reliability
	cc.slowOperationThreshold = slowOperation
}
