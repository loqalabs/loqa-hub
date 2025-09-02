#!/bin/bash

# This file is part of Loqa (https://github.com/loqalabs/loqa).
# Copyright (C) 2025 Loqa Labs
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU Affero General Public License for more details.
#
# You should have received a copy of the GNU Affero General Public License
# along with this program. If not, see <https://www.gnu.org/licenses/>.


# Setup script for Whisper.cpp local development (fallback mode)
# NOTE: Loqa now uses faster-whisper gRPC service by default (USE_GRPC_WHISPER=true)
# This script sets up whisper.cpp as a fallback option for development

set -e

echo "🧠 Setting up Whisper.cpp for fallback development..."
echo "ℹ️  Note: Loqa uses faster-whisper gRPC service by default"

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
echo "🔧 To use whisper.cpp instead of gRPC service:"
echo "   export USE_GRPC_WHISPER=false"
echo "   export CGO_CFLAGS=\"-I/tmp/whisper.cpp/include\""
echo "   export CGO_LDFLAGS=\"-L/tmp/whisper.cpp -lwhisper -lm -lstdc++\""
echo ""
echo "💡 For production use, keep USE_GRPC_WHISPER=true (default)"