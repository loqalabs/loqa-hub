# CLAUDE.md - Loqa Hub Service

This file provides Claude Code with specific guidance for working with the Loqa Hub service - the central orchestrator of the microservice ecosystem.

## Service Overview

Loqa Hub is the core backend service that handles:
- **Audio Processing**: Receives audio streams from relay devices via gRPC
- **Speech Recognition**: Coordinates with OpenAI-compatible STT service
- **Intent Parsing**: Uses Ollama LLM for natural language understanding
- **Multi-Command Processing**: Parses and executes compound utterances
- **Event Publishing**: Publishes commands to NATS message bus
- **Skills Management**: Loads and manages skill plugins
- **Voice Event Tracking**: Complete observability for voice interactions

## Architecture Role

- **Service Type**: Core backend service (Go)
- **Dependencies**: loqa-proto (gRPC definitions), loqa-skills (plugin system)
- **Communicates With**: loqa-relay (audio input), loqa-commander (web UI), loqa-device-service (commands)
- **Ports**: `:3000` (HTTP API), `:50051` (gRPC server)

## Development Commands

### Local Development
```bash
# Build the service
go build -o bin/hub ./cmd

# Run with development settings
go run ./cmd

# Run tests
go test ./...

# Run specific test suites
go test ./internal/llm -v                    # LLM integration tests
go test ./internal/api -v                    # HTTP API tests
go test ./internal/skills -v                 # Skills system tests
go test ./tests/integration -v               # Integration tests
go test ./tests/e2e -v                      # End-to-end tests

# Development with hot reload
air                                          # Requires air tool
```

### Build & Release
```bash
# Production build
make build

# Cross-platform builds
make build-linux
make build-darwin
make build-windows

# Docker image
docker build -t loqa-hub .
```

### Testing & Quality
```bash
# Run all tests with coverage
make test-coverage

# Linting (matches CI)
make lint

# Security scanning
make security-scan

# Pre-commit checks (recommended)
make pre-commit
```

## API Endpoints

### HTTP API (Port 3000)
```bash
# Health check
curl http://localhost:3000/health

# Voice events
curl http://localhost:3000/api/events

# Skills management
curl http://localhost:3000/api/skills
curl -X POST http://localhost:3000/api/skills -d '{"skill_path": "/path/to/skill"}'
curl -X POST http://localhost:3000/api/skills/{id}/enable
```

### gRPC API (Port 50051)
```bash
# Test audio streaming (requires grpcurl)
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe audio.AudioService
```

## Skills Management

### Skills CLI
```bash
# List all loaded skills
go run ./cmd/skills-cli --action list

# Load a new skill
go run ./cmd/skills-cli --action load --path /path/to/skill

# Enable/disable skills
go run ./cmd/skills-cli --action enable --skill skill-id
go run ./cmd/skills-cli --action disable --skill skill-id

# Reload a skill
go run ./cmd/skills-cli --action reload --skill skill-id

# Get detailed skill info
go run ./cmd/skills-cli --action info --skill skill-id
```

### Skills Development
```bash
# Skills are loaded from ../loqa-skills/ by default
# See ../loqa-skills/CLAUDE.md for skill development workflow
```

## Configuration

### Environment Variables
```bash
# STT Configuration
export STT_URL=http://localhost:8000
export STT_MODEL=whisper-1

# TTS Configuration  
export TTS_URL=http://localhost:8880/v1
export TTS_VOICE=af_bella
export TTS_SPEED=1.0

# LLM Configuration
export OLLAMA_URL=http://localhost:11434
export OLLAMA_MODEL=llama3.2

# Message Bus
export NATS_URL=nats://localhost:4222

# Server Configuration
export LOQA_PORT=3000
export LOQA_GRPC_PORT=50051
export LOG_LEVEL=info
```

### Database
```bash
# SQLite database (auto-created)
./data/loqa.db

# View database schema
sqlite3 ./data/loqa.db ".schema"

# Query voice events
sqlite3 ./data/loqa.db "SELECT * FROM voice_events ORDER BY timestamp DESC LIMIT 5;"
```

## Debugging & Troubleshooting

### Common Issues
```bash
# Check service dependencies
curl http://localhost:8000/health     # STT service
curl http://localhost:8880/health     # TTS service  
curl http://localhost:11434/api/tags  # Ollama LLM

# View logs with structured format
go run ./cmd --log-format=json | jq

# Enable debug logging
go run ./cmd --log-level=debug

# Test gRPC connectivity
grpcurl -plaintext localhost:50051 audio.AudioService/Health
```

### Performance Monitoring
```bash
# Enable profiling
go run ./cmd --pprof-port=6060

# View profiles
go tool pprof http://localhost:6060/debug/pprof/profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Testing Strategies

### Unit Tests
- Test all business logic in `internal/` packages
- Mock external dependencies (STT, TTS, LLM, NATS)
- Focus on intent parsing, command queue, and skills management

### Integration Tests  
- Test real service interactions with running dependencies
- Audio processing pipeline end-to-end
- Database operations and event storage

### End-to-End Tests
- Complete voice workflow: audio → transcription → intent → response
- Multi-command processing and rollback scenarios
- Skills loading, execution, and error handling

## Related Documentation

- **Master Documentation**: `../loqa/config/CLAUDE.md` - Full ecosystem overview
- **Protocol Definitions**: `../loqa-proto/CLAUDE.md` - gRPC contracts and generation  
- **Skills Development**: `../loqa-skills/CLAUDE.md` - Plugin development workflow
- **Frontend Integration**: `../loqa-commander/CLAUDE.md` - Web UI and API integration