#!/bin/sh
# Mu installer — downloads and runs Mu with one command.
# Usage: curl -fsSL https://micro.mu/install | sh
set -e

REPO="micro/mu"
INSTALL_DIR="${MU_INSTALL_DIR:-$HOME/.mu}"
BIN_DIR="${MU_BIN_DIR:-$HOME/.local/bin}"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Installing Mu for ${OS}/${ARCH}..."

# Check for Go
if command -v go >/dev/null 2>&1; then
  echo "Go found — building from source..."
  TMPDIR=$(mktemp -d)
  git clone --depth 1 https://github.com/${REPO}.git "$TMPDIR/mu" 2>/dev/null
  cd "$TMPDIR/mu"
  go build -o mu .
  mkdir -p "$BIN_DIR"
  mv mu "$BIN_DIR/mu"
  rm -rf "$TMPDIR"
else
  # Download prebuilt binary from GitHub releases
  echo "Downloading prebuilt binary..."
  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
  if [ -z "$LATEST" ]; then
    echo "No releases found. Install Go and run: go install github.com/${REPO}@latest"
    exit 1
  fi
  URL="https://github.com/${REPO}/releases/download/${LATEST}/mu-${OS}-${ARCH}"
  mkdir -p "$BIN_DIR"
  curl -fsSL "$URL" -o "$BIN_DIR/mu"
  chmod +x "$BIN_DIR/mu"
fi

# Create data directory
mkdir -p "$INSTALL_DIR"

# Add to PATH if needed
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
      zsh)  RC="$HOME/.zshrc" ;;
      bash) RC="$HOME/.bashrc" ;;
      *)    RC="" ;;
    esac
    if [ -n "$RC" ] && ! grep -q "$BIN_DIR" "$RC" 2>/dev/null; then
      echo "export PATH=\"$BIN_DIR:\$PATH\"" >> "$RC"
      echo "Added $BIN_DIR to PATH in $RC"
    fi
    export PATH="$BIN_DIR:$PATH"
    ;;
esac

echo ""
echo "✓ Mu installed to $BIN_DIR/mu"
echo ""
echo "Quick start:"
echo ""
echo "  # Run with Ollama (local, free)"
echo "  ollama serve &"
echo "  export OPENAI_BASE_URL=http://localhost:11434/v1"
echo "  export OPENAI_API_KEY=ollama"
echo "  mu --serve"
echo ""
echo "  # Or run with Claude"
echo "  export ANTHROPIC_API_KEY=your-key"
echo "  mu --serve"
echo ""
echo "  # Then open http://localhost:8081"
echo ""
