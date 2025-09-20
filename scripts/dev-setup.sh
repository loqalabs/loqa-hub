#!/bin/bash

# Development environment setup script for loqa-hub
# This script ensures all necessary tools are installed for development

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 Setting up loqa-hub development environment${NC}"
echo "=============================================="

# Check Go version
echo -e "${BLUE}📋 Checking Go version...${NC}"
go version

# Install development tools
echo -e "\n${YELLOW}🔧 Installing development tools...${NC}"
echo "  📦 golangci-lint (linting and static analysis)"
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

echo "  📊 Go coverage tools"
go install golang.org/x/tools/cmd/cover@latest

# Verify tools are installed
echo -e "\n${BLUE}✅ Verifying tool installations...${NC}"
if command -v golangci-lint &> /dev/null; then
    echo -e "${GREEN}  ✓ golangci-lint installed${NC}"
    golangci-lint version
else
    echo -e "${RED}  ✗ golangci-lint not found in PATH${NC}"
fi

if command -v cover &> /dev/null; then
    echo -e "${GREEN}  ✓ Go cover tool installed${NC}"
else
    echo -e "${RED}  ✗ Go cover tool not found in PATH${NC}"
fi

# Tidy dependencies
echo -e "\n${YELLOW}📦 Tidying Go modules...${NC}"
go mod tidy

# Run initial quality checks
echo -e "\n${YELLOW}🔍 Running initial quality checks...${NC}"
echo "  🧪 Running tests..."
go test ./... > /dev/null 2>&1 && echo -e "${GREEN}  ✓ All tests pass${NC}" || echo -e "${RED}  ✗ Some tests failed${NC}"

echo "  📊 Checking coverage..."
overall_coverage=$(go test -cover ./... 2>/dev/null | grep "coverage:" | awk '{sum += $5; count++} END {printf "%.1f", sum/count}' | sed 's/%//')
echo -e "${GREEN}  ✓ Overall coverage: ${overall_coverage}%${NC}"

echo "  🔧 Running linter..."
if golangci-lint run --fast > /dev/null 2>&1; then
    echo -e "${GREEN}  ✓ Linting passed${NC}"
else
    echo -e "${YELLOW}  ⚠ Linting issues found (run 'make lint' for details)${NC}"
fi

# Create coverage directory
echo -e "\n${YELLOW}📁 Setting up directories...${NC}"
mkdir -p coverage
echo -e "${GREEN}  ✓ Coverage directory created${NC}"

# Make scripts executable
echo -e "\n${YELLOW}🔧 Setting up scripts...${NC}"
chmod +x scripts/*.sh
echo -e "${GREEN}  ✓ Scripts made executable${NC}"

echo -e "\n${GREEN}🎉 Development environment setup complete!${NC}"
echo ""
echo "Available commands:"
echo "  make help           - Show all available commands"
echo "  make test           - Run tests"
echo "  make coverage       - Generate comprehensive coverage report"
echo "  make lint           - Run linting"
echo "  make quality-check  - Run all quality checks"
echo "  make build          - Build the application"
echo ""
echo "Documentation:"
echo "  COVERAGE.md         - Coverage analysis guide"
echo "  README.md           - Project overview"
echo ""
echo -e "${GREEN}Happy coding! 🚀${NC}"