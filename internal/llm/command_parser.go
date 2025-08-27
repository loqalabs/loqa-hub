package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
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

// ParseCommand classifies a voice command using LLM
func (cp *CommandParser) ParseCommand(transcription string) (*Command, error) {
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
			Response:   "I'm not sure what you want me to do.",
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
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
		command.Response = "I'm not sure what you want me to do."
	}

	return &command, nil
}

// TestConnection tests if Ollama is accessible and the model is available
func (cp *CommandParser) TestConnection() error {
	// Test basic connection
	resp, err := cp.client.Get(cp.ollamaURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot connect to Ollama at %s: %w", cp.ollamaURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
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