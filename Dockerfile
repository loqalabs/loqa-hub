# Go builder stage  
FROM golang:1.24rc1-alpine AS go-builder

# Accept build arguments for platform information
ARG TARGETPLATFORM
ARG BUILDPLATFORM

# Install build dependencies for whisper.cpp
RUN apk add --no-cache git build-base cmake binutils gcc g++ musl-dev linux-headers binutils-gold

# Enable CGO for whisper.cpp integration
ENV CGO_ENABLED=1

# Clone and build whisper.cpp
WORKDIR /tmp
RUN git clone https://github.com/ggerganov/whisper.cpp.git
WORKDIR /tmp/whisper.cpp
RUN make build && \
    echo "=== Built libraries ===" && \
    find build -name "*.so" -exec ls -la {} \; && \
    echo "=== Creating lib directory with symlinks ===" && \
    mkdir -p lib && \
    ln -sf ../build/src/libwhisper.so* lib/ && \
    ln -sf ../build/ggml/src/libggml*.so lib/ && \
    ls -la lib/

# Set environment variables for whisper.cpp
ENV C_INCLUDE_PATH=/tmp/whisper.cpp/include:/tmp/whisper.cpp/ggml/include
ENV LIBRARY_PATH=/tmp/whisper.cpp/build/src:/tmp/whisper.cpp/build/ggml/src

# Set working directory
WORKDIR /app

# Copy proto module first (needed for local replace)
COPY loqa-proto ./loqa-proto

# Copy loqa-hub 
COPY loqa-hub ./loqa-hub
WORKDIR /app/loqa-hub

# Download go modules
RUN go mod download

# Build the hub service with whisper support
# Debug: Check for linker and build tools and architecture
RUN which gcc && which ld && which ar && uname -a && go env GOOS GOARCH

# Set up proper linker paths
ENV PATH="/usr/bin:$PATH"
ENV CC="gcc"
ENV CXX="g++"

# Disable cross-compilation for CGO builds
ENV GOOS=""
ENV GOARCH=""

# Disable gold linker and use traditional ld
ENV CGO_LDFLAGS="-fuse-ld=bfd -L/tmp/whisper.cpp/build/src -L/tmp/whisper.cpp/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lm -lstdc++ -fopenmp"
ENV CGO_CFLAGS="-I/tmp/whisper.cpp/include -I/tmp/whisper.cpp/ggml/include"

# Create symlink for ld if missing  
RUN ln -sf /usr/bin/ld.bfd /usr/bin/ld

# Try to build with whisper support, fallback to no-whisper build if it fails
RUN go build -v -x -tags whisper -o loqa-hub ./cmd || \
    (echo "⚠️  Whisper build failed, building without whisper support" && \
     go build -v -o loqa-hub ./cmd)

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates libgomp libstdc++

# Create app directory
WORKDIR /app

# Copy binary and whisper libraries
COPY --from=go-builder /app/loqa-hub/loqa-hub .
COPY --from=go-builder /tmp/whisper.cpp/build/src/libwhisper.so* /usr/local/lib/
COPY --from=go-builder /tmp/whisper.cpp/build/ggml/src/libggml.so /usr/local/lib/
COPY --from=go-builder /tmp/whisper.cpp/build/ggml/src/libggml-cpu.so /usr/local/lib/
COPY --from=go-builder /tmp/whisper.cpp/build/ggml/src/libggml-base.so /usr/local/lib/

# Create symlinks for versioned library names
RUN ln -sf libggml-base.so /usr/local/lib/libggml-base.so.1 && \
    ln -sf libggml.so /usr/local/lib/libggml.so.1 && \
    ln -sf libggml-cpu.so /usr/local/lib/libggml-cpu.so.1

# Set library path for whisper.cpp libraries
ENV LD_LIBRARY_PATH=/usr/local/lib

# Whisper.cpp integration enabled

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