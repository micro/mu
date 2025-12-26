#!/bin/bash
#
# Install git hooks for the mu project
#

HOOKS_DIR=".git/hooks"
SCRIPTS_DIR="$(dirname "$0")"

echo "Installing git hooks..."

# Install pre-commit hook
if [ -f "$SCRIPTS_DIR/pre-commit" ]; then
    cp "$SCRIPTS_DIR/pre-commit" "$HOOKS_DIR/pre-commit"
    chmod +x "$HOOKS_DIR/pre-commit"
    echo "✅ Installed pre-commit hook"
else
    echo "❌ Error: pre-commit script not found"
    exit 1
fi

echo ""
echo "✅ All hooks installed successfully!"
echo ""
echo "The pre-commit hook will:"
echo "  - Run tests before every commit"
echo "  - Prevent commits if tests fail"
echo ""
echo "To skip the hook temporarily, use: git commit --no-verify"
