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
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/loqalabs/loqa-hub/internal/events"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/logging"
	"github.com/loqalabs/loqa-hub/internal/messaging"
	"github.com/loqalabs/loqa-hub/internal/storage"
	pb "github.com/loqalabs/loqa-proto/go/audio"
	"go.uber.org/zap"
)

// AudioService implements the gRPC AudioService
type AudioService struct {
	pb.UnimplementedAudioServiceServer
	transcriber   llm.Transcriber
	commandParser *llm.CommandParser
	natsService   *messaging.NATSService
	eventsStore   *storage.VoiceEventsStore
}

// NewAudioServiceWithSTT creates a new audio service using OpenAI-compatible STT service
func NewAudioServiceWithSTT(sttURL string, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {
	transcriber, err := llm.NewSTTClient(sttURL)
	if err != nil {
		return nil, err
	}
	return createAudioService(transcriber, eventsStore)
}

// createAudioService is a helper to create the service with any transcriber implementation
func createAudioService(transcriber llm.Transcriber, eventsStore *storage.VoiceEventsStore) (*AudioService, error) {

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

	// Test connection to Ollama (non-blocking)
	go func() {
		if err := commandParser.TestConnection(); err != nil {
			log.Printf("⚠️  Warning: Cannot connect to Ollama: %v", err)
			log.Println("🔄 Command parsing will use fallback logic")
		}
	}()

	return &AudioService{
		transcriber:   transcriber,
		commandParser: commandParser,
		natsService:   natsService,
		eventsStore:   eventsStore,
	}, nil
}

// StreamAudio handles bidirectional audio streaming from relay devices
func (as *AudioService) StreamAudio(stream pb.AudioService_StreamAudioServer) error {
	logging.Sugar.Info("🎙️  Hub: New audio stream connected")

	for {
		// Receive audio chunk from relay
		chunk, err := stream.Recv()
		if err == io.EOF {
			logging.Sugar.Info("🎙️  Hub: Audio stream ended")
			return nil
		}
		if err != nil {
			logging.LogError(err, "Error receiving audio chunk")
			return err
		}

		logging.LogAudioProcessing(chunk.RelayId, "received",
			zap.Int("bytes", len(chunk.AudioData)),
			zap.Bool("wake_word", chunk.IsWakeWord),
		)

		// Process audio if it's end of speech
		if chunk.IsEndOfSpeech {
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

			// Transcribe audio using STT service (with panic recovery and detailed logging)
			var transcription string
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

				transcription, err = as.transcriber.Transcribe(audioData, sampleRate)

				logging.LogAudioProcessing(chunk.RelayId, "stt_returned",
					zap.String("event_uuid", voiceEvent.UUID),
					zap.Bool("success", err == nil),
					zap.Int("transcription_length", len(transcription)),
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

			// Set transcription result
			voiceEvent.SetTranscription(transcription)

			logging.LogAudioProcessing(chunk.RelayId, "transcribed",
				zap.String("event_uuid", voiceEvent.UUID),
				zap.Int("audio_samples", len(audioData)),
				zap.String("transcription", transcription),
				zap.Bool("wake_word", chunk.IsWakeWord),
			)

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

			// Parse command using LLM
			command, err := as.commandParser.ParseCommand(transcription)
			if err != nil {
				logging.LogError(err, "Error parsing command",
					zap.String("relay_id", chunk.RelayId),
					zap.String("event_uuid", voiceEvent.UUID),
					zap.String("transcription", transcription),
				)
				command = &llm.Command{
					Intent:     "unknown",
					Entities:   make(map[string]string),
					Confidence: 0.0,
					Response:   "I'm having trouble understanding you right now.",
				}
			}

			// Set command parsing results
			voiceEvent.SetCommandResult(command.Intent, command.Entities, command.Confidence)

			commandStr := command.Intent
			responseText := command.Response

			logging.LogAudioProcessing(chunk.RelayId, "command_parsed",
				zap.String("event_uuid", voiceEvent.UUID),
				zap.String("intent", command.Intent),
				zap.Float64("confidence", command.Confidence),
				zap.Any("entities", command.Entities),
			)

			// Publish command event to NATS
			if as.natsService != nil && as.natsService.IsConnected() {
				commandEvent := &messaging.CommandEvent{
					RelayID:       chunk.RelayId,
					Transcription: transcription,
					Intent:        command.Intent,
					Entities:      command.Entities,
					Confidence:    command.Confidence,
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
						zap.String("intent", command.Intent),
					)
				}

				// If this is a device command, also publish a device command event
				if as.isDeviceCommand(command.Intent) {
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

			// Send response back to relay
			response := &pb.AudioResponse{
				RequestId:     chunk.RelayId, // Use relay ID as request ID
				Transcription: transcription,
				Command:       commandStr,
				ResponseText:  responseText,
				Success:       true,
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
		return []float32{}
	}

	// Ensure even number of bytes for 16-bit PCM
	dataLen := len(data)
	if dataLen%2 != 0 {
		dataLen -= 1 // Drop the last incomplete sample
	}

	// Convert 16-bit PCM bytes to float32 samples
	samples := make([]float32, dataLen/2)
	for i := 0; i < len(samples); i++ {
		// Add bounds checking
		if i*2+1 >= len(data) {
			break
		}
		// Reconstruct int16 from bytes (little-endian)
		val := int16(data[i*2]) | int16(data[i*2+1])<<8
		// Convert to float32 [-1,1]
		samples[i] = float32(val) / 32767.0
	}
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
