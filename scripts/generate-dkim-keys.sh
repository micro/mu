#!/bin/bash
# Generate DKIM keys for mu

set -e

# Configuration
DOMAIN="${1:-localhost}"
SELECTOR="${2:-default}"
KEYS_DIR="$HOME/.mu/keys"
PRIVATE_KEY="$KEYS_DIR/dkim.key"
PUBLIC_KEY="$KEYS_DIR/dkim.pub"

echo "Generating DKIM keys for domain: $DOMAIN"
echo "Selector: $SELECTOR"
echo ""

# Create keys directory
mkdir -p "$KEYS_DIR"
chmod 700 "$KEYS_DIR"

# Generate private key (2048-bit)
echo "Generating private key..."
openssl genrsa -out "$PRIVATE_KEY" 2048
chmod 600 "$PRIVATE_KEY"

# Extract public key
echo "Extracting public key..."
openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY"
chmod 644 "$PUBLIC_KEY"

# Extract public key in DNS format (single line, no headers)
echo ""
echo "=================================================="
echo "DKIM keys generated successfully!"
echo "=================================================="
echo ""
echo "Private key: $PRIVATE_KEY"
echo "Public key:  $PUBLIC_KEY"
echo ""
echo "=================================================="
echo "Add this DNS TXT record:"
echo "=================================================="
echo ""
echo "Name: ${SELECTOR}._domainkey.${DOMAIN}"
echo "Type: TXT"
echo "Value:"
echo ""

# Extract and format the public key for DNS
PUB_KEY=$(openssl rsa -in "$PRIVATE_KEY" -pubout -outform PEM 2>/dev/null | \
  grep -v "PUBLIC KEY" | tr -d '\n')

echo "v=DKIM1; k=rsa; p=${PUB_KEY}"

echo ""
echo "=================================================="
echo "To verify DNS record after adding:"
echo "=================================================="
echo ""
echo "dig TXT ${SELECTOR}._domainkey.${DOMAIN}"
echo ""
echo "=================================================="
echo "Environment variables (optional):"
echo "=================================================="
echo ""
echo "export DKIM_DOMAIN=\"${DOMAIN}\""
echo "export DKIM_SELECTOR=\"${SELECTOR}\""
echo ""
echo "DKIM will be automatically enabled when you start mu."
echo ""
