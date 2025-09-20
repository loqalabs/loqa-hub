#!/bin/bash

# Coverage analysis script for loqa-hub
# This script provides comprehensive test coverage analysis

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔍 Running comprehensive test coverage analysis for loqa-hub${NC}"
echo "================================================================"

# Create coverage directory if it doesn't exist
mkdir -p coverage

# Run tests with coverage for all packages that have tests
echo -e "${YELLOW}📊 Generating coverage reports...${NC}"

# Get list of packages with tests
PACKAGES_WITH_TESTS=$(go list ./... | while read pkg; do
    pkg_path=$(echo "$pkg" | sed 's|github.com/loqalabs/loqa-hub||' | sed 's|^/||')
    if [ -z "$pkg_path" ]; then
        pkg_path="."
    fi
    if ls ${pkg_path}/*_test.go 2>/dev/null >/dev/null; then
        echo "$pkg"
    fi
done)

if [ -z "$PACKAGES_WITH_TESTS" ]; then
    echo -e "${RED}❌ No test files found${NC}"
    exit 1
fi

# Generate coverage for each package
for pkg in $PACKAGES_WITH_TESTS; do
    pkg_name=$(basename "$pkg")
    echo "  📁 Testing package: $pkg_name"

    # Run tests with coverage output
    go test -coverprofile="coverage/${pkg_name}.out" -covermode=atomic "$pkg" 2>/dev/null || {
        echo -e "${RED}    ❌ Tests failed for $pkg_name${NC}"
        continue
    }

    # Generate human-readable coverage report
    if [ -f "coverage/${pkg_name}.out" ]; then
        coverage_percent=$(go tool cover -func="coverage/${pkg_name}.out" | tail -1 | awk '{print $3}')
        echo -e "${GREEN}    ✅ Coverage: $coverage_percent${NC}"
    fi
done

# Merge all coverage files
echo -e "\n${YELLOW}📋 Generating combined coverage report...${NC}"

# Create combined coverage file
echo "mode: atomic" > coverage/combined.out

# Merge all individual coverage files
for file in coverage/*.out; do
    if [ "$(basename "$file")" != "combined.out" ]; then
        tail -n +2 "$file" >> coverage/combined.out 2>/dev/null || true
    fi
done

# Generate combined coverage report
if [ -f coverage/combined.out ]; then
    echo -e "\n${GREEN}📊 Combined Coverage Report:${NC}"
    echo "=================================="
    go tool cover -func=coverage/combined.out | tail -1

    # Generate HTML report
    go tool cover -html=coverage/combined.out -o coverage/coverage.html
    echo -e "${GREEN}🌐 HTML coverage report generated: coverage/coverage.html${NC}"

    # Extract overall percentage
    overall_coverage=$(go tool cover -func=coverage/combined.out | tail -1 | awk '{print $3}' | sed 's/%//')

    echo -e "\n${GREEN}📈 Coverage Summary:${NC}"
    echo "=================================="
    if (( $(echo "$overall_coverage >= 80" | bc -l) )); then
        echo -e "${GREEN}✅ Overall coverage: ${overall_coverage}% (Good)${NC}"
    elif (( $(echo "$overall_coverage >= 60" | bc -l) )); then
        echo -e "${YELLOW}⚠️  Overall coverage: ${overall_coverage}% (Acceptable)${NC}"
    else
        echo -e "${RED}❌ Overall coverage: ${overall_coverage}% (Needs improvement)${NC}"
    fi
fi

echo -e "\n${GREEN}✅ Coverage analysis complete!${NC}"
echo "Coverage files saved in: ./coverage/"