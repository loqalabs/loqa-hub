FROM golang:1.21-alpine AS whisper-builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    build-base \
    cmake \
    pkgconfig

# Build whisper.cpp from source
WORKDIR /tmp
RUN git clone https://github.com/ggerganov/whisper.cpp.git
WORKDIR /tmp/whisper.cpp
RUN make

# Install headers and libraries
RUN mkdir -p /tmp/whisper.cpp/include && \
    cp *.h /tmp/whisper.cpp/include/ && \
    cp ggml/include/*.h /tmp/whisper.cpp/include/ || true

# Go builder stage
FROM golang:1.21-alpine AS go-builder

# Install basic tools
RUN apk add --no-cache git build-base

# Set CGO flags for whisper.cpp
ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-I/tmp/whisper.cpp/include"
ENV CGO_LDFLAGS="-L/tmp/whisper.cpp -lwhisper -lm -lstdc++"

# Copy whisper.cpp from builder
COPY --from=whisper-builder /tmp/whisper.cpp /tmp/whisper.cpp

# Set working directory
WORKDIR /app

# Copy proto module first (needed for local replace)
COPY loqa-proto ./loqa-proto

# Copy loqa-hub 
COPY loqa-hub ./loqa-hub
WORKDIR /app/loqa-hub

# Download go modules
RUN go mod download

# Copy is already done above

# Build the hub service
RUN go build -o loqa-hub ./cmd

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates libgomp libstdc++

# Create app directory
WORKDIR /app

# Copy binary
COPY --from=go-builder /app/loqa-hub .

# Copy whisper.cpp libraries (if they exist)
COPY --from=whisper-builder /tmp/whisper.cpp/libwhisper.* /usr/local/lib/

# Set library path
ENV LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH

# Set default environment variables
ENV WHISPER_MODEL_PATH=/models/ggml-tiny.bin
ENV WHISPER_LANGUAGE=en
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