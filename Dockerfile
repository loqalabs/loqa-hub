# Go builder stage  
FROM golang:1.22.2-bookworm AS go-builder

# Accept build arguments for platform information
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum for loqa-hub
COPY go.mod go.sum ./

# Download go modules
WORKDIR /app
RUN go mod download

# Copy the rest of the source files
COPY . ./

# Build the hub service
RUN go build -v -o loqa-hub ./cmd

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates libgomp libstdc++

# Create app directory
WORKDIR /app

# Copy binary
COPY --from=go-builder /app/loqa-hub .

# Set default environment variables
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