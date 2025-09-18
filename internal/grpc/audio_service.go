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

package grpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/loqalabs/loqa-hub/internal/config"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/messaging"
	"github.com/loqalabs/loqa-hub/internal/skills"
	"github.com/loqalabs/loqa-hub/internal/storage"
	pb "github.com/loqalabs/loqa-proto/go/audio"
	"go.uber.org/zap"
)

// RelayStream represents an active relay connection with arbitration data
type RelayStream struct {
	Stream         pb.AudioService_StreamAudioServer
	RelayID        string
	ConnectedAt    time.Time
	WakeWordSignal []float32
	SpeechAudio    []float32
	SignalStrength float64
	Status         RelayStatus
	CancelChannel  chan struct{}
}

// RelayStatus represents the current state of a relay in arbitration
type RelayStatus int

const (
	RelayStatusConnected RelayStatus = iota
	RelayStatusContending
	RelayStatusWinner
	RelayStatusCancelled
)

// ArbitrationWindow manages the temporal window for relay arbitration
type ArbitrationWindow struct {
	StartTime      time.Time
	WindowDuration time.Duration
	Relays         map[string]*RelayStream
	IsActive       bool
	WinnerID       string
	mutex          sync.RWMutex
}

// AudioService implements the gRPC AudioService
type AudioService struct {
	pb.UnimplementedAudioServiceServer
	transcriber               llm.Transcriber
	commandParser             *llm.CommandParser             // Fallback parser
	streamingPredictiveBridge *llm.StreamingPredictiveBridge // Primary processing engine
	ttsClient                 llm.TextToSpeech
	natsService               *messaging.NATSService
	audioStreamPublisher      *messaging.AudioStreamPublisher // NATS chunked audio streaming
	eventsStore               *storage.VoiceEventsStore
	currentExecutionContext   *CommandExecutionContext

	// Multi-relay collision detection
	arbitrationWindow         *ArbitrationWindow
	activeStreams             map[string]*RelayStream
	streamsMutex              sync.RWMutex
	arbitrationWindowDuration time.Duration
}

// CommandExecutionContext holds context information for command execution
type CommandExecutionContext struct {
	RelayID       string
	RequestID     string
	EventUUID     string
	Transcription string
}

// SetCommandExecutionContext sets the context for command execution (stored in AudioService)
func (as *AudioService) SetCommandExecutionContext(ctx *CommandExecutionContext) {
	as.currentExecutionContext = ctx
}

// Implement llm.CommandExecutor interface for AudioService
func (as *AudioService) ExecuteCommand(ctx context.Context, cmd *llm.Command) error {
	// For now, we just publish the command to NATS
	// In the future, this could be extended to handle actual command execution
	if as.natsService == nil || !as.natsService.IsConnected() {
		return fmt.Errorf("NATS service not available")
	}

	// Use the current execution context if available
	relayID := "multi-cmd"
	requestID := "multi-cmd"
	transcription := cmd.Response

	if as.currentExecutionContext != nil {
		relayID = as.currentExecutionContext.RelayID
		requestID = as.currentExecutionContext.RequestID
		transcription = as.currentExecutionContext.Transcription
	}

	commandEvent := &messaging.CommandEvent{
		RelayID:       relayID,
		Transcription: transcription,
		Intent:        cmd.Intent,
		Entities:      cmd.Entities,
		Confidence:    cmd.Confidence,
		Timestamp:     time.Now().UnixNano(),
		RequestID:     requestID,
	}

	log.Printf("🎯 Executing individual command: %s (part of multi-command)", cmd.Intent)

	err := as.natsService.PublishVoiceCommand(commandEvent)
	if err != nil {
		return fmt.Errorf("failed to publish command %s: %w", cmd.Intent, err)
	}

	// Also publish device command if applicable
	if as.isDeviceCommand(cmd.Intent) {
		deviceCommand := as.createDeviceCommand(commandEvent)
		if deviceCommand != nil {
			if deviceErr := as.natsService.PublishDeviceCommand(deviceCommand); deviceErr != nil {
				log.Printf("❌ Failed to publish device command for %s: %v", cmd.Intent, deviceErr)
				// Don't fail the entire command execution for device command publishing failure
			}
		}
	}

	return nil
}

