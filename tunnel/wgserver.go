package tunnel

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"mu/app"
)

var (
	wgDevice  *device.Device
	wgNet     *netstack.Net
	wgRunning bool
	wgMu      sync.Mutex
)

// StartWireGuard starts the userspace WireGuard server
func StartWireGuard() error {
	wgMu.Lock()
	defer wgMu.Unlock()

	if wgRunning {
		return nil
	}

	if err := loadVPNConfig(); err != nil {
		return fmt.Errorf("failed to load VPN config: %w", err)
	}

	// Create userspace TUN with netstack
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("10.66.66.1")},
		[]netip.Addr{netip.MustParseAddr("1.1.1.1")},
		1420,
	)
	if err != nil {
		return fmt.Errorf("failed to create TUN: %w", err)
	}

	// Create WireGuard device
	logger := device.NewLogger(device.LogLevelError, "[wg] ")
	wgDevice = device.NewDevice(tun, conn.NewDefaultBind(), logger)

	// Configure the device with server private key
	config := fmt.Sprintf(`private_key=%s
listen_port=51820
`, hexKey(vpnConfig.ServerPrivateKey))

	if err := wgDevice.IpcSet(config); err != nil {
		wgDevice.Close()
		return fmt.Errorf("failed to configure device: %w", err)
	}

	if err := wgDevice.Up(); err != nil {
		wgDevice.Close()
		return fmt.Errorf("failed to bring up device: %w", err)
	}

	wgNet = tnet
	wgRunning = true

	// Start TCP forwarder for the netstack
	go runTCPForwarder(tnet)

	app.Log("wg", "WireGuard server started on :51820")

	// Load existing peers
	loadExistingPeers()

	return nil
}

// AddPeer adds a peer to the WireGuard device
func AddPeer(publicKey string, allowedIP string) error {
	wgMu.Lock()
	defer wgMu.Unlock()

	if wgDevice == nil {
		return fmt.Errorf("WireGuard not running")
	}

	config := fmt.Sprintf(`public_key=%s
allowed_ip=%s
`, hexKey(publicKey), allowedIP)

	return wgDevice.IpcSet(config)
}

// StopWireGuard stops the WireGuard server
func StopWireGuard() {
	wgMu.Lock()
	defer wgMu.Unlock()

	if wgDevice != nil {
		wgDevice.Close()
		wgDevice = nil
	}
	wgRunning = false
	app.Log("wg", "WireGuard server stopped")
}

// WireGuardRunning returns whether WireGuard is running
func WireGuardRunning() bool {
	wgMu.Lock()
	defer wgMu.Unlock()
	return wgRunning
}

// hexKey converts base64 key to hex for WireGuard IPC
func hexKey(b64 string) string {
	data, _ := base64.StdEncoding.DecodeString(b64)
	return hex.EncodeToString(data)
}

// loadExistingPeers loads all existing client configs as peers
func loadExistingPeers() {
	configDir := filepath.Join(os.Getenv("HOME"), ".mu", "vpn", "clients")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".key") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(configDir, entry.Name()))
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		if len(lines) >= 3 {
			pubKey := strings.TrimSpace(lines[1])
			addr := strings.TrimSpace(lines[2])
			if pubKey != "" && addr != "" {
				AddPeer(pubKey, addr)
				app.Log("wg", "Loaded peer: %s", addr)
			}
		}
	}
}

// runTCPForwarder handles TCP connections from the WireGuard tunnel
func runTCPForwarder(tnet *netstack.Net) {
	for {
		// Accept TCP connections destined anywhere
		listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 0})
		if err != nil {
			app.Log("wg", "TCP listener error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		for {
			tunnelConn, err := listener.Accept()
			if err != nil {
				break
			}
			go handleTunnelConn(tunnelConn)
		}
	}
}

func handleTunnelConn(tunnelConn net.Conn) {
	defer tunnelConn.Close()

	// Get the original destination from the connection
	// For netstack, the remote addr is the destination the client wanted
	destAddr := tunnelConn.LocalAddr().String()

	// Connect to the real destination
	realConn, err := net.DialTimeout("tcp", destAddr, 10*time.Second)
	if err != nil {
		return
	}
	defer realConn.Close()

	// Proxy bidirectionally
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(realConn, tunnelConn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(tunnelConn, realConn)
		done <- struct{}{}
	}()
	<-done
}
