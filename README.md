[![Sponsor](https://img.shields.io/badge/Sponsor-Loqa-ff69b4?logo=githubsponsors&style=for-the-badge)](https://github.com/sponsors/annabarnes1138)
[![Ko-fi](https://img.shields.io/badge/Buy%20me%20a%20coffee-Ko--fi-FF5E5B?logo=ko-fi&logoColor=white&style=for-the-badge)](https://ko-fi.com/annabarnes)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL--3.0-blue?style=for-the-badge)](LICENSE)
[![Made with ❤️ by LoqaLabs](https://img.shields.io/badge/Made%20with%20%E2%9D%A4%EF%B8%8F-by%20LoqaLabs-ffb6c1?style=for-the-badge)](https://loqalabs.com)

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

Licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details.