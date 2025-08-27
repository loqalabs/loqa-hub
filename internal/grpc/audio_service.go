package grpc

import (
	"io"
	"log"
	"os"
	"time"

	pb "github.com/loqalabs/loqa-proto/go"
	"github.com/loqalabs/loqa-hub/internal/llm"
	"github.com/loqalabs/loqa-hub/internal/messaging"
)

// AudioService implements the gRPC AudioService
type AudioService struct {
	pb.UnimplementedAudioServiceServer
	transcriber    *llm.WhisperTranscriber
	commandParser  *llm.CommandParser
	natsService    *messaging.NATSService
}

// NewAudioService creates a new audio service
func NewAudioService(modelPath string) (*AudioService, error) {
	transcriber, err := llm.NewWhisperTranscriber(modelPath)
	if err != nil {
		return nil, err
	}

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
	}, nil
}

// StreamAudio handles bidirectional audio streaming from pucks
func (as *AudioService) StreamAudio(stream pb.AudioService_StreamAudioServer) error {
	log.Println("🎙️  Hub: New audio stream connected")

	for {
		// Receive audio chunk from puck
		chunk, err := stream.Recv()
		if err == io.EOF {
			log.Println("🎙️  Hub: Audio stream ended")
			return nil
		}
		if err != nil {
			log.Printf("❌ Error receiving audio chunk: %v", err)
			return err
		}

		log.Printf("📥 Hub: Received audio chunk from puck %s (%d bytes, wake_word: %v)", 
			chunk.PuckId, len(chunk.AudioData), chunk.IsWakeWord)

		// Process audio if it's end of speech
		if chunk.IsEndOfSpeech {
			log.Printf("🎯 Hub: Processing complete utterance from puck %s", chunk.PuckId)

			// Convert audio bytes back to float32
			audioData := bytesToFloat32Array(chunk.AudioData)
			
			// Transcribe audio using Whisper
			transcription, err := as.transcriber.Transcribe(audioData, int(chunk.SampleRate))
			if err != nil {
				log.Printf("❌ Error transcribing audio: %v", err)
				continue
			}
			
			wakeWordStatus := ""
			if chunk.IsWakeWord {
				wakeWordStatus = " [wake word detected]"
			}
			log.Printf("📝 Processing audio (%d samples)%s -> \"%s\"", len(audioData), wakeWordStatus, transcription)

			if transcription == "" {
				log.Printf("🔇 No speech detected in audio from puck %s", chunk.PuckId)
				continue
			}

			log.Printf("📝 Transcribed: \"%s\"", transcription)

			// Parse command using LLM
			command, err := as.commandParser.ParseCommand(transcription)
			if err != nil {
				log.Printf("❌ Error parsing command: %v", err)
				command = &llm.Command{
					Intent:     "unknown",
					Entities:   make(map[string]string),
					Confidence: 0.0,
					Response:   "I'm having trouble understanding you right now.",
				}
			}

			commandStr := command.Intent
			responseText := command.Response
			
			log.Printf("🧠 Parsed command - Intent: %s, Entities: %v, Confidence: %.2f", 
				command.Intent, command.Entities, command.Confidence)

			// Publish command event to NATS
			if as.natsService != nil && as.natsService.IsConnected() {
				commandEvent := &messaging.CommandEvent{
					PuckID:        chunk.PuckId,
					Transcription: transcription,
					Intent:        command.Intent,
					Entities:      command.Entities,
					Confidence:    command.Confidence,
					Timestamp:     time.Now().UnixNano(),
					RequestID:     chunk.PuckId, // Using puck ID as request ID for now
				}

				if err := as.natsService.PublishVoiceCommand(commandEvent); err != nil {
					log.Printf("⚠️  Warning: Failed to publish voice command to NATS: %v", err)
				}

				// If this is a device command, also publish a device command event
				if as.isDeviceCommand(command.Intent) {
					deviceCommand := as.createDeviceCommand(commandEvent)
					if deviceCommand != nil {
						if err := as.natsService.PublishDeviceCommand(deviceCommand); err != nil {
							log.Printf("⚠️  Warning: Failed to publish device command to NATS: %v", err)
						}
					}
				}
			}

			// Send response back to puck
			response := &pb.AudioResponse{
				RequestId:     chunk.PuckId, // Use puck ID as request ID
				Transcription: transcription,
				Command:       commandStr,
				ResponseText:  responseText,
				Success:       true,
			}

			if err := stream.Send(response); err != nil {
				log.Printf("❌ Error sending response: %v", err)
				return err
			}

			log.Printf("📤 Hub: Sent response to puck %s - Command: %s", 
				chunk.PuckId, commandStr)
		}
	}
}

// Helper function to convert bytes back to float32 array
func bytesToFloat32Array(data []byte) []float32 {
	// Convert 16-bit PCM bytes to float32 samples
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
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
			"lights": "lights",
			"light":  "lights",
			"lamp":   "lights",
			"music":  "audio",
			"audio":  "audio",
			"sound":  "audio",
			"tv":     "tv",
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