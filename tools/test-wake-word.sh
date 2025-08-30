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


# Test script for wake word functionality
echo "🧪 Testing LOQA Wake Word Detection"
echo "=================================="
echo

# Start services in background
echo "🚀 Starting LOQA Services..."
docker-compose up -d

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Check if services are running
if ! docker-compose ps | grep -q "Up"; then
    echo "❌ Services failed to start"
    docker-compose logs
    exit 1
fi

echo "✅ Services are running!"
echo

# Instructions for manual testing
echo "📋 Manual Test Instructions:"
echo "1. Open a new terminal window"
echo "2. Navigate to: $(pwd)/puck/test-go/"
echo "3. Run: ./test-puck --hub localhost:50051"
echo "4. Wait for 'Wake Word: \"Hey Loqa\" (enabled)' message"
echo "5. Say: 'Hey Loqa, turn on the lights'"
echo "6. Observe wake word detection logs"
echo
echo "🔍 To view hub logs: docker-compose logs -f"
echo "🛑 To stop test: docker-compose down"
echo

# Keep script running to monitor
echo "Press Ctrl+C to stop the test environment..."
docker-compose logs -f