#!/bin/bash

# Script to install git hooks that automatically remove Claude Code AI attributions
# This should be run in each repository in the Loqa ecosystem

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.git/hooks"

if [ ! -d "$HOOKS_DIR" ]; then
    echo "Error: Not in a git repository or .git/hooks directory not found"
    exit 1
fi

echo "Installing git hooks to remove Claude Code AI attributions..."

# Install prepare-commit-msg hook
cat > "$HOOKS_DIR/prepare-commit-msg" << 'EOF'
#!/bin/sh

# Git hook to automatically remove Claude Code AI attributions from commit messages
# This runs before the commit message editor opens

COMMIT_MSG_FILE=$1
COMMIT_SOURCE=$2
SHA1=$3

if [ -f "$COMMIT_MSG_FILE" ]; then
    # Create a temporary file
    TEMP_FILE=$(mktemp)
    
    # Remove Claude Code attribution lines
    grep -v "🤖 Generated with \[Claude Code\]" "$COMMIT_MSG_FILE" | \
    grep -v "Co-Authored-By: Claude <noreply@anthropic.com>" > "$TEMP_FILE"
    
    # Replace original file
    mv "$TEMP_FILE" "$COMMIT_MSG_FILE"
fi
EOF

# Install commit-msg hook as fallback
cat > "$HOOKS_DIR/commit-msg" << 'EOF'
#!/bin/sh

# Git hook to clean up commit messages after they're written
# This is a fallback in case prepare-commit-msg didn't catch everything

COMMIT_MSG_FILE=$1

if [ -f "$COMMIT_MSG_FILE" ]; then
    # Create a temporary file
    TEMP_FILE=$(mktemp)
    
    # Remove Claude Code attribution lines
    grep -v "🤖 Generated with \[Claude Code\]" "$COMMIT_MSG_FILE" | \
    grep -v "Co-Authored-By: Claude <noreply@anthropic.com>" > "$TEMP_FILE"
    
    # Replace original file
    mv "$TEMP_FILE" "$COMMIT_MSG_FILE"
fi
EOF

# Make hooks executable
chmod +x "$HOOKS_DIR/prepare-commit-msg"
chmod +x "$HOOKS_DIR/commit-msg"

echo "✅ Git hooks installed successfully!"
echo "   - prepare-commit-msg: Removes AI attribution before commit editor opens"
echo "   - commit-msg: Fallback to clean up any remaining attributions"
echo ""
echo "These hooks will automatically remove Claude Code AI attributions from commit messages."