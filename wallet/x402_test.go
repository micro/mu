package wallet

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCreditsToAtomic(t *testing.T) {
	cases := []struct {
		credits, decimals int
		want              string
	}{
		{5, 6, "50000"},     // $0.05 USDC
		{1, 6, "10000"},     // $0.01 USDC
		{0, 6, "10000"},     // floored to 1 credit
		{100, 6, "1000000"}, // $1.00
	}
	for _, c := range cases {
		if got := creditsToAtomic(c.credits, c.decimals); got != c.want {
			t.Errorf("creditsToAtomic(%d,%d)=%s want %s", c.credits, c.decimals, got, c.want)
		}
	}
}

func TestNormalizeNetwork(t *testing.T) {
	for in, want := range map[string]string{
		"base":         "eip155:8453",
		"eip155:8453":  "eip155:8453",
		"base-sepolia": "eip155:84532",
		"eip155:84532": "eip155:84532",
		"solana":       "solana", // passthrough
	} {
		if got := normalizeNetwork(in); got != want {
			t.Errorf("normalizeNetwork(%q)=%q want %q", in, got, want)
		}
	}
}

// TestCDPBearer certifies the JWT is well-formed (three segments, EdDSA header,
// correct claims) and that its signature verifies against the key — so a bad
// signing path can't silently ship.
func TestCDPBearer(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	cdpKeyID = "test-key-id"
	cdpKeySecret = base64.StdEncoding.EncodeToString(priv) // 64-byte seed+pub
	defer func() { cdpKeyID, cdpKeySecret = "", "" }()

	tok, err := cdpBearer("POST", "api.cdp.coinbase.com", "/platform/v2/x402/verify")
	if err != nil {
		t.Fatalf("cdpBearer: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	// Signature must verify over "header.payload".
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("JWT signature does not verify")
	}

	var hdr map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &hdr)
	if hdr["alg"] != "EdDSA" || hdr["kid"] != "test-key-id" || hdr["nonce"] == nil {
		t.Errorf("bad header: %v", hdr)
	}

	var claims map[string]any
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(cb, &claims)
	if claims["iss"] != "cdp" || claims["sub"] != "test-key-id" {
		t.Errorf("bad claims: %v", claims)
	}
	if claims["uri"] != "POST api.cdp.coinbase.com/platform/v2/x402/verify" {
		t.Errorf("bad uri claim: %v", claims["uri"])
	}
}

func TestBuildPaymentRequirementsShape(t *testing.T) {
	x402PayTo = "0x9a717EFF039622231C65ADbF7B2A002b544b06A9"
	x402NetworkID = "eip155:8453"
	defer func() { x402PayTo = "" }()

	reqs := BuildPaymentRequirements("chat_query", "https://m3o.com/mcp")
	if len(reqs) == 0 {
		t.Fatal("expected at least one requirement")
	}
	r := reqs[0]
	if r.Scheme != "exact" || r.Network != "eip155:8453" || r.PayTo != x402PayTo {
		t.Errorf("bad requirement: %+v", r)
	}
	if r.MaxAmountRequired == "" || strings.Contains(r.MaxAmountRequired, "$") {
		t.Errorf("maxAmountRequired must be atomic units, got %q", r.MaxAmountRequired)
	}
	if r.Extra["name"] == "" || r.Extra["version"] == "" {
		t.Errorf("extra must carry EIP-712 name/version, got %v", r.Extra)
	}
}
