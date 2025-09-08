# Go builder stage  
FROM golang:1.25.1-alpine AS go-builder

# Accept build arguments for platform information
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Install basic dependencies for Go module download
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy loqa-hub 
COPY loqa-hub ./loqa-hub
WORKDIR /app/loqa-hub

# Download go modules
RUN go mod download

# Build the hub service as static binary
ENV CGO_ENABLED=0
RUN go build -v -ldflags="-w -s" -o loqa-hub ./cmd

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create app directory
WORKDIR /app

# Copy binary
COPY --from=go-builder /app/loqa-hub/loqa-hub .

# Set default environment variables
ENV STT_URL=http://stt:8000
ENV STT_LANGUAGE=en
ENV OLLAMA_URL=http://ollama:11434
ENV OLLAMA_MODEL=llama3.2:3b
ENV NATS_URL=nats://nats:4222
ENV NATS_SUBJECT=loqa.commands
ENV LOQA_HOST=0.0.0.0
ENV LOQA_PORT=3000
ENV LOQA_GRPC_PORT=50051
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json

# Expose ports
EXPOSE 3000 50051

# Run the hub service
CMD ["./loqa-hub"]