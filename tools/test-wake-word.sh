#!/bin/bash

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