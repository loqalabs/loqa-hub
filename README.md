[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by LoqaLabs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20LoqaLabs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

# 🧠 Loqa Hub

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml)

Central orchestrator for the Loqa local-first voice assistant platform with HTTP/1.1 streaming architecture.

## Overview

Loqa Hub is the core stateless service that handles:
- **HTTP/1.1 Binary Streaming**: Real-time audio frame processing from ESP32 pucks
- **Wake Word Arbitration**: Multi-puck wake word competition with 500ms windows
- **Intent Cascade Processing**: Reflex → Local LLM → Cloud pipeline with privacy controls
- **Performance Tier Detection**: Automatic capability detection with graceful degradation
- **Skills Integration**: Modular plugin system for voice command handling
- **STT/TTS Integration**: OpenAI-compatible speech services
- **Stateless Design**: No persistent storage, fully event-driven architecture

## Architecture

### 🆕 HTTP/1.1 Streaming Transport

The new stateless architecture uses HTTP/1.1 chunked transfer encoding for real-time communication:

- **Binary Frame Protocol**: Efficient 4KB frames optimized for ESP32 devices
- **Magic Number Validation**: "LOQA" (0x4C4F5141) for protocol integrity
- **Frame Types**: Audio data, wake words, heartbeats, control signals
- **Session Management**: Stateless session tracking via frame headers
- **ESP32 Optimized**: 24-byte headers, minimal memory footprint

### 🎯 Wake Word Arbitration

Multi-puck wake word detection with intelligent arbitration:

- **Competition Window**: 500ms window for multiple puck responses
- **Scoring Algorithm**: Distance, confidence, and signal quality based
- **Winner Selection**: Highest scoring puck gets voice processing rights
- **Fair Arbitration**: Prevents single puck dominance

### 🧠 Intent Cascade Processing

Three-tier intent processing with privacy controls:

1. **Reflex Layer**: Instant responses for simple commands
2. **Local LLM**: Ollama-based processing for complex intents
3. **Cloud Processing**: Optional tier for advanced AI (privacy-optional)

### 🎚️ Performance Tier Detection

Automatic system capability detection:

- **Basic Tier**: Reflex-only processing, minimal resources
- **Standard Tier**: Local LLM, full features (4+ cores, 8GB+ RAM)
- **Pro Tier**: Advanced processing, experimental features (8+ cores, 16GB+ RAM)
- **Graceful Degradation**: Automatic fallback during performance issues

## Features

- 🌐 **HTTP/1.1 Streaming**: Real-time binary frame processing optimized for ESP32
- 🎯 **Wake Word Arbitration**: Multi-puck competition with intelligent winner selection
- 🧠 **Intent Cascade**: Reflex → LLM → Cloud processing pipeline
- 🎚️ **Adaptive Performance**: Automatic tier detection with graceful degradation
- 🧩 **Skills System**: Modular plugin architecture for extensible voice commands
- 🔒 **Privacy-First**: Local processing by default, cloud processing optional
- ⚡ **Stateless Design**: No database dependencies, fully event-driven
- 📊 **Real-time Metrics**: Performance monitoring and health status

### 🆕 Milestone 4a: Modular Skill Plugin Architecture

- 🧩 **Skill Plugin System**: Extensible architecture for voice command handling
- 📋 **Manifest-Driven**: JSON-based skill configuration with permissions and sandboxing
- 🔄 **Dynamic Loading**: Load, unload, and reload skills at runtime
- 🛡️ **Security**: Trust levels and sandbox modes for safe skill execution
- 🎛️ **Management Tools**: CLI and web UI for skill administration
- 🌐 **REST API**: Complete skill management via `/api/skills` endpoints
- 🔧 **Multi-Format Support**: Go plugins, process-based skills, and future WASM support

### 🆕 Stateless Event Processing

- 📊 **Real-time Events**: Every interaction generates structured events for immediate processing
- 🌐 **HTTP API Endpoints**: RESTful endpoints for system status and capabilities
- 📝 **Structured Logging**: Rich context logging with Zap (configurable JSON/console output)
- ⏱️ **Performance Metrics**: Real-time latency and throughput monitoring
- 🚨 **Health Status**: Comprehensive system health and degradation detection
- 🎯 **Arbitration Metrics**: Wake word competition statistics and analysis

### 🆕 Milestone 4b: Multi-Command Intent Parsing

- 🔗 **Compound Utterances**: Parse complex voice commands like "turn on the lights and play music"
- 🔄 **Sequential Execution**: Command queue system executes commands one by one to prevent conflicts
- ⏪ **Rollback Support**: Failed commands trigger rollback of previously executed commands in the chain
- 🗣️ **Response Aggregation**: Intelligent combination of multiple command responses for natural TTS
- ⚡ **Performance Optimized**: <200ms execution time per additional command
- 📊 **Conjunction Detection**: Supports "and", "then", "after that", "next", "also" conjunctions

### 🆕 Milestone 5a: STT Confidence & Wake Word Processing

