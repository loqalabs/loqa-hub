# Multi-Relay Collision Detection & Arbitration

This document describes the collision detection and arbitration system implemented in the Loqa Hub to handle multiple relays simultaneously detecting wake words and attempting to stream audio.

## Overview

When multiple relay devices are present in the same environment, they may all detect the same wake word simultaneously. Without collision detection, this would result in multiple concurrent audio streams processing the same voice command, leading to duplicate responses and poor user experience.

The collision detection system solves this by implementing a temporal arbitration window combined with signal strength analysis to select the "winning" relay that should handle the voice command.

## Architecture

### Core Components

- **ArbitrationWindow**: Time-bounded window for collecting competing relays
- **RelayStream**: Tracks individual relay state and audio data
- **RelayStatus**: Enum tracking relay lifecycle states
- **AudioService**: Enhanced with collision detection orchestration

### Key Data Structures

```go
type RelayStatus int32

const (
    RelayStatusConnected   RelayStatus = 0  // Single relay, no arbitration needed
    RelayStatusContending  RelayStatus = 1  // Multiple relays, arbitration in progress
    RelayStatusWinner      RelayStatus = 2  // Won arbitration, processing command
    RelayStatusCancelled   RelayStatus = 3  // Lost arbitration, cancelled
)

type RelayStream struct {
    RelayID         string
    Stream          pb.AudioService_StreamAudioServer
    WakeWordSignal  []float32  // Audio samples for signal strength analysis
    SignalStrength  float64    // Calculated RMS signal strength
    Status          RelayStatus
    CancelChannel   chan struct{}
}

type ArbitrationWindow struct {
    StartTime      time.Time
    WindowDuration time.Duration
    Relays         map[string]*RelayStream
    WinnerID       string
    IsActive       bool
}
```

## Arbitration Process

### 1. First Relay Detection

When the first relay sends a wake word detection:

1. Check if arbitration window is already active
2. If not, start new arbitration window (300ms duration)
3. Add relay to the window as first contender
4. Set relay status to `RelayStatusContending`

### 2. Additional Relays Join

When subsequent relays detect the same wake word within the window:

1. Attempt to join existing arbitration window
2. If window is still active, add relay as contender
3. If window has closed, reject the relay (send cancellation)

### 3. Arbitration Resolution

When the arbitration window expires:

1. Calculate signal strength for all contending relays using RMS analysis
2. Select relay with highest signal strength as winner
3. Update winner status to `RelayStatusWinner`
4. Send cancellation messages to all losing relays
5. Update losing relay status to `RelayStatusCancelled`
6. Mark arbitration window as inactive

### 4. Command Processing

After arbitration:

1. Winner relay continues normal audio streaming
2. Hub processes intent and generates response
3. Response sent only to winning relay
4. Losing relays are cleaned up and removed from active streams

## Signal Strength Analysis

### RMS Calculation

Signal strength is calculated using Root Mean Square (RMS) analysis of the wake word audio samples:

```go
func (as *AudioService) calculateSignalStrength(samples []float32) float64 {
    if len(samples) == 0 {
        return 0.0
    }

    var sum float64
    for _, sample := range samples {
        sum += float64(sample * sample)
    }

    return math.Sqrt(sum / float64(len(samples)))
}
```

### Why RMS?

- **Volume Independence**: RMS provides a better measure of signal energy than simple amplitude
- **Noise Resilience**: More resistant to brief noise spikes
- **Distance Correlation**: Higher RMS typically indicates closer proximity to speaking user
- **Real-time Compatible**: Can be calculated incrementally as audio streams arrive

## Temporal Window Strategy

### Window Duration: 300ms

The arbitration window duration is carefully chosen:

- **Long enough**: Accounts for network latency and relay processing variations
- **Short enough**: Maintains responsive user experience
- **Practical**: Most relay devices will detect wake words within 200ms of each other

### Window Timing

```
User speaks wake word
         |
    Relay A detects (t=0ms)     ← Arbitration window starts
    Relay B detects (t=50ms)    ← Joins window
    Relay C detects (t=150ms)   ← Joins window
         |
    Window closes (t=300ms)     ← Arbitration performed
         |
    Winner selected             ← Losers cancelled
```

