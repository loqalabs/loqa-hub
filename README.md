[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by LoqaLabs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20LoqaLabs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

# 🧠 Loqa Hub

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml)

Central orchestrator for the Loqa local-first voice assistant platform.

## Overview

Loqa Hub is the core service that handles:
- gRPC API for audio input from relay devices
- Speech-to-text processing via OpenAI-compatible STT service
- LLM-based intent parsing and command extraction with **multi-command support**
- Sequential command execution with rollback capabilities
- NATS integration for publishing commands to other services
- **NEW:** Complete voice event tracking and observability

## Features

- 🎤 **Audio Processing**: Receives audio streams from relay devices via gRPC
- 📝 **Speech Recognition**: Local speech-to-text using OpenAI-compatible STT service with confidence thresholds and wake word normalization
- 🤖 **Intent Parsing**: Natural language understanding via Ollama LLM with multi-command support
- 🔗 **Multi-Command Processing**: Parse and execute compound utterances like "turn on the lights and play music"
- 📡 **Event Publishing**: Publishes parsed commands to NATS message bus
- 🔄 **Command Queue**: Sequential execution with rollback capabilities for failed command chains
- ⚡ **Performance**: <200ms execution time per additional command in multi-command utterances
- 🔒 **Privacy-First**: All processing happens locally, no cloud dependencies

### 🆕 Milestone 4a: Modular Skill Plugin Architecture

- 🧩 **Skill Plugin System**: Extensible architecture for voice command handling
- 📋 **Manifest-Driven**: JSON-based skill configuration with permissions and sandboxing
- 🔄 **Dynamic Loading**: Load, unload, and reload skills at runtime
- 🛡️ **Security**: Trust levels and sandbox modes for safe skill execution
- 🎛️ **Management Tools**: CLI and web UI for skill administration
- 🌐 **REST API**: Complete skill management via `/api/skills` endpoints
- 🔧 **Multi-Format Support**: Go plugins, process-based skills, and future WASM support

### 🆕 Milestone 2: Observability & Event Tracking

- 📊 **Voice Event Tracking**: Every interaction generates structured events with full traceability
- 🗄️ **SQLite Storage**: Persistent event storage with optimized performance (WAL, indexes)
- 📝 **Structured Logging**: Rich context logging with Zap (configurable JSON/console output)
- 🌐 **HTTP API**: RESTful endpoints for event access and debugging
- 🔍 **Audio Fingerprinting**: SHA-256 hashing for deduplication and analysis
- ⏱️ **Performance Metrics**: Processing time tracking throughout the voice pipeline
- 🚨 **Error Tracking**: Comprehensive error state capture and reporting

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

## Architecture

The Hub service acts as the central nervous system of the Loqa platform, orchestrating the flow from voice input to actionable commands. With Milestone 2, all voice interactions are now fully traceable with structured events stored in SQLite and accessible via HTTP API.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOQA_HUB_PORT` | `3000` | HTTP server port |
| `LOQA_GRPC_PORT` | `50051` | gRPC server port |
| `DB_PATH` | `./data/loqa-hub.db` | SQLite database file location |
| `LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `LOG_FORMAT` | `console` | Log output format (json, console) |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama API endpoint |
| `OLLAMA_MODEL` | `llama3.2:3b` | Ollama model for intent parsing |
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |

### API Access

The Hub exposes RESTful APIs for voice events and skill management:

**Voice Events API:**
- `GET /api/voice-events` - List events with pagination and filtering
- `GET /api/voice-events/{uuid}` - Get specific event details
- `POST /api/voice-events` - Create events (testing/integrations)

**Skills Management API:**
- `GET /api/skills` - List all loaded skills with status
- `GET /api/skills/{id}` - Get detailed skill information
- `POST /api/skills` - Load a new skill from path
- `DELETE /api/skills/{id}` - Unload a skill
- `POST /api/skills/{id}/enable` - Enable a skill
- `POST /api/skills/{id}/disable` - Disable a skill
- `POST /api/skills/{id}/reload` - Reload a skill

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
