#!/bin/bash

# Setup script for Whisper.cpp local development

set -e

echo "🧠 Setting up Whisper.cpp for local development..."

# Check if already exists
if [ -d "/tmp/whisper.cpp" ]; then
    echo "✅ Whisper.cpp already exists at /tmp/whisper.cpp"
    exit 0
fi

# Clone and build whisper.cpp
echo "📥 Cloning Whisper.cpp..."
cd /tmp
git clone https://github.com/ggerganov/whisper.cpp.git

echo "🔨 Building Whisper.cpp..."
cd whisper.cpp
make clean && make

# Set up headers
echo "📋 Setting up headers..."
mkdir -p include
cp *.h include/ 2>/dev/null || true
cp ggml/include/*.h include/ 2>/dev/null || true

# Download model
echo "📥 Downloading base English model..."
mkdir -p models
if [ ! -f "models/ggml-base.en.bin" ]; then
    curl -L -o models/ggml-base.en.bin \
        https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin
fi

echo "✅ Whisper.cpp setup complete!"
echo ""
echo "🏃 You can now run: ./tools/build.sh"
echo "🔧 Or set environment variables:"
echo "   export CGO_CFLAGS=\"-I/tmp/whisper.cpp/include\""
echo "   export CGO_LDFLAGS=\"-L/tmp/whisper.cpp -lwhisper -lm -lstdc++\""