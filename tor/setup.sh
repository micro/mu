#!/bin/bash
set -e

# Install and configure Tor hidden service for mu
# Generates a vanity .onion address with prefix "muxyz"
# Run this on the production server as root

VANITY_PREFIX="muxyz"
HIDDEN_SERVICE_DIR="/var/lib/tor/mu_hidden_service"

echo "=== Installing dependencies ==="
apt-get update
apt-get install -y tor git autoconf libsodium-dev make gcc

echo "=== Building mkp224o (vanity .onion generator) ==="
WORKDIR=$(mktemp -d)
git clone https://github.com/cathugger/mkp224o "$WORKDIR/mkp224o"
cd "$WORKDIR/mkp224o"
./autogen.sh
./configure
make

echo "=== Generating vanity .onion address with prefix '$VANITY_PREFIX' ==="
echo "This may take a few minutes..."
./mkp224o "$VANITY_PREFIX" -n 1 -d "$WORKDIR/keys"

# Find the generated key directory
KEY_DIR=$(find "$WORKDIR/keys" -mindepth 1 -maxdepth 1 -type d | head -1)
if [ -z "$KEY_DIR" ]; then
    echo "ERROR: Failed to generate vanity address"
    exit 1
fi

ONION_ADDR=$(cat "$KEY_DIR/hostname")
echo "Generated address: $ONION_ADDR"

echo "=== Configuring hidden service ==="
# Stop tor before modifying hidden service dir
systemctl stop tor 2>/dev/null || true

# Set up hidden service directory with vanity keys
mkdir -p "$HIDDEN_SERVICE_DIR"
cp "$KEY_DIR/hs_ed25519_secret_key" "$HIDDEN_SERVICE_DIR/"
cp "$KEY_DIR/hs_ed25519_public_key" "$HIDDEN_SERVICE_DIR/"
cp "$KEY_DIR/hostname" "$HIDDEN_SERVICE_DIR/"

# Tor requires strict permissions
chown -R debian-tor:debian-tor "$HIDDEN_SERVICE_DIR"
chmod 700 "$HIDDEN_SERVICE_DIR"
chmod 600 "$HIDDEN_SERVICE_DIR/hs_ed25519_secret_key"
chmod 600 "$HIDDEN_SERVICE_DIR/hs_ed25519_public_key"

# Install torrc
cp "$(dirname "$0")/torrc" /etc/tor/torrc

echo "=== Starting Tor ==="
systemctl enable tor
systemctl restart tor

# Clean up build artifacts
rm -rf "$WORKDIR"

echo ""
echo "=== Setup complete ==="
echo "Your .onion address: $ONION_ADDR"
echo ""
echo "IMPORTANT: Back up these files to preserve your address:"
echo "  $HIDDEN_SERVICE_DIR/hs_ed25519_secret_key"
echo "  $HIDDEN_SERVICE_DIR/hs_ed25519_public_key"
echo "  $HIDDEN_SERVICE_DIR/hostname"