// NewAudioServiceWithSTT creates a new audio service using OpenAI-compatible STT service
func NewAudioServiceWithSTT(sttURL, sttLanguage string, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {
	transcriber, err := llm.NewSTTClient(sttURL, sttLanguage)
	if err != nil {
		return nil, err
	}
	return createAudioService(transcriber, nil, eventsStore)
}

// NewAudioServiceWithTTS creates a new audio service with both STT and TTS support
func NewAudioServiceWithTTS(sttURL, sttLanguage string, ttsConfig config.TTSConfig, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {
	return NewAudioServiceWithTTSAndOptions(sttURL, sttLanguage, ttsConfig, eventsStore, true)
}

// NewAudioServiceWithTTSAndOptions creates a new audio service with configurable health checks (for testing)
func NewAudioServiceWithTTSAndOptions(sttURL, sttLanguage string, ttsConfig config.TTSConfig, eventsStore *storage.VoiceEventsStore, enableHealthCheck bool) (*AudioService, error) {
	// Initialize STT client
	transcriber, err := llm.NewSTTClientWithOptions(sttURL, sttLanguage, enableHealthCheck)
	if err != nil {
		return nil, fmt.Errorf("failed to create STT client: %w", err)
	}

	// Initialize TTS client
	var ttsClient llm.TextToSpeech
	if ttsConfig.URL != "" {
		ttsClient, err = llm.NewOpenAITTSClient(ttsConfig)
		if err != nil {
			if ttsConfig.FallbackEnabled {
				logging.LogWarn("Failed to initialize TTS, continuing without TTS",
					zap.Error(err),
					zap.String("tts_url", ttsConfig.URL),
				)
				ttsClient = nil
			} else {
				return nil, fmt.Errorf("failed to create TTS client: %w", err)
			}
		}
	}

	return createAudioService(transcriber, ttsClient, eventsStore)
}

// createAudioService is a helper to create the service with any transcriber implementation
func createAudioService(transcriber llm.Transcriber, ttsClient llm.TextToSpeech, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {

	// Initialize command parser with Ollama
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}

	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaModel == "" {
		ollamaModel = "llama3.2:3b"
	}

	commandParser := llm.NewCommandParser(ollamaURL, ollamaModel)

	// Initialize StreamingPredictiveBridge (if possible) for fast responses
	var streamingPredictiveBridge *llm.StreamingPredictiveBridge

	// Create a minimal skill manager implementation for the bridge
	// Note: In production, this should be passed from the server
	skillManagerAdapter := &SkillManagerAdapter{}

	// Try to create the streaming predictive bridge
	streamingPredictiveBridge = createStreamingPredictiveBridge(ollamaURL, ollamaModel, skillManagerAdapter, ttsClient)

	// Initialize NATS service
	natsService, err := messaging.NewNATSService()
	if err != nil {
		log.Printf("⚠️  Warning: Failed to create NATS service: %v", err)
	}

	// Create AudioService first
	audioService := &AudioService{
		transcriber:               transcriber,
		commandParser:             commandParser,
		streamingPredictiveBridge: streamingPredictiveBridge,
		ttsClient:                 ttsClient,
		natsService:               natsService,
		audioStreamPublisher:      nil, // Will be set when NATS connects
		eventsStore:               eventsStore,
		activeStreams:             make(map[string]*RelayStream),
		arbitrationWindowDuration: 300 * time.Millisecond, // Configurable arbitration window
	}

	// Connect to NATS and initialize audio stream publisher (non-blocking)
	go func() {
		if natsService != nil {
			if err := natsService.Connect(); err != nil {
				log.Printf("⚠️  Warning: Cannot connect to NATS: %v", err)
				log.Println("🔄 Events will not be published to message bus")
			} else {
				// Initialize audio stream publisher with 4KB chunks and assign to service
				audioService.audioStreamPublisher = messaging.NewAudioStreamPublisher(natsService.GetConnection(), 4096)
				log.Println("🎵 Audio stream publisher initialized")
			}
		}
	}()

	// Test connection to Ollama with automatic retry (non-blocking)
	go func() {
		maxRetries := 10
		retryDelay := 15 * time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := commandParser.TestConnection(); err != nil {
				if attempt == 1 {
					log.Printf("⚠️  Warning: Cannot connect to Ollama: %v", err)
					log.Println("🔄 Command parsing will use fallback logic")
				}

				if attempt < maxRetries {
					log.Printf("🔄 Ollama connection attempt %d/%d failed, retrying in %v...", attempt, maxRetries, retryDelay)
					time.Sleep(retryDelay)
					continue
				} else {
					log.Printf("❌ Ollama connection failed after %d attempts. Service will continue with fallback logic.", maxRetries)
					return
				}
			} else {
				if attempt > 1 {
					log.Printf("✅ Ollama connection recovered after %d attempts", attempt)
				}
				return
			}
		}
	}()

	return audioService, nil
}

// SkillManagerAdapter provides a minimal SkillManagerInterface implementation
// for cases where a full skill manager is not available
type SkillManagerAdapter struct{}

// FindSkillForIntent implements a basic skill finding logic
func (sma *SkillManagerAdapter) FindSkillForIntent(intent *skills.VoiceIntent) (skills.SkillPlugin, error) {
	return nil, fmt.Errorf("no skills available")
}

// streamAudioResponse sends audio response via NATS chunked streaming instead of gRPC
func (s *AudioService) streamAudioResponse(relayID string, audioData []byte, audioFormat string, messageType string, priority int) error {
	if s.audioStreamPublisher == nil {
		log.Printf("⚠️  NATS audio publisher not available, skipping response to relay %s", relayID)
		return nil
	}

	if len(audioData) == 0 {
		log.Printf("🎵 No audio data to stream for relay %s, sending text-only response", relayID)
		return nil
	}

	// Determine sample rate based on audio format
	sampleRate := 16000 // Default for PCM
	if audioFormat == "mp3" {
		sampleRate = 22050 // Common for TTS
	}

	// Create audio reader from byte data
	audioReader := bytes.NewReader(audioData)

	// Stream audio to specific relay
	if err := s.audioStreamPublisher.StreamAudioToRelay(
		relayID,
		audioReader,
		audioFormat,
		sampleRate,
		messageType,
		priority,
	); err != nil {
		return fmt.Errorf("failed to stream audio to relay %s: %w", relayID, err)
	}

	log.Printf("🎵 Successfully streamed audio response to relay %s (%d bytes, %s)",
		relayID, len(audioData), messageType)
	return nil
}

// ExecuteSkill implements basic skill execution
func (sma *SkillManagerAdapter) ExecuteSkill(ctx context.Context, skill skills.SkillPlugin, intent *skills.VoiceIntent) (*skills.SkillResponse, error) {
	return nil, fmt.Errorf("skill execution not available")
}

// createStreamingPredictiveBridge initializes the streaming predictive bridge
func createStreamingPredictiveBridge(ollamaURL, ollamaModel string, skillManager llm.SkillManagerInterface, ttsClient llm.TextToSpeech) *llm.StreamingPredictiveBridge {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  Failed to create StreamingPredictiveBridge, will use fallback: %v", r)
		}
	}()

	// Create base command parser for classifier
	baseCommandParser := llm.NewCommandParser(ollamaURL, ollamaModel)

	// Create predictive response engine
	predictiveEngine := llm.NewPredictiveResponseEngine(skillManager)
	if predictiveEngine == nil {
		log.Printf("⚠️  Failed to create PredictiveResponseEngine, will use fallback")
		return nil
	}

	// Create async execution pipeline
	executionPipeline := llm.NewAsyncExecutionPipeline(skillManager)
	if executionPipeline == nil {
		log.Printf("⚠️  Failed to create AsyncExecutionPipeline, will use fallback")
		return nil
	}

	// Create device reliability tracker
	reliabilityTracker := llm.NewDeviceReliabilityTracker()

	// Create command classifier
	commandClassifier := llm.NewCommandClassifier(baseCommandParser, reliabilityTracker)
	if commandClassifier == nil {
		log.Printf("⚠️  Failed to create CommandClassifier, will use fallback")
		return nil
	}

	// Create status manager with proper initialization
	statusManager := llm.NewStatusManager(ttsClient)

	// Create streaming command parser
	streamingParser := llm.NewStreamingCommandParser(ollamaURL, ollamaModel, true)
	if streamingParser == nil {
		log.Printf("⚠️  Failed to create StreamingCommandParser, will use fallback")
		return nil
	}

	// Create the bridge
	bridge := llm.NewStreamingPredictiveBridge(
		streamingParser,
		predictiveEngine,
		statusManager,
		commandClassifier,
		executionPipeline,
	)

	if bridge != nil {
		log.Printf("✅ StreamingPredictiveBridge initialized successfully")
	} else {
		log.Printf("⚠️  Failed to create StreamingPredictiveBridge, will use fallback")
	}

	return bridge
}

