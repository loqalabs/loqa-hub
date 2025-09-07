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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// Default response when the command parser cannot understand the user's intent
	defaultUnclearResponse = "I'm not sure what you want me to do."
)

// CommandParser handles command classification using LLM
type CommandParser struct {
	ollamaURL string
	model     string
	client    *http.Client
}

// Command represents a parsed voice command
type Command struct {
	Intent     string            `json:"intent"`
	Entities   map[string]string `json:"entities"`
	Confidence float64           `json:"confidence"`
	Response   string            `json:"response"`
}

// MultiCommand represents multiple parsed voice commands from a compound utterance
type MultiCommand struct {
	Commands         []Command `json:"commands"`
	IsMulti          bool      `json:"is_multi"`
	OriginalText     string    `json:"original_text"`
	CombinedResponse string    `json:"combined_response"`
}

// CommandQueueItem represents a command in the execution queue
type CommandQueueItem struct {
	Command   *Command      `json:"command"`
	Index     int           `json:"index"`
	Executed  bool          `json:"executed"`
	Success   bool          `json:"success"`
	Error     error         `json:"-"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
}

// OllamaRequest represents a request to Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse represents a response from Ollama API
type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// NewCommandParser creates a new command parser
func NewCommandParser(ollamaURL, model string) *CommandParser {
	return &CommandParser{
		ollamaURL: ollamaURL,
		model:     model,
		client:    &http.Client{},
	}
}

// ParseCommand classifies a voice command using LLM, with multi-command support
func (cp *CommandParser) ParseCommand(transcription string) (*Command, error) {
	// First try multi-command parsing
	multiCmd, err := cp.ParseMultiCommand(transcription)
	if err != nil {
		return cp.parseSingleCommand(transcription)
	}

	// If it's not actually a multi-command, return the single command
	if !multiCmd.IsMulti || len(multiCmd.Commands) == 1 {
		if len(multiCmd.Commands) > 0 {
			return &multiCmd.Commands[0], nil
		}
		return cp.parseSingleCommand(transcription)
	}

	// For multi-command, return a combined command for backward compatibility
	return cp.createCombinedCommand(multiCmd), nil
}

// ParseMultiCommand parses a potentially compound utterance into multiple commands
func (cp *CommandParser) ParseMultiCommand(transcription string) (*MultiCommand, error) {
	if transcription == "" {
		return &MultiCommand{
			Commands:         []Command{},
			IsMulti:          false,
			OriginalText:     transcription,
			CombinedResponse: "I didn't hear anything.",
		}, nil
	}

	// Check if this is a compound utterance
	isCompound := cp.detectCompoundUtterance(transcription)

	if !isCompound {
		// Parse as single command
		cmd, err := cp.parseSingleCommand(transcription)
		if err != nil {
			return nil, err
		}
		return &MultiCommand{
			Commands:         []Command{*cmd},
			IsMulti:          false,
			OriginalText:     transcription,
			CombinedResponse: cmd.Response,
		}, nil
	}

	// Parse as multi-command
	return cp.parseCompoundUtterance(transcription)
}

// parseSingleCommand handles single command parsing (original logic)
func (cp *CommandParser) parseSingleCommand(transcription string) (*Command, error) {
	if transcription == "" {
		return &Command{
			Intent:     "unknown",
			Entities:   make(map[string]string),
			Confidence: 0.0,
			Response:   "I didn't hear anything.",
		}, nil
	}

	prompt := cp.buildPrompt(transcription)

	response, err := cp.queryOllama(prompt)
	if err != nil {
		log.Printf("❌ Error querying Ollama: %v", err)
		return &Command{
			Intent:     "unknown",
			Entities:   make(map[string]string),
			Confidence: 0.0,
			Response:   "I'm having trouble understanding you right now.",
		}, nil
	}

	command, err := cp.parseResponse(response)
	if err != nil {
		log.Printf("❌ Error parsing LLM response: %v", err)
		return &Command{
			Intent:     "unknown",
			Entities:   make(map[string]string),
			Confidence: 0.0,
			Response:   defaultUnclearResponse,
		}, nil
	}

	log.Printf("🧠 LLM parsed command - Intent: %s, Confidence: %.2f", command.Intent, command.Confidence)
	return command, nil
}

// buildPrompt creates a structured prompt for command classification
func (cp *CommandParser) buildPrompt(transcription string) string {
	return fmt.Sprintf(`You are a voice assistant command parser. Analyze the following voice command and respond with a JSON object.

Voice command: "%s"

Classify this command and respond with ONLY a JSON object in this exact format:
{
  "intent": "one of: turn_on, turn_off, greeting, question, unknown",
  "entities": {
    "device": "lights, music, tv, etc. or empty string if none",
    "location": "bedroom, kitchen, living room, etc. or empty string if none"
  },
  "confidence": 0.95,
  "response": "A natural response to the user"
}

Rules:
- turn_on: user wants to turn something on (lights, music, etc.)
- turn_off: user wants to turn something off 
- greeting: user is saying hello, hi, good morning, etc.
- question: user is asking a question
- unknown: unclear or unrecognized command
- confidence should be 0.0-1.0 based on how clear the intent is
- response should be natural and conversational
- Only respond with the JSON object, no other text`, transcription)
}

// queryOllama sends a request to Ollama API
func (cp *CommandParser) queryOllama(prompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:  cp.model,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := cp.client.Post(cp.ollamaURL+"/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error making request to Ollama: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	return ollamaResp.Response, nil
}

// parseResponse parses the LLM response into a Command struct
func (cp *CommandParser) parseResponse(response string) (*Command, error) {
	// Clean up the response - sometimes LLMs include extra text
	response = strings.TrimSpace(response)

	// Find JSON object in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	jsonStr := response[start : end+1]

	var command Command
	if err := json.Unmarshal([]byte(jsonStr), &command); err != nil {
		return nil, fmt.Errorf("error unmarshaling command JSON: %w", err)
	}

	// Validate and set defaults
	if command.Intent == "" {
		command.Intent = "unknown"
	}
	if command.Entities == nil {
		command.Entities = make(map[string]string)
	}
	if command.Confidence < 0 || command.Confidence > 1 {
		command.Confidence = 0.5
	}
	if command.Response == "" {
		command.Response = defaultUnclearResponse
	}

	return &command, nil
}

// detectCompoundUtterance checks if the transcription contains multiple commands
func (cp *CommandParser) detectCompoundUtterance(transcription string) bool {
	// Define conjunction patterns that indicate compound utterances
	conjunctionPatterns := []string{
		` and `, ` then `, ` after that `, ` next `, ` also `,
		`, and `, `, then `, `, after that `, `, next `, `, also `,
		` and then `, ` then also `, ` and also `,
	}

	// Define exclusion patterns that are likely to be part of single entities
	exclusionPatterns := []string{
		"rock and roll", "rhythm and blues", "black and white", "salt and pepper",
		"peanut butter and jelly", "research and development", "arts and crafts",
	}

	lowerText := strings.ToLower(transcription)

	// Check for exclusion patterns first
	for _, exclusion := range exclusionPatterns {
		if strings.Contains(lowerText, exclusion) {
			return false
		}
	}

	// Then check for conjunction patterns
	for _, pattern := range conjunctionPatterns {
		if strings.Contains(lowerText, pattern) {
			return true
		}
	}
	return false
}

// parseCompoundUtterance parses a compound utterance into multiple commands
func (cp *CommandParser) parseCompoundUtterance(transcription string) (*MultiCommand, error) {
	prompt := cp.buildMultiCommandPrompt(transcription)

	response, err := cp.queryOllama(prompt)
	if err != nil {
		log.Printf("❌ Error querying Ollama for multi-command: %v", err)
		// Fallback to single command parsing
		cmd, fallbackErr := cp.parseSingleCommand(transcription)
		if fallbackErr != nil {
			return nil, err
		}
		return &MultiCommand{
			Commands:         []Command{*cmd},
			IsMulti:          false,
			OriginalText:     transcription,
			CombinedResponse: cmd.Response,
		}, nil
	}

	multiCmd, err := cp.parseMultiCommandResponse(response, transcription)
	if err != nil {
		log.Printf("❌ Error parsing multi-command response: %v", err)
		// Fallback to single command parsing
		cmd, fallbackErr := cp.parseSingleCommand(transcription)
		if fallbackErr != nil {
			return nil, err
		}
		return &MultiCommand{
			Commands:         []Command{*cmd},
			IsMulti:          false,
			OriginalText:     transcription,
			CombinedResponse: cmd.Response,
		}, nil
	}

	log.Printf("🧠 LLM parsed multi-command - %d commands from: %s", len(multiCmd.Commands), transcription)
	return multiCmd, nil
}

// buildMultiCommandPrompt creates a structured prompt for multi-command classification
func (cp *CommandParser) buildMultiCommandPrompt(transcription string) string {
	return fmt.Sprintf(`You are a voice assistant command parser that handles compound utterances with multiple commands. Analyze the following voice command and respond with a JSON object.

Voice command: "%s"

If this contains multiple distinct commands (connected by "and", "then", "after that", etc.), break them down into separate commands. If it's just one command, return it as a single command.

Respond with ONLY a JSON object in this exact format:
{
  "is_multi": true/false,
  "commands": [
    {
      "intent": "one of: turn_on, turn_off, greeting, question, unknown",
      "entities": {
        "device": "lights, music, tv, etc. or empty string if none",
        "location": "bedroom, kitchen, living room, etc. or empty string if none"
      },
      "confidence": 0.95,
      "response": "A natural response for this specific command"
    }
  ],
  "combined_response": "A single natural response combining all commands"
}

Rules:
- is_multi: true if there are multiple distinct commands, false otherwise
- For each command: turn_on (turn something on), turn_off (turn something off), greeting (hello/hi), question (asking something), unknown (unclear)
- confidence should be 0.0-1.0 based on how clear each intent is
- response should be natural and conversational for each individual command
- combined_response should be a single natural response acknowledging all commands
- Only respond with the JSON object, no other text`, transcription)
}

// parseMultiCommandResponse parses the LLM response into a MultiCommand struct
func (cp *CommandParser) parseMultiCommandResponse(response, originalText string) (*MultiCommand, error) {
	// Clean up the response - sometimes LLMs include extra text
	response = strings.TrimSpace(response)

	// Find JSON object in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")

	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("no valid JSON found in multi-command response")
	}

	jsonStr := response[start : end+1]

	// Define a temporary struct for parsing
	type MultiCommandResponse struct {
		IsMulti          bool      `json:"is_multi"`
		Commands         []Command `json:"commands"`
		CombinedResponse string    `json:"combined_response"`
	}

	var multiResp MultiCommandResponse
	if err := json.Unmarshal([]byte(jsonStr), &multiResp); err != nil {
		return nil, fmt.Errorf("error unmarshaling multi-command JSON: %w", err)
	}

	// Validate and set defaults for each command
	for i := range multiResp.Commands {
		cmd := &multiResp.Commands[i]
		if cmd.Intent == "" {
			cmd.Intent = "unknown"
		}
		if cmd.Entities == nil {
			cmd.Entities = make(map[string]string)
		}
		if cmd.Confidence < 0 || cmd.Confidence > 1 {
			cmd.Confidence = 0.5
		}
		if cmd.Response == "" {
			cmd.Response = defaultUnclearResponse
		}
	}

	// Set default combined response
	if multiResp.CombinedResponse == "" {
		if len(multiResp.Commands) > 1 {
			multiResp.CombinedResponse = "I'll handle those commands for you."
		} else if len(multiResp.Commands) == 1 {
			multiResp.CombinedResponse = multiResp.Commands[0].Response
		} else {
			multiResp.CombinedResponse = defaultUnclearResponse
		}
	}

	return &MultiCommand{
		Commands:         multiResp.Commands,
		IsMulti:          multiResp.IsMulti && len(multiResp.Commands) > 1,
		OriginalText:     originalText,
		CombinedResponse: multiResp.CombinedResponse,
	}, nil
}

// createCombinedCommand creates a single command from a multi-command for backward compatibility
func (cp *CommandParser) createCombinedCommand(multiCmd *MultiCommand) *Command {
	if len(multiCmd.Commands) == 0 {
		return &Command{
			Intent:     "unknown",
			Entities:   make(map[string]string),
			Confidence: 0.0,
			Response:   defaultUnclearResponse,
		}
	}

	if len(multiCmd.Commands) == 1 {
		return &multiCmd.Commands[0]
	}

	// Combine multiple commands into a single one
	// Use the intent of the first command, combine entities, and use average confidence
	firstCmd := multiCmd.Commands[0]
	combinedEntities := make(map[string]string)
	totalConfidence := 0.0

	for _, cmd := range multiCmd.Commands {
		for k, v := range cmd.Entities {
			if existing, exists := combinedEntities[k]; exists {
				// If entity exists, combine with comma
				combinedEntities[k] = existing + ", " + v
			} else {
				combinedEntities[k] = v
			}
		}
		totalConfidence += cmd.Confidence
	}

	avgConfidence := totalConfidence / float64(len(multiCmd.Commands))

	return &Command{
		Intent:     "multi_" + firstCmd.Intent,
		Entities:   combinedEntities,
		Confidence: avgConfidence,
		Response:   multiCmd.CombinedResponse,
	}
}

// TestConnection tests if Ollama is accessible and the model is available
func (cp *CommandParser) TestConnection() error {
	// Test basic connection
	resp, err := cp.client.Get(cp.ollamaURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot connect to Ollama at %s: %w", cp.ollamaURL, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama API returned status %d", resp.StatusCode)
	}

	// Test model availability with a simple query
	testPrompt := "Respond with just 'OK'"
	_, err = cp.queryOllama(testPrompt)
	if err != nil {
		return fmt.Errorf("model %s not available or not responding: %w", cp.model, err)
	}

	log.Printf("✅ LLM Command Parser connected to Ollama (%s) with model %s", cp.ollamaURL, cp.model)
	return nil
}
