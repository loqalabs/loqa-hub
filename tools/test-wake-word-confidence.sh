#!/bin/bash
# Test script for STT confidence thresholds and wake word normalization
#
# This file is part of Loqa (https://github.com/loqalabs/loqa).
# Copyright (C) 2025 Loqa Labs
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU Affero General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

echo "🧪 Testing STT Confidence Thresholds & Wake Word Normalization"
echo "============================================================="
echo

# Test the STT confidence and wake word logic with unit tests
echo "📋 Running Unit Tests..."
echo "========================"
go test -v -run "TestPostProcessTranscription|TestEstimateConfidence" ./internal/llm/

echo
echo "✅ Unit Tests Completed!"
echo

# Test examples
echo "📖 Wake Word Processing Examples:"
echo "================================="

cat << 'EOF'

Input: "Hey Loqa turn on the lights"
Expected Output:
- Wake Word Detected: true
- Wake Word Variant: "hey loqa"
- Cleaned Text: "turn on the lights"
- Confidence: High (>0.6)
- Needs Confirmation: false

Input: "Hey Luca turn off the music"
Expected Output:
- Wake Word Detected: true
- Wake Word Variant: "hey luca"
- Cleaned Text: "turn off the music"
- Confidence: High (>0.6)
- Needs Confirmation: false

Input: "Hey Loqa"
Expected Output:
- Wake Word Detected: true
- Wake Word Variant: "hey loqa"
- Cleaned Text: ""
- Confidence: High (>0.6)
- Needs Confirmation: true (empty command)

Input: "???"
Expected Output:
- Wake Word Detected: false
- Wake Word Variant: ""
- Cleaned Text: "???"
- Confidence: Low (<0.6)
- Needs Confirmation: true

Input: "on"
Expected Output:
- Wake Word Detected: false
- Wake Word Variant: ""
- Cleaned Text: "on"
- Confidence: Low (<0.6)
- Needs Confirmation: true

EOF

echo
echo "🔧 Manual Testing Instructions:"
echo "==============================="
echo "1. Start the voice pipeline: cd ../.. && docker-compose up -d"
echo "2. Start test relay: cd ../loqa-relay && go run ./test-go/cmd --hub localhost:50051"
echo "3. Test wake word variations:"
echo "   - Say: 'Hey Loqa turn on lights'"
echo "   - Say: 'Hey Luca turn off music'"
echo "   - Say: 'Hey Luka what time is it'"
echo "   - Say: 'Hey Loqa' (just wake word)"
echo "   - Whisper something unclear"
echo "4. Check hub logs for confidence scores and wake word detection"
echo
echo "Expected behavior:"
echo "- Wake words should be stripped from commands sent to LLM"
echo "- Low confidence should trigger confirmation prompts"
echo "- Wake word variants should be normalized properly"
echo
echo "🎯 Success Criteria:"
echo "- All unit tests pass ✓"
echo "- Wake words properly stripped from transcriptions ✓"
echo "- Common misspellings (Luca, Luka) handled ✓"
echo "- Low confidence triggers confirmation prompts ✓"
echo "- System gracefully handles edge cases ✓"