// startArbitrationWindow initiates a new arbitration window
func (as *AudioService) startArbitrationWindow(relayID string, stream pb.AudioService_StreamAudioServer) *ArbitrationWindow {
	as.streamsMutex.Lock()
	defer as.streamsMutex.Unlock()

	// Create new arbitration window
	window := &ArbitrationWindow{
		StartTime:      time.Now(),
		WindowDuration: as.arbitrationWindowDuration,
		Relays:         make(map[string]*RelayStream),
		IsActive:       true,
		mutex:          sync.RWMutex{}, // Initialize mutex
	}

	// Add the initial relay (with window locking)
	window.mutex.Lock()
	relayStream := &RelayStream{
		Stream:        stream,
		RelayID:       relayID,
		ConnectedAt:   time.Now(),
		Status:        RelayStatusContending,
		CancelChannel: make(chan struct{}),
	}

	window.Relays[relayID] = relayStream
	window.mutex.Unlock()

	as.activeStreams[relayID] = relayStream
	as.arbitrationWindow = window

	logging.LogAudioProcessing(relayID, "arbitration_window_started",
		zap.Duration("window_duration", as.arbitrationWindowDuration),
		zap.String("first_relay", relayID),
	)

	// Start arbitration timer
	go as.runArbitrationTimer(window)

	return window
}

