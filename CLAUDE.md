# Development Guidance

See [/loqalabs/CLAUDE.md](../CLAUDE.md) for complete development workflow guidance.

## Service Context

**loqa-hub** - Central orchestrator service with STT/TTS/LLM pipeline (Go)

- **Role**: Core service handling voice processing, skills coordination, and API endpoints
- **🆕 Streaming**: Real-time LLM responses with progressive audio synthesis
- **Quality Gates**: `make quality-check` (includes go fmt, go vet, golangci-lint, tests, docker build)
- **Development**: `go run ./cmd` (dev), `go build -o bin/hub ./cmd` (build)
- **Dependencies**: loqa-proto (gRPC definitions), loqa-skills (plugin system)

## 🆕 Streaming Architecture Development

### Key Components

- **StreamingCommandParser** (`internal/llm/streaming_command_parser.go`)
  - Handles token-level streaming from Ollama
  - Intelligent phrase buffering for natural audio boundaries
  - Progressive response generation with visual and audio channels

- **StreamingAudioPipeline** (`internal/llm/streaming_audio_pipeline.go`)
  - Parallel TTS synthesis with sequence ordering
  - Concurrent worker pools for audio generation
  - Memory-safe pipeline management

- **StreamingInterruptHandler** (`internal/llm/streaming_interrupt_handler.go`)
  - Graceful stream cancellation with 500ms timeout
  - Session lifecycle management
  - Resource cleanup to prevent leaks

- **StreamingMetricsCollector** (`internal/llm/streaming_metrics.go`)
  - Real-time performance monitoring
  - Health status assessment
  - Performance trend analysis and recommendations

### Development Guidelines

- **Memory Safety**: Always use context cancellation and cleanup goroutines
- **Error Handling**: Implement graceful fallback to non-streaming mode
- **Testing**: Comprehensive unit tests with mock TTS clients and timeout handling
- **Performance**: Monitor first-token latency, throughput, and buffer efficiency
- **Configuration**: Use feature flags for production rollout

### Configuration

Streaming behavior is controlled via `config.StreamingConfig`:

```go
type StreamingConfig struct {
    Enabled              bool          // Feature flag
    OllamaURL           string        // Streaming endpoint
    Model               string        // LLM model
    MaxBufferTime       time.Duration // Phrase buffering
    MaxTokensPerPhrase  int           // Buffer size limits
    AudioConcurrency    int           // TTS worker count
    VisualFeedbackDelay time.Duration // UI responsiveness
    InterruptTimeout    time.Duration // Cleanup timeout
    FallbackEnabled     bool          // Graceful degradation
    MetricsEnabled      bool          // Performance tracking
}
```

## 🚀 Predictive Response Architecture Development

### Key Components

- **PredictiveResponseEngine** (`internal/llm/predictive_response.go`)
  - Instant acknowledgments decoupled from execution
  - Asynchronous skill execution with worker pools
  - Device reliability tracking for prediction accuracy
  - Smart status update strategies (Silent/ErrorOnly/Verbose/Progress)

- **CommandClassifier** (`internal/llm/command_classifier.go`)
  - Dynamic intent classification beyond static categories (15+ intent types)
  - Confidence scoring and device reliability integration
  - Optimistic vs cautious response pattern selection

- **AsyncExecutionPipeline** (`internal/llm/async_execution.go`)
  - Background skill execution with retry logic
  - Worker pool management for concurrent operations
  - Performance metrics and error handling

- **StatusManager** (`internal/llm/status_manager.go`)
  - Intelligent status update routing based on operation type
  - Error pattern detection and automated recovery
  - TTS integration for audio feedback

- **StreamingPredictiveBridge** (`internal/llm/streaming_predictive_bridge.go`)
  - Integration between streaming LLM and predictive response
  - Session management and hybrid response strategies
  - Graceful fallback between streaming and predictive modes

### Development Guidelines

- **Instant Feedback**: Always provide immediate acknowledgment (<200ms perceived)
- **Async Execution**: Never block user interaction waiting for device responses
- **Smart Updates**: Use appropriate update strategy based on operation type and device reliability
- **Error Recovery**: Implement graceful corrections when predictions fail
- **Confidence-Based**: Adapt response patterns based on command/device confidence

### Response Type Patterns

```go
type PredictiveType string
const (
    PredictiveOptimistic PredictiveType = "optimistic" // High confidence + reliable device
    PredictiveCautious   PredictiveType = "cautious"   // Uncertain command or device
    PredictiveConfirm    PredictiveType = "confirm"    // Critical operations requiring confirmation
    PredictiveProgress   PredictiveType = "progress"   // Slow operations needing status updates
)
```

### Configuration

Predictive response behavior is controlled via `config.PredictiveConfig`:

```go
type PredictiveConfig struct {
    Enabled              bool          // Feature flag
    ConfidenceThreshold  float64       // Minimum confidence for optimistic responses
    ExecutionTimeout     time.Duration // Max time for async execution
    StatusUpdateStrategy UpdateStrategy // Default update strategy
    DeviceReliabilityMin float64       // Minimum device reliability for optimistic responses
}
```

All workflow rules and development guidance are provided automatically by the MCP server based on repository detection.