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
	"github.com/loqalabs/loqa-hub/internal/events"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/messaging"
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
	transcriber             llm.Transcriber
	commandParser           *llm.CommandParser
	ttsClient               llm.TextToSpeech
	natsService             *messaging.NATSService
	eventsStore             *storage.VoiceEventsStore
	currentExecutionContext *CommandExecutionContext

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
func NewAudioServiceWithSTT(sttURL string, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {
	transcriber, err := llm.NewSTTClient(sttURL)
	if err != nil {
		return nil, err
	}
	return createAudioService(transcriber, nil, eventsStore)
}

// NewAudioServiceWithTTS creates a new audio service with both STT and TTS support
func NewAudioServiceWithTTS(sttURL string, ttsConfig config.TTSConfig, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {
	// Initialize STT client
	transcriber, err := llm.NewSTTClient(sttURL)
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

	// Initialize NATS service
	natsService, err := messaging.NewNATSService()
	if err != nil {
		log.Printf("⚠️  Warning: Failed to create NATS service: %v", err)
	}

	// Connect to NATS (non-blocking)
	go func() {
		if natsService != nil {
			if err := natsService.Connect(); err != nil {
				log.Printf("⚠️  Warning: Cannot connect to NATS: %v", err)
				log.Println("🔄 Events will not be published to message bus")
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

	return &AudioService{
		transcriber:               transcriber,
		commandParser:             commandParser,
		ttsClient:                 ttsClient,
		natsService:               natsService,
		eventsStore:               eventsStore,
		activeStreams:             make(map[string]*RelayStream),
		arbitrationWindowDuration: 300 * time.Millisecond, // Configurable arbitration window
	}, nil
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

	// Clear arbitration window
	as.streamsMutex.Lock()
	as.arbitrationWindow = nil
	as.streamsMutex.Unlock()
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
		return false
	}

	return relay.Status == RelayStatusWinner || relay.Status == RelayStatusConnected
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
					response := &pb.AudioResponse{
						RequestId:     relayID,
						Transcription: "",
						Command:       "relay_too_late",
						ResponseText:  "Arbitration window closed. Please try again.",
						Success:       false,
					}
					if err := stream.Send(response); err != nil {
						logging.LogError(err, "Error sending too-late response")
					}
					return nil
				}
			}
		}

		// Handle end of speech processing
		if chunk.IsEndOfSpeech {
			// Check if this relay is still active (not cancelled during arbitration)
			if !as.isRelayActive(relayID) {
				logging.LogAudioProcessing(relayID, "relay_cancelled_before_processing")
				return nil
			}

			// Wait for arbitration to complete if still in progress
			maxWait := time.Now().Add(500 * time.Millisecond) // Max wait for arbitration
			for as.arbitrationWindow != nil && time.Now().Before(maxWait) {
				time.Sleep(10 * time.Millisecond)
			}

			// Final check if relay won arbitration
			if !as.isRelayActive(relayID) {
				logging.LogAudioProcessing(relayID, "relay_lost_arbitration")
				return nil
			}
			logging.LogAudioProcessing(chunk.RelayId, "processing_utterance")

			// Create voice event for tracking
			voiceEvent := events.NewVoiceEvent(chunk.RelayId, chunk.RelayId)

			// Convert audio bytes back to float32
			audioData := bytesToFloat32Array(chunk.AudioData)

			// Validate converted audio data
			if len(audioData) == 0 {
				logging.LogWarn("Empty audio data after conversion",
					zap.String("relay_id", chunk.RelayId),
					zap.Int("original_bytes", len(chunk.AudioData)),
				)
				voiceEvent.SetResponse("Invalid audio data received")
				as.storeVoiceEvent(voiceEvent)

				// Send error response back to relay
				response := &pb.AudioResponse{
					RequestId:     chunk.RelayId,
					Transcription: "",
					Command:       "error",
					ResponseText:  "Invalid audio data received. Please try again.",
					Success:       false,
				}
				if err := stream.Send(response); err != nil {
					logging.LogError(err, "Error sending empty-audio response to relay")
					return err
				}
				continue
			}

			// Set audio metadata (safe conversion from int32 to int)
			sampleRate := int(chunk.SampleRate)
			voiceEvent.SetAudioMetadata(audioData, sampleRate, chunk.IsWakeWord)

			// Transcribe audio using STT service with confidence (with panic recovery and detailed logging)
			var transcription string
			var transcriptionResult *llm.TranscriptionResult
			var err error

			// Log audio data characteristics before STT call
			logging.LogAudioProcessing(chunk.RelayId, "stt_pre_call",
				zap.Int("samples_count", len(audioData)),
				zap.Int("sample_rate", sampleRate), // Use the validated sample rate
				zap.Float32("audio_min", findMin(audioData)),
				zap.Float32("audio_max", findMax(audioData)),
				zap.String("event_uuid", voiceEvent.UUID),
			)

			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("STT transcription panic: %v", r)
						logging.LogError(err, "STT panic recovered",
							zap.String("relay_id", chunk.RelayId),
							zap.String("event_uuid", voiceEvent.UUID),
							zap.Int("samples_count", len(audioData)),
						)
					}
				}()

				logging.LogAudioProcessing(chunk.RelayId, "stt_calling",
					zap.String("event_uuid", voiceEvent.UUID),
				)

				transcriptionResult, err = as.transcriber.TranscribeWithConfidence(audioData, sampleRate)
				if err == nil && transcriptionResult != nil {
					transcription = transcriptionResult.Text
				}

				logging.LogAudioProcessing(chunk.RelayId, "stt_returned",
					zap.String("event_uuid", voiceEvent.UUID),
					zap.Bool("success", err == nil),
					zap.Int("transcription_length", len(transcription)),
					zap.Float64("confidence_estimate", func() float64 {
						if transcriptionResult != nil {
							return transcriptionResult.ConfidenceEstimate
						}
						return 0.0
					}()),
					zap.Bool("wake_word_detected", func() bool {
						if transcriptionResult != nil {
							return transcriptionResult.WakeWordDetected
						}
						return false
					}()),
					zap.Bool("needs_confirmation", func() bool {
						if transcriptionResult != nil {
							return transcriptionResult.NeedsConfirmation
						}
						return false
					}()),
				)
			}()

			if err != nil {
				logging.LogError(err, "Error transcribing audio",
					zap.String("relay_id", chunk.RelayId),
					zap.String("event_uuid", voiceEvent.UUID),
				)
				voiceEvent.SetError(err)
				voiceEvent.SetResponse("Sorry, I couldn't process your audio. Please try again.")
				as.storeVoiceEvent(voiceEvent)

				// Send error response back to relay
				response := &pb.AudioResponse{
					RequestId:     chunk.RelayId,
					Transcription: "",
					Command:       "error",
					ResponseText:  "Sorry, I couldn't process your audio. Please try again.",
					Success:       false,
				}
				if err := stream.Send(response); err != nil {
					logging.LogError(err, "Error sending error response to relay")
					return err
				}
				continue
			}

			// Set transcription result (with original transcription for logging)
			if transcriptionResult != nil {
				voiceEvent.SetTranscription(transcriptionResult.Text)
			} else {
				voiceEvent.SetTranscription(transcription)
			}

			logging.LogAudioProcessing(chunk.RelayId, "transcribed",
				zap.String("event_uuid", voiceEvent.UUID),
				zap.Int("audio_samples", len(audioData)),
				zap.String("transcription", transcription),
				zap.Bool("wake_word", chunk.IsWakeWord),
				zap.Float64("confidence_estimate", func() float64 {
					if transcriptionResult != nil {
						return transcriptionResult.ConfidenceEstimate
					}
					return 0.0
				}()),
				zap.Bool("wake_word_detected", func() bool {
					if transcriptionResult != nil {
						return transcriptionResult.WakeWordDetected
					}
					return false
				}()),
				zap.String("wake_word_variant", func() string {
					if transcriptionResult != nil {
						return transcriptionResult.WakeWordVariant
					}
					return ""
				}()),
			)

			// Handle low confidence or empty transcriptions
			if transcription == "" {
				logging.LogAudioProcessing(chunk.RelayId, "no_speech_detected",
					zap.String("event_uuid", voiceEvent.UUID),
				)
				voiceEvent.SetResponse("No speech detected")
				as.storeVoiceEvent(voiceEvent)

				// Send response back to relay for no speech detected
				response := &pb.AudioResponse{
					RequestId:     chunk.RelayId,
					Transcription: "",
					Command:       "no_speech",
					ResponseText:  "I didn't hear anything. Please try again.",
					Success:       true,
				}
				if err := stream.Send(response); err != nil {
					logging.LogError(err, "Error sending no-speech response to relay")
					return err
				}
				continue
			}

			// Handle low confidence transcriptions that need confirmation
			if transcriptionResult != nil && transcriptionResult.NeedsConfirmation {
				logging.LogAudioProcessing(chunk.RelayId, "low_confidence_detected",
					zap.String("event_uuid", voiceEvent.UUID),
					zap.Float64("confidence", transcriptionResult.ConfidenceEstimate),
					zap.String("transcription", transcription),
				)

				confirmationMessage := fmt.Sprintf("I'm not sure I heard you correctly. Did you say '%s'? Please repeat if that's not right.", transcription)
				voiceEvent.SetResponse(confirmationMessage)
				as.storeVoiceEvent(voiceEvent)

				// Send confirmation request back to relay
				response := &pb.AudioResponse{
					RequestId:     chunk.RelayId,
					Transcription: transcription,
					Command:       "confirmation_needed",
					ResponseText:  confirmationMessage,
					Success:       true,
				}
				if err := stream.Send(response); err != nil {
					logging.LogError(err, "Error sending confirmation response to relay")
					return err
				}
				continue
			}

			// Parse command using LLM (with multi-command support)
			multiCmd, err := as.commandParser.ParseMultiCommand(transcription)
			if err != nil {
				logging.LogError(err, "Error parsing multi-command",
					zap.String("relay_id", chunk.RelayId),
					zap.String("event_uuid", voiceEvent.UUID),
					zap.String("transcription", transcription),
				)
				// Fallback to single unknown command
				multiCmd = &llm.MultiCommand{
					Commands: []llm.Command{{
						Intent:     "unknown",
						Entities:   make(map[string]string),
						Confidence: 0.0,
						Response:   "I'm having trouble understanding you right now.",
					}},
					IsMulti:          false,
					OriginalText:     transcription,
					CombinedResponse: "I'm having trouble understanding you right now.",
				}
			}

			var commandStr string
			var responseText string
			var primaryCommand *llm.Command

			// Handle multi-command or single command
			if multiCmd.IsMulti && len(multiCmd.Commands) > 1 {
				// Multi-command processing
				logging.LogAudioProcessing(chunk.RelayId, "multi_command_detected",
					zap.String("event_uuid", voiceEvent.UUID),
					zap.Int("command_count", len(multiCmd.Commands)),
					zap.String("original_text", multiCmd.OriginalText),
				)

				// Set execution context for command execution
				as.SetCommandExecutionContext(&CommandExecutionContext{
					RelayID:       chunk.RelayId,
					RequestID:     chunk.RelayId,
					EventUUID:     voiceEvent.UUID,
					Transcription: transcription,
				})

				// Execute commands sequentially using command queue
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				queue := llm.NewCommandQueue(multiCmd.Commands, 5*time.Second, true)
				execResult, execErr := queue.Execute(ctx, as)

				if execErr != nil {
					logging.LogError(execErr, "Multi-command execution failed",
						zap.String("relay_id", chunk.RelayId),
						zap.String("event_uuid", voiceEvent.UUID),
					)
					commandStr = "multi_command_failed"
					responseText = "I had trouble executing some of your commands."
				} else {
					commandStr = fmt.Sprintf("multi_%s", multiCmd.Commands[0].Intent)
					responseText = execResult.CombinedResponse
					if execResult.RollbackOccurred {
						responseText += " Some commands were rolled back due to failures."
					}

					logging.LogAudioProcessing(chunk.RelayId, "multi_command_completed",
						zap.String("event_uuid", voiceEvent.UUID),
						zap.Bool("success", execResult.Success),
						zap.Duration("duration", execResult.TotalDuration),
						zap.Int("completed_commands", len(execResult.CompletedItems)),
						zap.Bool("rollback_occurred", execResult.RollbackOccurred),
					)
				}

				// Use first command for voice event tracking
				primaryCommand = &multiCmd.Commands[0]
			} else {
				// Single command processing (existing logic)
				if len(multiCmd.Commands) > 0 {
					primaryCommand = &multiCmd.Commands[0]
				} else {
					// Create a default unknown command
					defaultCmd := llm.Command{
						Intent:     "unknown",
						Entities:   make(map[string]string),
						Confidence: 0.0,
						Response:   "I'm not sure what you want me to do.",
					}
					primaryCommand = &defaultCmd
				}
				commandStr = primaryCommand.Intent
				responseText = primaryCommand.Response

				logging.LogAudioProcessing(chunk.RelayId, "single_command_parsed",
					zap.String("event_uuid", voiceEvent.UUID),
					zap.String("intent", primaryCommand.Intent),
					zap.Float64("confidence", primaryCommand.Confidence),
					zap.Any("entities", primaryCommand.Entities),
				)
			}

			// Set command parsing results for voice event
			voiceEvent.SetCommandResult(primaryCommand.Intent, primaryCommand.Entities, primaryCommand.Confidence)

			// Publish command event to NATS (only for single commands or primary command in multi-command)
			if as.natsService != nil && as.natsService.IsConnected() {
				// For multi-commands, individual commands are published during execution
				// Here we publish the primary/summary command event
				commandEvent := &messaging.CommandEvent{
					RelayID:       chunk.RelayId,
					Transcription: transcription,
					Intent:        primaryCommand.Intent,
					Entities:      primaryCommand.Entities,
					Confidence:    primaryCommand.Confidence,
					Timestamp:     time.Now().UnixNano(),
					RequestID:     chunk.RelayId, // Using relay ID as request ID for now
				}

				if err := as.natsService.PublishVoiceCommand(commandEvent); err != nil {
					logging.LogWarn("Failed to publish voice command to NATS",
						zap.Error(err),
						zap.String("relay_id", chunk.RelayId),
						zap.String("event_uuid", voiceEvent.UUID),
					)
				} else {
					logging.LogNATSEvent("loqa.voice.commands", "published",
						zap.String("relay_id", chunk.RelayId),
						zap.String("intent", primaryCommand.Intent),
					)
				}

				// If this is a device command, also publish a device command event
				if as.isDeviceCommand(primaryCommand.Intent) {
					deviceCommand := as.createDeviceCommand(commandEvent)
					if deviceCommand != nil {
						if err := as.natsService.PublishDeviceCommand(deviceCommand); err != nil {
							logging.LogWarn("Failed to publish device command to NATS",
								zap.Error(err),
								zap.String("device_type", deviceCommand.DeviceType),
							)
						} else {
							logging.LogNATSEvent("loqa.devices.commands", "published",
								zap.String("device_type", deviceCommand.DeviceType),
								zap.String("action", deviceCommand.Action),
							)
						}
					}
				}
			}

			// Generate TTS audio if TTS client is available and responseText is not empty
			var ttsAudioData []byte
			var ttsAudioFormat string
			var ttsAudioDuration float32

			if as.ttsClient != nil && responseText != "" {
				ttsOptions := &llm.TTSOptions{
					ResponseFormat: "mp3", // Default format, could be configurable
					Speed:          1.0,   // Default speed, could be configurable
					Normalize:      true,  // Default normalization
				}

				ttsResult, err := as.ttsClient.Synthesize(responseText, ttsOptions)
				if err != nil {
					logging.LogWarn("TTS synthesis failed, sending text-only response",
						zap.Error(err),
						zap.String("relay_id", chunk.RelayId),
						zap.String("event_uuid", voiceEvent.UUID),
						zap.String("response_text", responseText),
					)
				} else {
					// Read audio data from the result
					audioBytes, readErr := io.ReadAll(ttsResult.Audio)
					if readErr != nil {
						logging.LogWarn("Failed to read TTS audio data",
							zap.Error(readErr),
							zap.String("relay_id", chunk.RelayId),
							zap.String("event_uuid", voiceEvent.UUID),
						)
					} else {
						ttsAudioData = audioBytes
						ttsAudioFormat = ttsOptions.ResponseFormat
						if ttsResult.Length > 0 {
							// Estimate duration based on typical bitrates
							// For MP3 at 128kbps: ~16KB per second
							ttsAudioDuration = float32(len(audioBytes)) / (16 * 1024)
						}

						logging.LogTTSOperation("synthesis_success",
							zap.String("relay_id", chunk.RelayId),
							zap.String("event_uuid", voiceEvent.UUID),
							zap.Int("audio_bytes", len(audioBytes)),
							zap.String("format", ttsAudioFormat),
							zap.Float32("duration", ttsAudioDuration),
						)
					}

					// Close the audio stream
					if closer, ok := ttsResult.Audio.(io.Closer); ok {
						if err := closer.Close(); err != nil {
							logging.LogError(err, "Failed to close TTS audio stream")
						}
					}
				}
			}

			// Send response back to relay
			response := &pb.AudioResponse{
				RequestId:     chunk.RelayId, // Use relay ID as request ID
				Transcription: transcription,
				Command:       commandStr,
				ResponseText:  responseText,
				Success:       true,
				ResponseAudio: ttsAudioData,
				AudioFormat:   ttsAudioFormat,
				AudioDuration: ttsAudioDuration,
			}

			if err := stream.Send(response); err != nil {
				logging.LogError(err, "Error sending response to relay",
					zap.String("relay_id", chunk.RelayId),
					zap.String("event_uuid", voiceEvent.UUID),
				)
				return err
			}

			// Set final response and store the complete voice event
			voiceEvent.SetResponse(responseText)
			as.storeVoiceEvent(voiceEvent)

			logging.LogAudioProcessing(chunk.RelayId, "response_sent",
				zap.String("event_uuid", voiceEvent.UUID),
				zap.String("intent", commandStr),
				zap.String("response", responseText),
			)
		}
	}
}

// Helper functions for audio analysis
func findMin(data []float32) float32 {
	if len(data) == 0 {
		return 0
	}
	min := data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
	}
	return min
}

func findMax(data []float32) float32 {
	if len(data) == 0 {
		return 0
	}
	max := data[0]
	for _, v := range data {
		if v > max {
			max = v
		}
	}
	return max
}

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

// storeVoiceEvent stores a voice event in the database
func (as *AudioService) storeVoiceEvent(voiceEvent *events.VoiceEvent) {
	if as.eventsStore == nil {
		logging.LogWarn("Events store not available, skipping voice event storage",
			zap.String("event_uuid", voiceEvent.UUID),
		)
		return
	}

	if err := as.eventsStore.Insert(voiceEvent); err != nil {
		logging.LogError(err, "Failed to store voice event",
			zap.String("event_uuid", voiceEvent.UUID),
			zap.String("relay_id", voiceEvent.RelayID),
		)
	} else {
		logging.LogDatabaseOperation("insert", "voice_events",
			zap.String("event_uuid", voiceEvent.UUID),
			zap.String("intent", voiceEvent.Intent),
			zap.Bool("success", voiceEvent.Success),
		)
	}
}
