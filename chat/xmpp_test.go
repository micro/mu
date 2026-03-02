package chat

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTempCert generates a self-signed certificate and writes cert/key PEM
// files into dir. Returns (certPath, keyPath).
func writeTempCert(t *testing.T, dir string, notBefore time.Time) (string, string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    notBefore,
		NotAfter:     notBefore.Add(24 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "privkey.pem")

	cf, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	cf.Close()

	kf, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pem.Encode(kf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	kf.Close()

	return certPath, keyPath
}

// TestTLSCertStoreReload verifies that tlsCertStore reloads the certificate
// when the file modification time changes (simulating a Let's Encrypt renewal).
func TestTLSCertStoreReload(t *testing.T) {
	dir := t.TempDir()

	// Write initial certificate
	certPath, keyPath := writeTempCert(t, dir, time.Now().Add(-time.Hour))

	store := &tlsCertStore{certFile: certPath, keyFile: keyPath}

	// First call: should load and cache the cert
	cert1, err := store.GetCertificate(nil)
	if err != nil {
		t.Fatalf("first GetCertificate: %v", err)
	}
	if cert1 == nil {
		t.Fatal("expected non-nil cert on first call")
	}

	// Second call with unchanged files: should return cached cert (same pointer)
	cert2, err := store.GetCertificate(nil)
	if err != nil {
		t.Fatalf("second GetCertificate: %v", err)
	}
	if cert1 != cert2 {
		t.Error("expected cached cert pointer on second call with unchanged files")
	}

	// Simulate renewal: write a new cert with a future notBefore so it produces
	// a different leaf certificate, then touch the file to update mtime.
	time.Sleep(10 * time.Millisecond) // ensure different mtime
	writeTempCert(t, dir, time.Now())
	// Force mtime to be strictly after the cached value
	now := time.Now().Add(time.Second)
	os.Chtimes(certPath, now, now)
	os.Chtimes(keyPath, now, now)

	// Third call: files changed → should reload and return a new cert
	cert3, err := store.GetCertificate(nil)
	if err != nil {
		t.Fatalf("third GetCertificate after renewal: %v", err)
	}
	if cert3 == nil {
		t.Fatal("expected non-nil cert after renewal")
	}
	if cert3 == cert2 {
		t.Error("expected new cert pointer after file modification")
	}
}

// TestTLSCertStoreFallback verifies that a reload failure keeps the old cert.
func TestTLSCertStoreFallback(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTempCert(t, dir, time.Now().Add(-time.Hour))

	store := &tlsCertStore{certFile: certPath, keyFile: keyPath}

	// Prime the cache
	cert1, err := store.GetCertificate(nil)
	if err != nil || cert1 == nil {
		t.Fatalf("initial load: %v", err)
	}

	// Corrupt the cert file and mark it as newer so reload is triggered
	now := time.Now().Add(time.Second)
	os.WriteFile(certPath, []byte("not a cert"), 0600)
	os.Chtimes(certPath, now, now)

	// Should fall back to the cached cert without returning an error
	cert2, err := store.GetCertificate(nil)
	if err != nil {
		t.Fatalf("expected fallback to cached cert, got error: %v", err)
	}
	if cert2 != cert1 {
		t.Error("expected same cached cert pointer after failed reload")
	}
}

// TestNewXMPPServer tests server initialization
func TestNewXMPPServer(t *testing.T) {
	server := NewXMPPServer("test.example.com", "5222", "5269")

	if server == nil {
		t.Fatal("Expected server to be created, got nil")
	}

	if server.Domain != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got '%s'", server.Domain)
	}

	if server.Port != "5222" {
		t.Errorf("Expected port '5222', got '%s'", server.Port)
	}

	if server.sessions == nil {
		t.Error("Expected sessions map to be initialized")
	}

	if len(server.sessions) != 0 {
		t.Errorf("Expected 0 sessions initially, got %d", len(server.sessions))
	}
}

// TestGenerateStreamID tests stream ID generation
func TestGenerateStreamID(t *testing.T) {
	id1 := generateStreamID()
	if id1 == "" {
		t.Error("Expected non-empty stream ID")
	}

	// Wait a bit to ensure different timestamp
	time.Sleep(1 * time.Millisecond)

	id2 := generateStreamID()
	if id2 == "" {
		t.Error("Expected non-empty stream ID")
	}

	if id1 == id2 {
		t.Error("Expected different stream IDs for different calls")
	}
}

// TestGetXMPPStatus tests status retrieval
func TestGetXMPPStatus(t *testing.T) {
	// Test when server is nil (not started)
	status := GetXMPPStatus()

	if status["enabled"] != false {
		t.Error("Expected enabled to be false when server is nil")
	}

	// Create a server instance
	xmppServer = NewXMPPServer("test.example.com", "5222", "5269")

	status = GetXMPPStatus()

	if status["enabled"] != true {
		t.Error("Expected enabled to be true when server exists")
	}

	if status["domain"] != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got '%v'", status["domain"])
	}

	if status["c2s_port"] != "5222" {
		t.Errorf("Expected port '5222', got '%v'", status["c2s_port"])
	}

	if status["sessions"] != 0 {
		t.Errorf("Expected 0 sessions, got '%v'", status["sessions"])
	}

	// Clean up
	xmppServer = nil
}

// TestXMPPServerStop tests graceful shutdown
func TestXMPPServerStop(t *testing.T) {
	server := NewXMPPServer("test.example.com", "5222", "5269")

	// Stop should not error even if listener is nil
	err := server.Stop()
	if err != nil {
		t.Errorf("Expected no error on stop with nil listener, got %v", err)
	}

	// Check that context is cancelled
	select {
	case <-server.ctx.Done():
		// Context cancelled as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected context to be cancelled after Stop()")
	}
}
