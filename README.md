[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by Loqa Labs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20Loqa Labs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

# 🧠 Loqa Hub

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml)

Central orchestrator for the Loqa local-first voice assistant platform.

## Overview

Loqa Hub is the core service that handles:
- gRPC API for audio input from pucks
- Speech-to-text processing via Whisper.cpp
- LLM-based intent parsing and command extraction
- NATS integration for publishing commands to other services
- **NEW:** Complete voice event tracking and observability

## Features

- 🎤 **Audio Processing**: Receives audio streams from puck devices via gRPC
- 📝 **Speech Recognition**: Local speech-to-text using Whisper.cpp
- 🤖 **Intent Parsing**: Natural language understanding via Ollama LLM
- 📡 **Event Publishing**: Publishes parsed commands to NATS message bus
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

## License

Licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details.