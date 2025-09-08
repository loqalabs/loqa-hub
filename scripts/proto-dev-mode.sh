#!/bin/bash

# Proto Development Mode Toggle Script
# This script toggles between development (local proto) and production (released proto) modes

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GO_MOD="$ROOT_DIR/go.mod"

print_usage() {
    echo "Usage: $0 [dev|prod|status]"
    echo ""
    echo "Commands:"
    echo "  dev     - Enable development mode (use local proto changes)"
    echo "  prod    - Enable production mode (use released proto version)"
    echo "  status  - Show current mode"
    echo ""
    echo "Development mode allows testing proto changes before they're released."
}

check_proto_dir() {
    local proto_path="$ROOT_DIR/../loqa-proto/go"
    if [[ ! -d "$proto_path" ]]; then
        echo "❌ Error: loqa-proto directory not found at $proto_path"
        echo "   Make sure loqa-proto is cloned in the same parent directory"
        exit 1
    fi
}

enable_dev_mode() {
    check_proto_dir
    
    if grep -q "^replace github.com/loqalabs/loqa-proto/go" "$GO_MOD"; then
        echo "✅ Development mode already enabled"
        return
    fi
    
    # Uncomment the replace directive
    sed -i.bak 's|^// replace github.com/loqalabs/loqa-proto/go|replace github.com/loqalabs/loqa-proto/go|g' "$GO_MOD"
    
    # Run go mod tidy to update dependencies
    echo "🔄 Updating dependencies..."
    cd "$ROOT_DIR"
    go mod tidy
    
    echo "✅ Development mode enabled - using local proto changes"
    echo "   📁 Proto path: ../loqa-proto/go"
}

enable_prod_mode() {
    if grep -q "^// replace github.com/loqalabs/loqa-proto/go" "$GO_MOD"; then
        echo "✅ Production mode already enabled"
        return
    fi
    
    # Comment out the replace directive
    sed -i.bak 's|^replace github.com/loqalabs/loqa-proto/go|// replace github.com/loqalabs/loqa-proto/go|g' "$GO_MOD"
    
    # Run go mod tidy to update dependencies
    echo "🔄 Updating dependencies..."
    cd "$ROOT_DIR"
    go mod tidy
    
    echo "✅ Production mode enabled - using released proto version"
    echo "   📦 Using version from go.mod require statement"
}

show_status() {
    if grep -q "^replace github.com/loqalabs/loqa-proto/go" "$GO_MOD"; then
        echo "📍 Current mode: Development"
        echo "   📁 Using local proto changes from ../loqa-proto/go"
    elif grep -q "^// replace github.com/loqalabs/loqa-proto/go" "$GO_MOD"; then
        echo "📍 Current mode: Production"
        echo "   📦 Using released proto version from go.mod"
    else
        echo "⚠️  Current mode: Unknown (no replace directive found)"
        echo "   📦 Using released proto version from go.mod"
    fi
}

# Main script logic
case "${1:-}" in
    "dev")
        enable_dev_mode
        ;;
    "prod")
        enable_prod_mode
        ;;
    "status")
        show_status
        ;;
    *)
        print_usage
        exit 1
        ;;
esac