package tunnel

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/curve25519"

	"mu/auth"
)

var vpnConfig *VPNConfig

type VPNConfig struct {
	ServerPrivateKey string
	ServerPublicKey  string
	ServerEndpoint   string
}

type ClientConfig struct {
	PrivateKey string
	PublicKey  string
	Address    string
}

func generateKeyPair() (privateKey, publicKey string, err error) {
	var priv [32]byte
	if _, err := rand.Read(priv[:]); err != nil {
		return "", "", err
	}
	
	// Clamp per WireGuard spec
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)
	
	return base64.StdEncoding.EncodeToString(priv[:]),
		base64.StdEncoding.EncodeToString(pub[:]), nil
}

func loadVPNConfig() error {
	if vpnConfig != nil {
		return nil
	}
	
	configDir := filepath.Join(os.Getenv("HOME"), ".mu", "vpn")
	os.MkdirAll(configDir, 0700)
	
	vpnConfig = &VPNConfig{
		ServerEndpoint: os.Getenv("VPN_ENDPOINT"),
	}
	
	serverKeyFile := filepath.Join(configDir, "server.key")
	
	if data, err := os.ReadFile(serverKeyFile); err == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) >= 2 {
			vpnConfig.ServerPrivateKey = strings.TrimSpace(lines[0])
			vpnConfig.ServerPublicKey = strings.TrimSpace(lines[1])
		}
	} else {
		priv, pub, err := generateKeyPair()
		if err != nil {
			return err
		}
		vpnConfig.ServerPrivateKey = priv
		vpnConfig.ServerPublicKey = pub
		os.WriteFile(serverKeyFile, []byte(priv+"\n"+pub+"\n"), 0600)
	}
	
	return nil
}

func getClientConfig(userID string) (*ClientConfig, error) {
	configDir := filepath.Join(os.Getenv("HOME"), ".mu", "vpn", "clients")
	os.MkdirAll(configDir, 0700)
	
	clientFile := filepath.Join(configDir, userID+".key")
	
	if data, err := os.ReadFile(clientFile); err == nil {
		lines := strings.Split(string(data), "\n")
		if len(lines) >= 3 {
			return &ClientConfig{
				PrivateKey: strings.TrimSpace(lines[0]),
				PublicKey:  strings.TrimSpace(lines[1]),
				Address:    strings.TrimSpace(lines[2]),
			}, nil
		}
	}
	
	// Generate new client
	priv, pub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	
	// Simple IP assignment based on hash of userID
	var ipNum int
	for _, c := range userID {
		ipNum += int(c)
	}
	ipNum = (ipNum % 253) + 2 // 2-254
	
	cfg := &ClientConfig{
		PrivateKey: priv,
		PublicKey:  pub,
		Address:    fmt.Sprintf("10.66.66.%d/32", ipNum),
	}
	
	os.WriteFile(clientFile, []byte(cfg.PrivateKey+"\n"+cfg.PublicKey+"\n"+cfg.Address+"\n"), 0600)
	
	// Register peer with WireGuard if running
	if WireGuardRunning() {
		AddPeer(cfg.PublicKey, cfg.Address)
	}
	
	return cfg, nil
}

func generateWireGuardConf(client *ClientConfig) string {
	endpoint := vpnConfig.ServerEndpoint
	if endpoint == "" {
		endpoint = "YOUR_SERVER:51820"
	}
	
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = 1.1.1.1

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = %s
PersistentKeepalive = 25`, client.PrivateKey, client.Address, vpnConfig.ServerPublicKey, endpoint)
}

// VPNSection returns the VPN setup HTML for the tunnel page
func VPNSection(r *http.Request) string {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		return `
<div class="vpn-section">
	<h2>VPN (Full Network Tunnel)</h2>
	<p class="desc">Route ALL device traffic through this server using WireGuard.</p>
	<p style="margin-top:16px;"><a href="/login" style="color:#007bff;">Login</a> to get your personal VPN configuration.</p>
</div>`
	}

	// Check if WireGuard server is running
	if !WireGuardRunning() {
		return `
<div class="vpn-section">
	<h2>VPN (Full Network Tunnel)</h2>
	<p class="desc">VPN server not configured. Set VPN_ENDPOINT environment variable to enable.</p>
</div>`
	}
	
	if err := loadVPNConfig(); err != nil {
		return `<div class="error">Failed to load VPN config</div>`
	}
	
	client, err := getClientConfig(acc.ID)
	if err != nil {
		return `<div class="error">Failed to generate client config</div>`
	}
	
	conf := generateWireGuardConf(client)
	
	endpoint := vpnConfig.ServerEndpoint
	if endpoint == "" {
		endpoint = "Not configured (set VPN_ENDPOINT)"
	}
	
	// Server config for adding this peer
	peerConfig := fmt.Sprintf(`[Peer]
PublicKey = %s
AllowedIPs = %s`, client.PublicKey, strings.Replace(client.Address, "/32", "/32", 1))
	
	return fmt.Sprintf(`
<div class="vpn-section">
	<h2>VPN (Full Network Tunnel)</h2>
	<p class="desc">Route ALL device traffic through this server using WireGuard.</p>
	
	<h3>1. Install WireGuard</h3>
	<p style="margin-bottom:12px;">Download the free WireGuard app (one time):</p>
	<div style="display:flex;gap:12px;flex-wrap:wrap;margin-bottom:24px;">
		<a href="https://apps.apple.com/app/wireguard/id1441195209" target="_blank" style="background:#333;color:#fff;padding:8px 16px;border-radius:4px;text-decoration:none;">iOS App Store</a>
		<a href="https://play.google.com/store/apps/details?id=com.wireguard.android" target="_blank" style="background:#333;color:#fff;padding:8px 16px;border-radius:4px;text-decoration:none;">Android Play Store</a>
	</div>
	
	<h3>2. Scan QR Code</h3>
	<p style="margin-bottom:12px;">Open WireGuard → + → Create from QR code</p>
	<div id="wg-qrcode" style="background:#fff;padding:16px;display:inline-block;border-radius:8px;"></div>
	
	<h3 style="margin-top:24px;">3. Connect</h3>
	<p>Toggle the tunnel on in WireGuard when you need it.</p>
	
	<details style="margin-top:24px;">
		<summary style="cursor:pointer;color:#888;">Show config (for manual import)</summary>
		<pre style="background:#1a1a1a;padding:16px;border-radius:8px;overflow-x:auto;font-size:12px;margin-top:12px;">%s</pre>
	</details>
	
	<details style="margin-top:16px;">
		<summary style="cursor:pointer;color:#888;">Server setup (admin only)</summary>
		<p style="margin-top:12px;">Endpoint: <code>%s</code></p>
		<p style="margin-top:8px;">Add this peer to <code>/etc/wireguard/wg0.conf</code>:</p>
		<pre style="background:#1a1a1a;padding:16px;border-radius:8px;overflow-x:auto;font-size:12px;margin-top:8px;">%s</pre>
		<p style="margin-top:8px;">Then run: <code>sudo wg syncconf wg0 <(wg-quick strip wg0)</code></p>
	</details>
</div>

<script src="https://cdn.jsdelivr.net/npm/qrcode-generator@1.4.4/qrcode.min.js"></script>
<script>
(function() {
	var qr = qrcode(0, 'M');
	qr.addData(%q);
	qr.make();
	document.getElementById('wg-qrcode').innerHTML = qr.createSvgTag(4);
})();
</script>
`, conf, endpoint, peerConfig, conf)
}