// joinArbitrationWindow adds a relay to an existing arbitration window
func (as *AudioService) joinArbitrationWindow(relayID string, stream pb.AudioService_StreamAudioServer) bool {
	as.streamsMutex.Lock()
	defer as.streamsMutex.Unlock()

	window := as.arbitrationWindow
	if window == nil {
		return false
	}

	// Lock window for atomic access to its state
	window.mutex.Lock()
	defer window.mutex.Unlock()

	if !window.IsActive {
		return false
	}

	// Check if window is still open
	elapsed := time.Since(window.StartTime)
	if elapsed > window.WindowDuration {
		return false
	}

	// Add relay to window
	relayStream := &RelayStream{
		Stream:        stream,
		RelayID:       relayID,
		ConnectedAt:   time.Now(),
		Status:        RelayStatusContending,
		CancelChannel: make(chan struct{}),
	}

	window.Relays[relayID] = relayStream
	as.activeStreams[relayID] = relayStream

	logging.LogAudioProcessing(relayID, "arbitration_window_joined",
		zap.Duration("elapsed", elapsed),
		zap.Duration("remaining", window.WindowDuration-elapsed),
		zap.Int("relay_count", len(window.Relays)),
	)

	return true
}

// runArbitrationTimer manages the arbitration window lifecycle
func (as *AudioService) runArbitrationTimer(window *ArbitrationWindow) {
	timer := time.NewTimer(window.WindowDuration)
	defer timer.Stop()

	<-timer.C

	// Window closed, perform arbitration
	as.performArbitration(window)
}

// performArbitration selects the winning relay based on signal strength
func (as *AudioService) performArbitration(window *ArbitrationWindow) {
	window.mutex.Lock()
	defer window.mutex.Unlock()

	if !window.IsActive {
		return // Already arbitrated
	}

	logging.LogAudioProcessing("arbitration", "arbitration_starting",
		zap.Int("relay_count", len(window.Relays)),
	)

	// Find relay with strongest wake word signal
	var winnerID string
	var maxSignalStrength float64

	for relayID, relay := range window.Relays {
		if relay.Status != RelayStatusContending {
			continue
		}

		// Calculate signal strength from wake word audio
		signalStrength := as.calculateSignalStrength(relay.WakeWordSignal)
		relay.SignalStrength = signalStrength

		logging.LogAudioProcessing(relayID, "arbitration_signal_analysis",
			zap.Float64("signal_strength", signalStrength),
			zap.Int("samples", len(relay.WakeWordSignal)),
		)

		if signalStrength > maxSignalStrength {
			maxSignalStrength = signalStrength
			winnerID = relayID
		}
	}

	// Fallback to first relay if no clear winner
	if winnerID == "" && len(window.Relays) > 0 {
		for relayID := range window.Relays {
			winnerID = relayID
			break
		}
	}

	// Mark winner and cancel losers
	window.WinnerID = winnerID
	window.IsActive = false

	for relayID, relay := range window.Relays {
		if relayID == winnerID {
			relay.Status = RelayStatusWinner
			logging.LogAudioProcessing(relayID, "arbitration_winner",
				zap.Float64("signal_strength", relay.SignalStrength),
				zap.Int("competing_relays", len(window.Relays)),
			)
		} else {
			relay.Status = RelayStatusCancelled
			close(relay.CancelChannel)
			logging.LogAudioProcessing(relayID, "arbitration_cancelled",
				zap.Float64("signal_strength", relay.SignalStrength),
				zap.String("winner", winnerID),
			)

			// Send cancellation response to losing relay
			go as.sendCancellationResponse(relay)
		}
	}

	// Process the winning relay's audio
	if winnerID != "" {
		if winnerRelay, exists := window.Relays[winnerID]; exists {
			logging.LogAudioProcessing(winnerID, "arbitration_winner_processing_started")

			// Process the winner's full audio (wake word + speech)
			go as.processWinningRelayAudio(winnerRelay)
		}
	}

	// Clear arbitration window
	as.streamsMutex.Lock()
	as.arbitrationWindow = nil
	as.streamsMutex.Unlock()
}

