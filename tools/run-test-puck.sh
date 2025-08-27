#!/bin/bash

# Loqa Test Puck Runner
# Builds and runs the Go test puck for voice interaction testing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Loqa Test Puck Builder & Runner${NC}"
echo "======================================="

# Check if we're in the right directory
if [ ! -f "deployments/docker-compose.yml" ]; then
    echo -e "${RED}❌ Error: Please run this script from the loqa-voice-assistant root directory${NC}"
    exit 1
fi

# Default values
HUB_ADDRESS="localhost:50051"
PUCK_ID="test-puck-001"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --hub)
            HUB_ADDRESS="$2"
            shift 2
            ;;
        --id)
            PUCK_ID="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --hub ADDRESS    Hub gRPC address (default: localhost:50051)"
            echo "  --id ID          Puck identifier (default: test-puck-001)"
            echo "  -h, --help       Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                                    # Use defaults"
            echo "  $0 --hub localhost:50051 --id puck-1 # Custom settings"
            exit 0
            ;;
        *)
            echo -e "${RED}❌ Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo -e "${YELLOW}📋 Configuration:${NC}"
echo "  Hub Address: $HUB_ADDRESS"
echo "  Puck ID: $PUCK_ID"
echo ""

# Check if PortAudio is available
echo -e "${BLUE}🔍 Checking PortAudio dependency...${NC}"
if ! pkg-config --exists portaudio-2.0; then
    echo -e "${YELLOW}⚠️  PortAudio not found. Installing...${NC}"
    
    # Platform-specific installation
    if [[ "$OSTYPE" == "darwin"* ]]; then
        if command -v brew >/dev/null 2>&1; then
            echo "Installing PortAudio via Homebrew..."
            brew install portaudio
        else
            echo -e "${RED}❌ Homebrew not found. Please install PortAudio manually:${NC}"
            echo "  brew install portaudio"
            exit 1
        fi
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        echo "Please install PortAudio for your Linux distribution:"
        echo "  Ubuntu/Debian: sudo apt-get install portaudio19-dev"
        echo "  CentOS/RHEL:   sudo yum install portaudio-devel"
        echo "  Fedora:        sudo dnf install portaudio-devel"
        exit 1
    else
        echo -e "${RED}❌ Unsupported platform. Please install PortAudio manually.${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}✅ PortAudio is available${NC}"

# Build the test puck
echo -e "${BLUE}🔨 Building test puck...${NC}"
cd puck/test-go

# Clean and build
echo "Cleaning previous builds..."
rm -f test-puck

echo "Downloading Go dependencies..."
go mod tidy

echo "Building test puck binary..."
go build -o test-puck ./cmd

if [ ! -f "test-puck" ]; then
    echo -e "${RED}❌ Build failed${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Test puck built successfully${NC}"

# Check if hub is running
echo -e "${BLUE}🔍 Checking hub connectivity...${NC}"
if ! nc -z ${HUB_ADDRESS%:*} ${HUB_ADDRESS#*:} 2>/dev/null; then
    echo -e "${YELLOW}⚠️  Hub not reachable at $HUB_ADDRESS${NC}"
    echo "Make sure the hub service is running:"
    echo "  cd deployments && docker-compose up -d"
    echo ""
    echo "Starting puck anyway (will retry connection)..."
fi

# Run the test puck
echo -e "${BLUE}🎤 Starting test puck...${NC}"
echo ""
echo "==========================================="
echo -e "${GREEN}🎙️  Test puck is ready!${NC}"
echo "Say \"Hey Loqa\" followed by your command!"
echo "Press Ctrl+C to stop"
echo "==========================================="
echo ""

./test-puck --hub "$HUB_ADDRESS" --id "$PUCK_ID"