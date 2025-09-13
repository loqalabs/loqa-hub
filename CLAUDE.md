# Development Guidance

See [/loqalabs/CLAUDE.md](../CLAUDE.md) for complete development workflow guidance.

## Service Context

**loqa-hub** - Central orchestrator service with STT/TTS/LLM pipeline (Go)

- **Role**: Core service handling voice processing, skills coordination, and API endpoints
- **Quality Gates**: `make quality-check` (includes go fmt, go vet, golangci-lint, tests, docker build)
- **Development**: `go run ./cmd` (dev), `go build -o bin/hub ./cmd` (build)
- **Dependencies**: loqa-proto (gRPC definitions), loqa-skills (plugin system)

All workflow rules and development guidance are provided automatically by the MCP server based on repository detection.