// processWinningRelayAudio handles transcription and response generation for the arbitration winner
func (as *AudioService) processWinningRelayAudio(relay *RelayStream) {
	// Combine wake word signal and speech audio for full transcription
	var fullAudio []float32
	fullAudio = append(fullAudio, relay.WakeWordSignal...)
	fullAudio = append(fullAudio, relay.SpeechAudio...)

	if len(fullAudio) == 0 {
		logging.LogWarn("processWinningRelayAudio: no audio data to process",
			zap.String("relay_id", relay.RelayID))
		return
	}

	logging.LogAudioProcessing(relay.RelayID, "winner_transcription_starting",
		zap.Int("total_samples", len(fullAudio)),
		zap.Int("wake_word_samples", len(relay.WakeWordSignal)),
		zap.Int("speech_samples", len(relay.SpeechAudio)),
	)

	// Transcribe the full audio
	result, err := as.transcriber.TranscribeWithConfidence(fullAudio, 16000)
	if err != nil {
		logging.LogWarn("Transcription failed for winning relay",
			zap.String("relay_id", relay.RelayID),
			zap.Error(err))

		// Send error response
		as.sendErrorResponse(relay, "Sorry, I couldn't hear you clearly. Please try again.")
		return
	}

	if result.Text == "" {
		logging.LogAudioProcessing(relay.RelayID, "winner_no_speech_detected")
		as.sendErrorResponse(relay, "I didn't hear anything. Please try again.")
		return
	}

	logging.LogAudioProcessing(relay.RelayID, "winner_transcription_completed",
		zap.String("transcription", result.Text),
		zap.Float64("confidence", result.ConfidenceEstimate),
	)

	// Process the command using StreamingPredictiveBridge or fallback
	as.processTranscriptionForRelay(relay, result.Text)
}

// processTranscriptionForRelay handles command processing and response generation
func (as *AudioService) processTranscriptionForRelay(relay *RelayStream, transcription string) {
	var bridgeSession *llm.BridgeSession
	var multiCmd *llm.MultiCommand
	var err error

	// Try StreamingPredictiveBridge first for fast response
	if as.streamingPredictiveBridge != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		bridgeSession, err = as.streamingPredictiveBridge.ProcessVoiceCommand(ctx, transcription)
		if err == nil && bridgeSession != nil && bridgeSession.PredictiveResponse != nil {
			logging.LogAudioProcessing(relay.RelayID, "winner_streaming_bridge_success",
				zap.String("transcription", transcription),
				zap.String("immediate_ack", bridgeSession.PredictiveResponse.ImmediateAck),
			)

			// Send immediate response
			as.sendSuccessResponse(relay, transcription, bridgeSession.PredictiveResponse.ImmediateAck)
			return
		}

		logging.LogAudioProcessing(relay.RelayID, "winner_streaming_bridge_fallback",
			zap.Error(err),
		)
	}

	// Fallback: Parse command using traditional LLM multi-command support
	multiCmd, err = as.commandParser.ParseMultiCommand(transcription)
	if err != nil {
		logging.LogWarn("Command parsing failed for winning relay",
			zap.String("relay_id", relay.RelayID),
			zap.String("transcription", transcription),
			zap.Error(err))

		as.sendErrorResponse(relay, "Sorry, I couldn't understand that command.")
		return
	}

	if len(multiCmd.Commands) == 0 {
		logging.LogAudioProcessing(relay.RelayID, "winner_no_commands_found",
			zap.String("transcription", transcription))
		as.sendErrorResponse(relay, "I'm not sure how to help with that.")
		return
	}

	// Execute commands and send response
	logging.LogAudioProcessing(relay.RelayID, "winner_command_execution_starting",
		zap.Int("command_count", len(multiCmd.Commands)),
	)

	// Use the first command's response for simplicity
	command := multiCmd.Commands[0]
	as.sendSuccessResponse(relay, transcription, command.Response)
}

