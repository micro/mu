package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"entry":[]}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		body      []byte
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      body,
			signature: validSignature,
			secret:    secret,
			want:      true,
		},
		{
			name:      "tampered body",
			body:      []byte(`{"entry":[{"changed":true}]}`),
			signature: validSignature,
			secret:    secret,
			want:      false,
		},
		{
			name:      "wrong secret",
			body:      body,
			signature: validSignature,
			secret:    "different-secret",
			want:      false,
		},
		{
			name:      "missing sha256 prefix",
			body:      body,
			signature: hex.EncodeToString(mac.Sum(nil)),
			secret:    secret,
			want:      false,
		},
		{
			name:      "invalid hex",
			body:      body,
			signature: "sha256=not-hex",
			secret:    secret,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := verifySignature(tt.body, tt.signature, tt.secret); got != tt.want {
				t.Fatalf("verifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}
