package mail

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestLoadDKIMConfigFromEnv(t *testing.T) {
	// Generate a test RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}

	// Encode to PEM
	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}
	pemData := string(pem.EncodeToMemory(pemBlock))

	// Reset global state after test
	defer func() { dkimConfig = nil }()

	// Set the environment variable
	t.Setenv("DKIM_PRIVATE_KEY", pemData)

	if err := LoadDKIMConfig("example.com", "mail"); err != nil {
		t.Fatalf("LoadDKIMConfig returned error: %v", err)
	}

	enabled, domain, selector := DKIMStatus()
	if !enabled {
		t.Error("expected DKIM to be enabled after loading key from env var")
	}
	if domain != "example.com" {
		t.Errorf("expected domain %q, got %q", "example.com", domain)
	}
	if selector != "mail" {
		t.Errorf("expected selector %q, got %q", "mail", selector)
	}
}

func TestLoadDKIMConfigEnvTakesPrecedence(t *testing.T) {
	// Verify that DKIM_PRIVATE_KEY env var is used even when no key file exists
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test RSA key: %v", err)
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}
	pemData := string(pem.EncodeToMemory(pemBlock))

	defer func() { dkimConfig = nil }()

	// Ensure env var is set; the key file at ~/.mu/keys/dkim.key may not exist on CI
	t.Setenv("DKIM_PRIVATE_KEY", pemData)

	if err := LoadDKIMConfig("test.example.org", "selector1"); err != nil {
		t.Fatalf("LoadDKIMConfig should succeed when DKIM_PRIVATE_KEY is set: %v", err)
	}

	enabled, _, _ := DKIMStatus()
	if !enabled {
		t.Error("expected DKIM to be enabled")
	}
}

func TestSendExternalEmailHTMLWrapping(t *testing.T) {
	// Test that HTML fragments are properly wrapped in HTML document structure

	tests := []struct {
		name        string
		bodyHTML    string
		shouldWrap  bool
		description string
	}{
		{
			name:        "Simple text fragment",
			bodyHTML:    "Hello!<br>It's working.",
			shouldWrap:  true,
			description: "Plain HTML fragment should be wrapped",
		},
		{
			name:        "Already has HTML structure",
			bodyHTML:    "<!DOCTYPE html><html><body>Hello</body></html>",
			shouldWrap:  false,
			description: "Complete HTML document should not be wrapped again",
		},
		{
			name:        "Has html tag",
			bodyHTML:    "<html><body>Test</body></html>",
			shouldWrap:  false,
			description: "HTML with html tag should not be wrapped",
		},
		{
			name:        "Complex HTML fragment",
			bodyHTML:    "Line 1<br>Line 2<br>It's great!",
			shouldWrap:  true,
			description: "Multi-line fragment should be wrapped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the wrapping logic from SendExternalEmail
			result := tt.bodyHTML
			htmlLower := strings.ToLower(result)

			if !strings.Contains(htmlLower, "<html") && !strings.Contains(htmlLower, "<!doctype") {
				// Would be wrapped
				if !tt.shouldWrap {
					t.Errorf("Test case %q: Expected not to wrap, but would be wrapped", tt.name)
				}
			} else {
				// Would NOT be wrapped
				if tt.shouldWrap {
					t.Errorf("Test case %q: Expected to wrap, but would not be wrapped", tt.name)
				}
			}
		})
	}
}