// sendSuccessResponse sends a successful voice response to the relay
func (as *AudioService) sendSuccessResponse(relay *RelayStream, transcription, responseText string) {
	response := &pb.AudioResponse{
		Success:       true,
		Transcription: transcription,
		ResponseText:  responseText,
		Command:       "voice_command_success",
	}

	// Generate TTS audio if available
	if as.ttsClient != nil {
		ttsOptions := &llm.TTSOptions{
			Voice:          "af_bella",
			Speed:          1.0,
			ResponseFormat: "mp3",
		}
		ttsResult, err := as.ttsClient.Synthesize(responseText, ttsOptions)
		if err != nil {
			log.Printf("❌ TTS synthesis failed: %v", err)
		} else if ttsResult == nil {
			log.Printf("❌ TTS result is nil")
		} else if ttsResult.Audio == nil {
			log.Printf("❌ TTS result.Audio is nil")
		} else {
			// Read audio data immediately to avoid context cancellation
			audioBytes, err := io.ReadAll(ttsResult.Audio)
			// Close immediately after reading
			if closer, ok := ttsResult.Audio.(io.Closer); ok {
				_ = closer.Close()
			}
			// Clean up TTS resources (including context cancellation)
			if ttsResult.Cleanup != nil {
				ttsResult.Cleanup()
			}

			if err != nil {
				log.Printf("❌ Failed to read TTS audio data: %v", err)
			} else if len(audioBytes) == 0 {
				log.Printf("❌ TTS audio data is empty")
			} else {
				response.ResponseAudio = audioBytes
				response.AudioFormat = ttsOptions.ResponseFormat
				response.AudioDuration = float32(ttsResult.Length) / 16000.0 // Approximate duration
				log.Printf("✅ TTS audio ready: %d bytes, format: %s", len(audioBytes), ttsOptions.ResponseFormat)
			}
		}
	}

	// Stream success response via NATS instead of gRPC
	if len(response.ResponseAudio) > 0 {
		if err := as.streamAudioResponse(
			relay.RelayID,
			response.ResponseAudio,
			response.AudioFormat,
			"response",
			3, // medium priority
		); err != nil {
			logging.LogWarn("Failed to stream success audio to relay",
				zap.String("relay_id", relay.RelayID),
				zap.Error(err))
		}
	} else {
		log.Printf("🎵 No TTS audio for success response to relay %s", relay.RelayID)
	}

	logging.LogAudioProcessing(relay.RelayID, "winner_response_sent",
		zap.String("response_text", responseText),
		zap.Bool("has_audio", len(response.ResponseAudio) > 0),
	)
}

// sendErrorResponse sends an error response to the relay
func (as *AudioService) sendErrorResponse(relay *RelayStream, errorMessage string) {
	response := &pb.AudioResponse{
		Success:      false,
		ResponseText: errorMessage,
		Command:      "error",
	}

	// Generate TTS audio for error message
	if as.ttsClient != nil {
		ttsOptions := &llm.TTSOptions{
			Voice:          "af_bella",
			Speed:          1.0,
			ResponseFormat: "mp3",
		}
		ttsResult, err := as.ttsClient.Synthesize(errorMessage, ttsOptions)
		if err == nil && ttsResult != nil && ttsResult.Audio != nil {
			// Read audio data immediately to avoid context cancellation
			audioBytes, readErr := io.ReadAll(ttsResult.Audio)
			// Close immediately after reading
			if closer, ok := ttsResult.Audio.(io.Closer); ok {
				_ = closer.Close()
			}
			// Clean up TTS resources (including context cancellation)
			if ttsResult.Cleanup != nil {
				ttsResult.Cleanup()
			}

			if readErr == nil && len(audioBytes) > 0 {
				response.ResponseAudio = audioBytes
				response.AudioFormat = ttsOptions.ResponseFormat
				response.AudioDuration = float32(len(audioBytes)) / 16000.0 // Approximate
			}
		}
	}

	// Stream error response via NATS instead of gRPC
	if len(response.ResponseAudio) > 0 {
		if err := as.streamAudioResponse(
			relay.RelayID,
			response.ResponseAudio,
			response.AudioFormat,
			"error",
			4, // lower priority for errors
		); err != nil {
			logging.LogWarn("Failed to stream error audio to relay",
				zap.String("relay_id", relay.RelayID),
				zap.Error(err))
		}
	} else {
		log.Printf("🎵 No TTS audio for error response to relay %s", relay.RelayID)
	}
}

// calculateSignalStrength computes RMS signal strength from audio samples
func (as *AudioService) calculateSignalStrength(samples []float32) float64 {
	if len(samples) == 0 {
		logging.LogWarn("calculateSignalStrength: no samples provided")
		return 0.0
	}

	var sum float64
	var nonZeroSamples int
	var maxSample float32
	for _, sample := range samples {
		sum += float64(sample * sample)
		if sample != 0 {
			nonZeroSamples++
		}
		absVal := sample
		if absVal < 0 {
			absVal = -absVal
		}
		if absVal > maxSample {
			maxSample = absVal
		}
	}

	rms := math.Sqrt(sum / float64(len(samples)))

	logging.LogAudioProcessing("", "signal_strength_calculation",
		zap.Int("total_samples", len(samples)),
		zap.Int("non_zero_samples", nonZeroSamples),
		zap.Float32("max_sample", maxSample),
		zap.Float64("rms", rms),
		zap.Float64("sum", sum),
	)

	return rms
}

// sendCancellationResponse sends a cancellation message to a losing relay
func (as *AudioService) sendCancellationResponse(relay *RelayStream) {
	// Check if stream is available (nil in tests)
	if relay.Stream == nil {
		logging.LogAudioProcessing(relay.RelayID, "cancellation_skipped",
			zap.String("reason", "nil_stream_in_test"),
		)
		return
	}

	response := &pb.AudioResponse{
		RequestId:     relay.RelayID,
		Transcription: "",
		Command:       "relay_cancelled",
		ResponseText:  "Another relay is handling this request.",
		Success:       false,
	}

	if err := relay.Stream.Send(response); err != nil {
		logging.LogError(err, "Failed to send cancellation response",
			zap.String("relay_id", relay.RelayID),
		)
	}
}