- 🗣️ **Wake Word Normalization**: Automatically strips wake words ("Hey Loqa", "Hey Luca", etc.) before intent parsing
- 🎯 **Confidence Thresholds**: Estimates transcription confidence and handles low-quality audio gracefully
- 🔄 **Fallback Confirmation**: Prompts users to repeat unclear commands instead of failing silently
- 📊 **Pattern Recognition**: Detects nonsensical patterns, repetition, and other quality indicators
- 🎛️ **Configurable Thresholds**: 60% default confidence threshold with room for user customization
- 🔍 **Enhanced Logging**: Detailed confidence and wake word detection information in voice events
- 🔄 **Backward Compatible**: Seamless fallback to single-command parsing when needed

### 🆕 Milestone 6: Real-Time Streaming LLM Responses

- ⚡ **Token-Level Streaming**: Immediate visual feedback as LLM generates responses
- 🎵 **Progressive Audio Synthesis**: Parallel TTS processing with intelligent phrase buffering
- 🔄 **Graceful Interruption**: 500ms timeout for clean stream cancellation
- 📊 **Performance Metrics**: Real-time monitoring of streaming latency and throughput
- 🛡️ **Fallback Mode**: Automatic degradation to traditional parsing when streaming fails
- 🧠 **Intelligent Buffering**: Natural speech boundaries detection for optimal audio quality
- ⚙️ **Configurable Streaming**: Feature flags and settings for production deployment
- 🎯 **Memory Safety**: Comprehensive cleanup to prevent goroutine leaks and resource exhaustion

## Endpoints

The Hub service provides HTTP/1.1 streaming and RESTful API endpoints:

### Streaming Endpoints
- `/stream/puck` - HTTP/1.1 binary streaming for ESP32 pucks
- `/api/capabilities` - System capabilities and performance tier information
- `/api/arbitration/stats` - Wake word arbitration statistics
- `/api/tier` - Current performance tier and feature availability

### Management Endpoints
- `/health` - System health status with service availability
- `/api/intent/process` - Text-based intent processing for testing

## Configuration

### Environment Variables

#### Core Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LOQA_PORT` | `3000` | HTTP server port for streaming and API |
| `STT_URL` | `http://stt:8000` | OpenAI-compatible STT service URL |
| `TTS_URL` | `http://tts:8880` | OpenAI-compatible TTS service URL |
| `OLLAMA_URL` | `http://ollama:11434` | Ollama API endpoint for intent processing |
| `OLLAMA_MODEL` | `llama3.2:3b` | Ollama model for intent parsing |
| `NATS_URL` | `nats://nats:4222` | NATS server URL for event publishing |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `LOG_FORMAT` | `console` | Log output format (json, console) |

#### 🆕 Streaming Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `STREAMING_ENABLED` | `true` | Enable real-time streaming LLM responses |
| `STREAMING_OLLAMA_URL` | `http://localhost:11434` | Ollama API URL for streaming |
| `STREAMING_MODEL` | `llama3.2:3b` | LLM model for streaming responses |
| `STREAMING_MAX_BUFFER_TIME` | `2s` | Maximum time to buffer tokens before TTS |
| `STREAMING_MAX_TOKENS_PER_PHRASE` | `50` | Maximum tokens to buffer per phrase |
| `STREAMING_AUDIO_CONCURRENCY` | `3` | Number of parallel TTS synthesis workers |
| `STREAMING_VISUAL_FEEDBACK_DELAY` | `50ms` | Delay before showing visual tokens |
| `STREAMING_INTERRUPT_TIMEOUT` | `500ms` | Timeout for graceful stream interruption |
| `STREAMING_FALLBACK_ENABLED` | `true` | Enable fallback to non-streaming mode |
| `STREAMING_METRICS_ENABLED` | `true` | Enable streaming performance metrics |

### API Access

The Hub exposes HTTP/1.1 streaming and RESTful APIs:

**Streaming Transport:**
- `POST /stream/puck` - HTTP/1.1 binary streaming endpoint for ESP32 pucks

**System Information:**
- `GET /health` - System health and service availability
- `GET /api/capabilities` - Performance tier and system capabilities
- `GET /api/tier` - Current tier and feature availability
- `GET /api/arbitration/stats` - Wake word arbitration statistics

**Intent Processing:**
- `POST /api/intent/process` - Process text through intent cascade

See [`API.md`](API.md) for complete endpoint documentation with examples.

## Getting Started

See the main [Loqa documentation](https://github.com/loqalabs/loqa) for setup and usage instructions.

## Development

### Claude Code Configuration

This repository includes a `.claude-code.json` configuration file that provides Claude Code with project-specific context:

- **Service Role**: Central hub service that orchestrates communication between all microservices
- **Testing Requirements**: Comprehensive unit, integration, and microservice communication testing
- **Architecture Awareness**: Understands dependencies on loqa-proto and loqa-skills
- **Cross-Service Coordination**: Knows how to coordinate changes across the microservice ecosystem

See the main project documentation at [`loqa/config/CLAUDE.md`](https://github.com/loqalabs/loqa/blob/main/config/CLAUDE.md) for complete details.

### Local Development Setup

```bash
# Build the project
make build

# Run tests
make test

# Run linting (catches CI errors locally)
make lint-fast

# Run all pre-commit checks
make pre-commit

# Install development tools
make install-tools
```

### Pre-commit Workflow

Before pushing code, run the pre-commit checks to catch CI errors early:

```bash
make pre-commit
```

This will run formatting, linting, and tests - the same checks that run in CI.

## License

Licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details. 
# Workflow name standardization
