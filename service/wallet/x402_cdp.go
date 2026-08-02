package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// CDP (Coinbase Developer Platform) credentials authenticate calls to
// Coinbase's hosted x402 facilitator, which — unlike the open testnet
// facilitator — can settle real payments on Base mainnet. They are only
// consulted when the configured facilitator is CDP; the open facilitator
// ignores the Authorization header.
//
// Standard ecosystem env var names (same as @coinbase/x402) so operators can
// reuse credentials/tooling without translation.
var (
	cdpKeyID     = os.Getenv("CDP_API_KEY_ID")
	cdpKeySecret = os.Getenv("CDP_API_KEY_SECRET")
)

// cdpConfigured reports whether CDP facilitator credentials are present.
func cdpConfigured() bool { return cdpKeyID != "" && cdpKeySecret != "" }

// cdpBearer builds a CDP Bearer JWT (EdDSA / Ed25519) authorizing a single
// REST call. host is like "api.cdp.coinbase.com" and path like
// "/platform/v2/x402/verify"; the token binds to that exact method+URI and is
// valid for two minutes, per CDP's authentication spec.
func cdpBearer(method, host, path string) (string, error) {
	key, err := ed25519KeyFromSecret(cdpKeySecret)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	header := map[string]any{
		"alg":   "EdDSA",
		"typ":   "JWT",
		"kid":   cdpKeyID,
		"nonce": hex.EncodeToString(nonce),
	}
	now := time.Now().Unix()
	claims := map[string]any{
		"sub": cdpKeyID,
		"iss": "cdp",
		"aud": []string{"cdp_service"},
		"nbf": now,
		"exp": now + 120,
		"uri": method + " " + host + path,
	}

	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signing := b64url(hb) + "." + b64url(cb)
	sig := ed25519.Sign(key, []byte(signing))
	return signing + "." + b64url(sig), nil
}

// ed25519KeyFromSecret decodes a CDP Ed25519 secret. CDP secrets are base64 and
// decode to 64 bytes (32-byte seed + 32-byte public key) — exactly Go's
// ed25519.PrivateKey layout; a bare 32-byte seed is also accepted.
func ed25519KeyFromSecret(secret string) (ed25519.PrivateKey, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("CDP_API_KEY_SECRET is empty")
	}
	if strings.Contains(secret, "BEGIN") {
		return nil, fmt.Errorf("CDP_API_KEY_SECRET looks like a PEM/EC key; x402 bearer auth needs an Ed25519 Secret API Key — create one in the CDP portal")
	}
	raw, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		if raw, err = base64.RawURLEncoding.DecodeString(secret); err != nil {
			return nil, fmt.Errorf("CDP_API_KEY_SECRET is not valid base64: %w", err)
		}
	}
	switch len(raw) {
	case ed25519.PrivateKeySize: // 64: seed + public key
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize: // 32: seed only
		return ed25519.NewKeyFromSeed(raw), nil
	default:
		return nil, fmt.Errorf("CDP_API_KEY_SECRET decoded to %d bytes; expected 32 or 64 (Ed25519)", len(raw))
	}
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
