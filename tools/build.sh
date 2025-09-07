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

# No external dependencies needed for STT REST client
export CGO_ENABLED=0
echo "🧠 Building with REST-based STT client"

# Build protobuf module
echo "📦 Building protobuf module..."
cd proto/go
go mod tidy

# Build hub service
echo "🏢 Building hub service..."
cd ../../hub
go mod tidy
go build -o ../bin/loqa-hub ./cmd

# Build test relay (if needed for testing)
echo "🎤 Building test relay..."
cd ../relay/test-go
go mod tidy
go build -o ../../bin/test-relay ./cmd

echo "✅ Build complete!"
echo ""
echo "🐳 Run services in Docker: docker-compose up -d"
echo "🏃 Or run hub locally: ./bin/loqa-hub"
echo "🎤 Run test relay: ./bin/test-relay"