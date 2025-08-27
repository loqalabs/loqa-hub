package messaging

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSService handles NATS messaging for the Loqa system
type NATSService struct {
	conn *nats.Conn
	url  string
}

// CommandEvent represents a voice command event
type CommandEvent struct {
	PuckID       string            `json:"puck_id"`
	Transcription string           `json:"transcription"`
	Intent       string            `json:"intent"`
	Entities     map[string]string `json:"entities"`
	Confidence   float64           `json:"confidence"`
	Timestamp    int64             `json:"timestamp"`
	RequestID    string            `json:"request_id"`
}

// DeviceCommandEvent represents a command to execute on devices
type DeviceCommandEvent struct {
	CommandEvent
	DeviceType string `json:"device_type"`
	DeviceID   string `json:"device_id,omitempty"`
	Location   string `json:"location,omitempty"`
	Action     string `json:"action"`
}

// DeviceResponseEvent represents a response from device execution
type DeviceResponseEvent struct {
	RequestID   string `json:"request_id"`
	DeviceType  string `json:"device_type"`
	DeviceID    string `json:"device_id,omitempty"`
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	Timestamp   int64  `json:"timestamp"`
}

// NATS subjects for different event types
const (
	SubjectVoiceCommands   = "loqa.voice.commands"
	SubjectDeviceCommands  = "loqa.devices.commands"
	SubjectDeviceResponses = "loqa.devices.responses"
	SubjectSystemEvents    = "loqa.system.events"
)

// NewNATSService creates a new NATS service instance
func NewNATSService() (*NATSService, error) {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	return &NATSService{
		url: natsURL,
	}, nil
}

// Connect establishes connection to NATS server
func (ns *NATSService) Connect() error {
	log.Printf("🔌 Connecting to NATS at %s", ns.url)

	// Connection options with retry logic
	opts := []nats.Option{
		nats.Name("loqa-hub"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Retry indefinitely
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("⚠️  NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("🔄 NATS reconnected to %s", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Println("🔌 NATS connection closed")
		}),
	}

	conn, err := nats.Connect(ns.url, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	ns.conn = conn
	log.Printf("✅ Connected to NATS server at %s", conn.ConnectedUrl())
	return nil
}

// PublishVoiceCommand publishes a voice command event
func (ns *NATSService) PublishVoiceCommand(event *CommandEvent) error {
	if ns.conn == nil {
		return fmt.Errorf("NATS connection not established")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal command event: %w", err)
	}

	subject := SubjectVoiceCommands
	if err := ns.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	log.Printf("📤 Published voice command to NATS - Intent: %s, PuckID: %s", 
		event.Intent, event.PuckID)
	return nil
}

// PublishDeviceCommand publishes a device command event
func (ns *NATSService) PublishDeviceCommand(event *DeviceCommandEvent) error {
	if ns.conn == nil {
		return fmt.Errorf("NATS connection not established")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal device command event: %w", err)
	}

	// Create specific subject based on device type
	subject := fmt.Sprintf("%s.%s", SubjectDeviceCommands, event.DeviceType)
	if err := ns.conn.Publish(subject, data); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}

	log.Printf("📤 Published device command to NATS - Device: %s, Action: %s", 
		event.DeviceType, event.Action)
	return nil
}

// SubscribeToVoiceCommands subscribes to voice command events
func (ns *NATSService) SubscribeToVoiceCommands(handler func(*CommandEvent)) (*nats.Subscription, error) {
	if ns.conn == nil {
		return nil, fmt.Errorf("NATS connection not established")
	}

	return ns.conn.Subscribe(SubjectVoiceCommands, func(msg *nats.Msg) {
		var event CommandEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("❌ Error unmarshaling voice command: %v", err)
			return
		}

		log.Printf("📥 Received voice command from NATS - Intent: %s, PuckID: %s", 
			event.Intent, event.PuckID)
		handler(&event)
	})
}

// SubscribeToDeviceCommands subscribes to device command events
func (ns *NATSService) SubscribeToDeviceCommands(deviceType string, handler func(*DeviceCommandEvent)) (*nats.Subscription, error) {
	if ns.conn == nil {
		return nil, fmt.Errorf("NATS connection not established")
	}

	subject := fmt.Sprintf("%s.%s", SubjectDeviceCommands, deviceType)
	return ns.conn.Subscribe(subject, func(msg *nats.Msg) {
		var event DeviceCommandEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("❌ Error unmarshaling device command: %v", err)
			return
		}

		log.Printf("📥 Received device command from NATS - Device: %s, Action: %s", 
			event.DeviceType, event.Action)
		handler(&event)
	})
}

// SubscribeToDeviceResponses subscribes to device response events
func (ns *NATSService) SubscribeToDeviceResponses(handler func(*DeviceResponseEvent)) (*nats.Subscription, error) {
	if ns.conn == nil {
		return nil, fmt.Errorf("NATS connection not established")
	}

	return ns.conn.Subscribe(SubjectDeviceResponses, func(msg *nats.Msg) {
		var event DeviceResponseEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			log.Printf("❌ Error unmarshaling device response: %v", err)
			return
		}

		log.Printf("📥 Received device response from NATS - Device: %s, Success: %t", 
			event.DeviceType, event.Success)
		handler(&event)
	})
}

// PublishDeviceResponse publishes a device response event
func (ns *NATSService) PublishDeviceResponse(event *DeviceResponseEvent) error {
	if ns.conn == nil {
		return fmt.Errorf("NATS connection not established")
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal device response event: %w", err)
	}

	if err := ns.conn.Publish(SubjectDeviceResponses, data); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", SubjectDeviceResponses, err)
	}

	log.Printf("📤 Published device response to NATS - Device: %s, Success: %t", 
		event.DeviceType, event.Success)
	return nil
}

// Close closes the NATS connection
func (ns *NATSService) Close() {
	if ns.conn != nil {
		ns.conn.Close()
		log.Println("🔌 NATS connection closed")
	}
}

// IsConnected returns true if connected to NATS
func (ns *NATSService) IsConnected() bool {
	return ns.conn != nil && ns.conn.IsConnected()
}

// GetStats returns connection statistics
func (ns *NATSService) GetStats() nats.Statistics {
	if ns.conn != nil {
		return ns.conn.Stats()
	}
	return nats.Statistics{}
}