## Implementation Details

### Concurrency Safety

All relay management operations are protected by `streamsMutex`:

```go
as.streamsMutex.Lock()
defer as.streamsMutex.Unlock()
```

### Memory Management

- Losing relays are automatically cleaned up after cancellation
- Arbitration windows are reset after completion
- Audio buffers are released when relays are cancelled

### Error Handling

- Network failures during arbitration gracefully handled
- Relay disconnections automatically trigger cleanup
- Invalid audio data doesn't crash arbitration process

## Testing

### Unit Tests

The collision detection system includes comprehensive unit tests:

- `TestCalculateSignalStrength`: Validates RMS signal strength calculations
- `TestArbitrationWindow`: Tests window lifecycle and relay management
- `TestJoinArbitrationWindowAfterTimeout`: Validates timing constraints
- `TestRelayStatusChecking`: Verifies relay state management
- `TestSignalStrengthArbitration`: End-to-end arbitration scenarios
- `TestCleanupRelay`: Memory management and cleanup validation

### MockStream Implementation

Testing uses `MockStream` to simulate gRPC streaming interfaces without network overhead.

## Configuration

### Environment Variables

```bash
# Arbitration window duration (default: 300ms)
ARBITRATION_WINDOW_DURATION=300ms

# Maximum concurrent relays (default: 10)
MAX_CONCURRENT_RELAYS=10
```

### Tuning Guidelines

- **Increase window duration** if relays have high network latency
- **Decrease window duration** for more responsive experience in low-latency environments
- **Monitor signal strength thresholds** for environments with varying acoustics

## Performance Characteristics

### Latency Impact

- **Single relay**: No additional latency (bypass arbitration)
- **Multiple relays**: 300ms arbitration delay maximum
- **Signal processing**: <1ms for RMS calculation per relay

### Memory Usage

- **Per relay**: ~100 bytes for RelayStream metadata
- **Audio buffering**: Wake word samples only (~2KB per relay for 250ms at 16kHz)
- **Cleanup**: Automatic memory release for cancelled relays

### Scalability

- Designed for typical home environments (2-5 relays)
- Can handle up to 10 concurrent relays effectively
- Linear performance degradation with relay count

## Future Enhancements

### Planned Improvements

1. **Adaptive Windows**: Adjust window duration based on environment characteristics
2. **Machine Learning**: Train models to predict optimal arbitration strategies
3. **User Preferences**: Allow users to prefer specific relays (bedroom vs kitchen)
4. **Acoustic Fingerprinting**: Use room acoustics to improve relay selection

### Advanced Features

- **Multi-command Coordination**: Coordinate complex multi-step commands across relays
- **Load Balancing**: Distribute processing across multiple hub instances
- **Fault Tolerance**: Handle hub failures during active arbitration

## Troubleshooting

### Common Issues

**Multiple responses to single command:**
- Check arbitration window duration
- Verify relay wake word detection timing
- Monitor signal strength calculations

**Delayed responses:**
- Reduce arbitration window duration
- Check network latency between relays and hub
- Verify gRPC connection health

**Wrong relay selected:**
- Validate microphone placement and sensitivity
- Check for acoustic interference
- Monitor RMS signal strength values

### Debug Logging

Enable detailed collision detection logging:

```bash
LOG_LEVEL=debug go run ./cmd
```

Key log messages:
- `Started arbitration window for relay`
- `Relay joined arbitration window`
- `Arbitration completed, winner selected`
- `Cancelled losing relay`

## Security Considerations

### Relay Authentication

Future implementations should include:
- Relay device authentication
- Encrypted wake word transmission
- Anti-spoofing measures for signal strength

### Privacy Protection

- Wake word audio is not stored permanently
- Signal strength calculations performed in memory only
- Relay identifiers can be anonymized for logging

## Integration Notes

### Backward Compatibility

- Single relay deployments experience no behavior changes
- Existing gRPC protocol remains unchanged
- Legacy relays continue to function normally

### Protocol Extensions

Future protocol versions may include:
- Relay capability negotiation
- Advanced arbitration parameters
- Real-time arbitration status updates