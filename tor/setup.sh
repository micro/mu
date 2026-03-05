#!/bin/bash
set -e

# Install and configure Tor hidden service for mu
# Run this on the production server as root

echo "Installing Tor..."
apt-get update
apt-get install -y tor

echo "Configuring hidden service..."
cp torrc /etc/tor/torrc

# Ensure correct permissions
chown -R debian-tor:debian-tor /var/lib/tor/

echo "Restarting Tor..."
systemctl enable tor
systemctl restart tor

# Wait for hidden service to generate its .onion address
sleep 5

if [ -f /var/lib/tor/mu_hidden_service/hostname ]; then
    echo ""
    echo "Your .onion address:"
    cat /var/lib/tor/mu_hidden_service/hostname
    echo ""
    echo "Users can access mu at this address using Tor Browser."
    echo "Backup /var/lib/tor/mu_hidden_service/ to preserve this address."
else
    echo "Waiting for Tor to generate .onion address..."
    echo "Check: cat /var/lib/tor/mu_hidden_service/hostname"
fi
