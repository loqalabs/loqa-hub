# Go builder stage
FROM --platform=$BUILDPLATFORM golang:1.25.1-alpine AS go-builder

# Accept build arguments for cross-compilation
ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

# Install basic dependencies for Go module download
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better layer caching
COPY loqa-hub/go.mod loqa-hub/go.sum ./loqa-hub/
WORKDIR /app/loqa-hub

# Download go modules (cached independently of source changes)
RUN go mod download

# Copy source code
COPY loqa-hub/ ./

# Build the hub service as static binary with proper cross-compilation
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH:-amd64}

RUN echo "Building for GOOS=${GOOS} GOARCH=${GOARCH}" && \
    go build -v -ldflags="-w -s" -o loqa-hub ./cmd

# Runtime stage - use platform-specific base image
FROM --platform=$TARGETPLATFORM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create app directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=go-builder /app/loqa-hub/loqa-hub .

# Set default environment variables for HTTP/1.1 streaming architecture
ENV STT_URL=http://stt:8000
ENV TTS_URL=http://tts:8880
ENV OLLAMA_URL=http://ollama:11434
ENV OLLAMA_MODEL=llama3.2:3b
ENV NATS_URL=nats://nats:4222
ENV LOQA_HOST=0.0.0.0
ENV LOQA_PORT=3000
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Expose HTTP port for streaming and API
EXPOSE 3000

# Run the hub service
CMD ["./loqa-hub"]