// isRelayActive checks if a relay should continue processing
func (as *AudioService) isRelayActive(relayID string) bool {
	as.streamsMutex.RLock()
	defer as.streamsMutex.RUnlock()

	relay, exists := as.activeStreams[relayID]
	if !exists {
		logging.LogAudioProcessing(relayID, "relay_not_found_in_active_streams")
		return false
	}

	isActive := relay.Status == RelayStatusWinner ||
		relay.Status == RelayStatusConnected ||
		relay.Status == RelayStatusContending

	// Debug logging to understand status
	statusName := "unknown"
	switch relay.Status {
	case RelayStatusConnected:
		statusName = "connected"
	case RelayStatusContending:
		statusName = "contending"
	case RelayStatusWinner:
		statusName = "winner"
	case RelayStatusCancelled:
		statusName = "cancelled"
	}

	logging.LogAudioProcessing(relayID, "relay_status_check",
		zap.String("status", statusName),
		zap.Bool("is_active", isActive),
	)

	return isActive
}

// cleanupRelay removes a relay from active tracking
func (as *AudioService) cleanupRelay(relayID string) {
	as.streamsMutex.Lock()
	defer as.streamsMutex.Unlock()

	delete(as.activeStreams, relayID)

	logging.LogAudioProcessing(relayID, "relay_cleanup_completed")
}

// StreamAudio handles bidirectional audio streaming from relay devices with collision detection
func (as *AudioService) StreamAudio(stream pb.AudioService_StreamAudioServer) error {
	var relayID string
	var wakeWordBuffer []float32

	logging.Sugar.Info("🎙️  Hub: New audio stream connected")

	// Cleanup on exit
	defer func() {
		if relayID != "" {
			as.cleanupRelay(relayID)
		}
	}()

	for {
		// Receive audio chunk from relay
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			logging.Sugar.Info("🎙️  Hub: Audio stream ended", zap.String("relay_id", relayID))
			return nil
		}
		if err != nil {
			logging.LogError(err, "Error receiving audio chunk", zap.String("relay_id", relayID))
			return err
		}

		// Set relay ID from first chunk
		if relayID == "" {
			relayID = chunk.RelayId
		}

		logging.LogAudioProcessing(chunk.RelayId, "received",
			zap.Int("bytes", len(chunk.AudioData)),
			zap.Bool("wake_word", chunk.IsWakeWord),
		)

		// Handle wake word detection for collision arbitration
		if chunk.IsWakeWord {
			// Convert audio data for signal analysis
			audioData := bytesToFloat32Array(chunk.AudioData)
			wakeWordBuffer = append(wakeWordBuffer, audioData...)

			// Check if arbitration window exists
			as.streamsMutex.RLock()
			window := as.arbitrationWindow
			as.streamsMutex.RUnlock()

			if window == nil {
				// Start new arbitration window
				as.startArbitrationWindow(relayID, stream)
				// Populate the wake word signal for the initial relay
				as.streamsMutex.Lock()
				if relay, exists := as.activeStreams[relayID]; exists {
					relay.WakeWordSignal = wakeWordBuffer
				}
				as.streamsMutex.Unlock()
			} else {
				// Try to join existing window
				if as.joinArbitrationWindow(relayID, stream) {
					// Successfully joined, update wake word signal
					as.streamsMutex.Lock()
					if relay, exists := as.activeStreams[relayID]; exists {
						relay.WakeWordSignal = wakeWordBuffer
					}
					as.streamsMutex.Unlock()
				} else {
					// Window closed, send cancellation
					// Relay too late - no need to send response in fire-and-forget model
					log.Printf("⏰ Relay %s attempted connection after arbitration window closed", relayID)
					return nil
				}
			}
		} else {
			// Collect speech audio (non-wake-word chunks) for arbitration winner processing
			audioData := bytesToFloat32Array(chunk.AudioData)
			as.streamsMutex.Lock()
			if relay, exists := as.activeStreams[relayID]; exists {
				relay.SpeechAudio = append(relay.SpeechAudio, audioData...)
			}
			as.streamsMutex.Unlock()
		}

		// Handle end of speech processing
		if chunk.IsEndOfSpeech {
			// Note: Audio processing is now handled by the arbitration system
			// The winning relay will be processed automatically via processWinningRelayAudio()
			// This preserves the connection for response delivery but doesn't duplicate processing

			logging.LogAudioProcessing(relayID, "end_of_speech_detected",
				zap.String("processing_note", "arbitration_system_handles_winner"),
			)

			// Check if this relay is still active (not cancelled during arbitration)
			if !as.isRelayActive(relayID) {
				logging.LogAudioProcessing(relayID, "relay_cancelled_before_processing")
				return nil
			}

			// Wait for arbitration-based processing to complete and send response
			// The winning relay will be processed via processWinningRelayAudio()
			// Don't exit - keep connection alive to receive response

			logging.LogAudioProcessing(relayID, "waiting_for_arbitration_response")

			// Wait longer for arbitration system to complete and send response
			// The processWinningRelayAudio will send response via relay.Stream.Send()
			for i := 0; i < 50; i++ { // Wait up to 5 seconds for response
				time.Sleep(100 * time.Millisecond)
				// Check if arbitration is complete and relay still active
				if as.arbitrationWindow == nil {
					break // Arbitration completed
				}
			}

			logging.LogAudioProcessing(relayID, "arbitration_wait_completed")
			return nil // Exit after arbitration processing is done
		}
	}
}

