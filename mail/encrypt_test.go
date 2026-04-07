package mail

import (
	"crypto/rand"
	"io"
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Set up a test key
	encKey = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, encKey); err != nil {
		t.Fatal(err)
	}
	encEnabled = true

	tests := []string{
		"Hello, world!",
		"Sensitive email content with special chars: <html>&amp;</html>",
		"Short",
		strings.Repeat("Long message ", 1000),
		"",
	}

	for _, plaintext := range tests {
		encrypted, err := encrypt(plaintext)
		if err != nil {
			t.Fatalf("encrypt(%q): %v", plaintext[:min(len(plaintext), 20)], err)
		}

		if plaintext != "" && !strings.HasPrefix(encrypted, encPrefix) {
			t.Fatalf("encrypted text should have prefix %q", encPrefix)
		}

		if plaintext != "" && encrypted == plaintext {
			t.Fatal("encrypted text should differ from plaintext")
		}

		decrypted, err := decrypt(encrypted)
		if err != nil {
			t.Fatalf("decrypt: %v", err)
		}

		if decrypted != plaintext {
			t.Fatalf("roundtrip failed: got %q, want %q", decrypted[:min(len(decrypted), 50)], plaintext[:min(len(plaintext), 50)])
		}
	}
}

func TestEncryptMessage(t *testing.T) {
	encKey = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, encKey); err != nil {
		t.Fatal(err)
	}
	encEnabled = true

	m := &Message{
		ID:      "123",
		From:    "alice",
		FromID:  "alice-id",
		To:      "bob",
		ToID:    "bob-id",
		Subject: "Secret meeting",
		Body:    "Meet me at the park",
	}

	if err := encryptMessage(m); err != nil {
		t.Fatal(err)
	}

	// ID, From, FromID should NOT be encrypted
	if strings.HasPrefix(m.ID, encPrefix) {
		t.Error("ID should not be encrypted")
	}
	if strings.HasPrefix(m.From, encPrefix) {
		t.Error("From should not be encrypted")
	}
	if strings.HasPrefix(m.FromID, encPrefix) {
		t.Error("FromID should not be encrypted")
	}

	// To, ToID, Subject, Body SHOULD be encrypted
	if !strings.HasPrefix(m.Subject, encPrefix) {
		t.Error("Subject should be encrypted")
	}
	if !strings.HasPrefix(m.Body, encPrefix) {
		t.Error("Body should be encrypted")
	}
	if !strings.HasPrefix(m.To, encPrefix) {
		t.Error("To should be encrypted")
	}
	if !strings.HasPrefix(m.ToID, encPrefix) {
		t.Error("ToID should be encrypted")
	}

	// Decrypt and verify
	if err := decryptMessage(m); err != nil {
		t.Fatal(err)
	}

	if m.Subject != "Secret meeting" {
		t.Errorf("Subject = %q, want %q", m.Subject, "Secret meeting")
	}
	if m.Body != "Meet me at the park" {
		t.Errorf("Body = %q, want %q", m.Body, "Meet me at the park")
	}
	if m.To != "bob" {
		t.Errorf("To = %q, want %q", m.To, "bob")
	}
	if m.ToID != "bob-id" {
		t.Errorf("ToID = %q, want %q", m.ToID, "bob-id")
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	encKey = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, encKey); err != nil {
		t.Fatal(err)
	}
	encEnabled = true

	// Pre-encryption messages (no enc: prefix) should pass through unchanged
	m := &Message{
		Subject: "Old unencrypted subject",
		Body:    "Old unencrypted body",
		To:      "charlie",
		ToID:    "charlie-id",
	}

	if err := decryptMessage(m); err != nil {
		t.Fatal(err)
	}

	if m.Subject != "Old unencrypted subject" {
		t.Error("Plaintext subject should pass through unchanged")
	}
	if m.Body != "Old unencrypted body" {
		t.Error("Plaintext body should pass through unchanged")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
