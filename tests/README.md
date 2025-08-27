# Loqa Voice Assistant Tests

This directory contains the test suites for the Loqa Voice Assistant system.

## Test Structure

### Integration Tests (`integration/`)
Tests individual components and their interactions:
- Hub gRPC service connectivity
- Audio streaming functionality
- Service communication patterns

### End-to-End Tests (`e2e/`)
Tests complete system workflows:
- Full voice pipeline from puck to hub
- Docker Compose service orchestration
- Real-world usage scenarios

## Running Tests

### Prerequisites
1. Build the project: `./tools/build.sh`
2. Ensure Docker and Docker Compose are installed
3. Have test audio files available (if needed)

### Integration Tests
```bash
cd tests/integration
go test -v ./...
```

### End-to-End Tests
```bash
cd tests/e2e
go test -v ./...

# Skip e2e tests in short mode
go test -short ./...
```

### All Tests
```bash
# From project root
go test ./tests/...
```

## Test Configuration

### Environment Variables
- `HUB_ADDRESS`: Hub service address (default: localhost:50051)
- `TEST_TIMEOUT`: Test timeout duration (default: 30s)
- `COMPOSE_FILE`: Docker Compose file path

### Test Data
- Test audio files should be placed in `testdata/`
- Audio format: 16kHz, mono, PCM
- Sample test phrases included for wake word testing

## CI/CD Integration

These tests are designed to run in CI environments:
- Integration tests run against mock services
- E2E tests require full Docker environment
- Tests clean up resources automatically

## Troubleshooting

### Common Issues
1. **Connection refused**: Ensure hub service is running
2. **Docker errors**: Check Docker daemon and permissions
3. **Timeout errors**: Increase test timeout for slower systems

### Debug Mode
```bash
# Run with verbose output
go test -v -args -debug ./tests/...

# View Docker Compose logs during tests
docker-compose logs -f
```