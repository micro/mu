# Mu Android App

Native Android app with built-in WireGuard VPN.

## Features

- WebView pointing to mu.xyz
- Built-in WireGuard VPN client
- Floating button to toggle VPN on/off
- All traffic routes through your server when enabled

## Building

1. Open this folder in Android Studio
2. Update `SERVER_PUBLIC_KEY` and `SERVER_ENDPOINT` in `MainActivity.kt`
3. Build → Generate Signed APK

## Server Setup

On your server (mu.xyz), set up WireGuard:

```bash
# Install WireGuard
sudo apt install wireguard

# Generate server keys (or use the ones from /vpn page)
wg genkey | tee /etc/wireguard/server.key | wg pubkey > /etc/wireguard/server.pub

# Create config
sudo cat > /etc/wireguard/wg0.conf << EOF
[Interface]
Address = 10.66.66.1/24
PrivateKey = $(cat /etc/wireguard/server.key)
ListenPort = 51820
PostUp = iptables -A FORWARD -i wg0 -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i wg0 -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE

# Add peers as they connect (or pre-configure)
EOF

# Enable IP forwarding
echo 'net.ipv4.ip_forward=1' | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# Start WireGuard
sudo systemctl enable wg-quick@wg0
sudo systemctl start wg-quick@wg0
```

## How It Works

1. App generates a WireGuard keypair on first launch
2. User taps VPN button → Android asks for VPN permission
3. WireGuard tunnel connects to your server
4. All device traffic routes through the tunnel
5. Traffic exits from your UK server

## TODO

- [ ] Auto-fetch server config from mu.xyz/vpn/config
- [ ] Register client public key with server automatically
- [ ] Show connection status in the WebView
- [ ] iOS version using NetworkExtension framework
