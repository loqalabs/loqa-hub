# 🧠 Loqa Hub

[![CI/CD Pipeline](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml/badge.svg)](https://github.com/loqalabs/loqa-hub/actions/workflows/ci.yml)

Central orchestrator for the Loqa local-first voice assistant platform.

## Overview

Loqa Hub is the core service that handles:
- gRPC API for audio input from pucks
- Speech-to-text processing via Whisper.cpp
- LLM-based intent parsing and command extraction
- NATS integration for publishing commands to other services

## Features

- 🎤 **Audio Processing**: Receives audio streams from puck devices via gRPC
- 📝 **Speech Recognition**: Local speech-to-text using Whisper.cpp
- 🤖 **Intent Parsing**: Natural language understanding via Ollama LLM
- 📡 **Event Publishing**: Publishes parsed commands to NATS message bus
- 🔒 **Privacy-First**: All processing happens locally, no cloud dependencies

## Architecture

The Hub service acts as the central nervous system of the Loqa platform, orchestrating the flow from voice input to actionable commands.

## Getting Started

See the main [Loqa documentation](https://github.com/loqalabs/loqa) for setup and usage instructions.

## License

Licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.