// Helper functions for audio analysis

// Helper function to convert bytes back to float32 array
func bytesToFloat32Array(data []byte) []float32 {
	// Validate input data
	if len(data) == 0 {
		logging.LogWarn("bytesToFloat32Array: empty input data")
		return []float32{}
	}

	// Ensure even number of bytes for 16-bit PCM
	dataLen := len(data)
	if dataLen%2 != 0 {
		logging.LogWarn("bytesToFloat32Array: odd number of bytes, dropping last byte",
			zap.Int("original_length", len(data)),
			zap.Int("adjusted_length", dataLen-1),
		)
		dataLen -= 1 // Drop the last incomplete sample
	}

	// Convert 16-bit PCM bytes to float32 samples
	samples := make([]float32, dataLen/2)
	var nonZeroSamples int
	var maxAbsValue float32

	for i := 0; i < len(samples); i++ {
		// Add bounds checking
		if i*2+1 >= len(data) {
			break
		}
		// Reconstruct int16 from bytes (little-endian)
		val := int16(data[i*2]) | int16(data[i*2+1])<<8
		// Convert to float32 [-1,1]
		samples[i] = float32(val) / 32767.0

		// Track statistics for debugging
		if samples[i] != 0 {
			nonZeroSamples++
		}
		absVal := samples[i]
		if absVal < 0 {
			absVal = -absVal
		}
		if absVal > maxAbsValue {
			maxAbsValue = absVal
		}
	}

	logging.LogAudioProcessing("", "bytes_to_float32_conversion",
		zap.Int("input_bytes", len(data)),
		zap.Int("output_samples", len(samples)),
		zap.Int("non_zero_samples", nonZeroSamples),
		zap.Float32("max_abs_value", maxAbsValue),
	)

	return samples
}

// isDeviceCommand checks if an intent represents a device command
func (as *AudioService) isDeviceCommand(intent string) bool {
	deviceIntents := map[string]bool{
		"turn_on":  true,
		"turn_off": true,
		"dim":      true,
		"brighten": true,
		"play":     true,
		"stop":     true,
		"pause":    true,
		"volume":   true,
	}
	return deviceIntents[intent]
}

// createDeviceCommand creates a device command from a voice command
func (as *AudioService) createDeviceCommand(commandEvent *messaging.CommandEvent) *messaging.DeviceCommandEvent {
	deviceType := as.extractDeviceType(commandEvent.Entities)
	if deviceType == "" {
		// Default to lights if no specific device mentioned
		deviceType = "lights"
	}

	action := as.mapIntentToAction(commandEvent.Intent)
	if action == "" {
		return nil
	}

	return &messaging.DeviceCommandEvent{
		CommandEvent: *commandEvent,
		DeviceType:   deviceType,
		DeviceID:     commandEvent.Entities["device_id"],
		Location:     commandEvent.Entities["location"],
		Action:       action,
	}
}

// extractDeviceType extracts device type from entities
func (as *AudioService) extractDeviceType(entities map[string]string) string {
	if device, exists := entities["device"]; exists {
		// Map common device names to types
		deviceMap := map[string]string{
			"lights":     "lights",
			"light":      "lights",
			"lamp":       "lights",
			"music":      "audio",
			"audio":      "audio",
			"sound":      "audio",
			"tv":         "tv",
			"television": "tv",
		}
		if deviceType, found := deviceMap[device]; found {
			return deviceType
		}
		return device
	}
	return ""
}

// mapIntentToAction maps voice intents to device actions
func (as *AudioService) mapIntentToAction(intent string) string {
	actionMap := map[string]string{
		"turn_on":  "on",
		"turn_off": "off",
		"dim":      "dim",
		"brighten": "brighten",
		"play":     "play",
		"stop":     "stop",
		"pause":    "pause",
		"volume":   "volume",
	}
	return actionMap[intent]
}
