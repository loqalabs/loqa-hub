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


# Build script for Loqa Voice Assistant

set -e

echo "🔧 Building Loqa Voice Assistant..."

# Set whisper.cpp library paths (if available)
if [ -d "/tmp/whisper.cpp" ]; then
    export CGO_ENABLED=1
    export CGO_CFLAGS="-I/tmp/whisper.cpp/include"
    export CGO_LDFLAGS="-L/tmp/whisper.cpp -lwhisper -lm -lstdc++"
    echo "🧠 Using local Whisper.cpp installation"
else
    echo "⚠️  Whisper.cpp not found at /tmp/whisper.cpp - build may fail"
    echo "   Run Docker build for full Whisper integration"
fi

# Build protobuf module
echo "📦 Building protobuf module..."
cd proto/go
go mod tidy

# Build hub service
echo "🏢 Building hub service..."
cd ../../hub
go mod tidy
go build -o ../bin/loqa-hub ./cmd

# Build device service
echo "🔧 Building device service..."
go build -o ../bin/device-service ./cmd/device-service

# Build test puck (if needed for testing)
echo "🎤 Building test puck..."
cd ../puck/test-go
go mod tidy
go build -o ../../bin/test-puck ./cmd

echo "✅ Build complete!"
echo ""
echo "🐳 Run services in Docker: docker-compose up -d"
echo "🏃 Or run hub locally: DYLD_LIBRARY_PATH=/tmp/whisper.cpp/build/src:/tmp/whisper.cpp/build/ggml/src:/tmp/whisper.cpp/build/ggml/src/ggml-metal:/tmp/whisper.cpp/build/ggml/src/ggml-blas ./bin/loqa-hub"
echo "🎤 Run test puck: ./bin/test-puck"