package wallet

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// TestEIP712TypeHashes pins the type hashes to their canonical published values,
// which certifies the keccak256 + type-string encoding used for signing.
func TestEIP712TypeHashes(t *testing.T) {
	if got := hex.EncodeToString(transferWithAuthorizationTypeHash); got != "7c7c6cdb67a18743f49ec6fa9b35f50d52ed05cbed4cc592e13b44501c1a2267" {
		t.Errorf("TransferWithAuthorization typehash = %s", got)
	}
	if got := hex.EncodeToString(eip712DomainTypeHash); got != "8b73c3c69bb8fe3d512ecc4cf759cc79239f7b179b0ffacaa9a75d522b39400f" {
		t.Errorf("EIP712Domain typehash = %s", got)
	}
}

// TestSignX402PaymentStructure signs a payment and checks the decoded X-PAYMENT
// payload is well-formed (v1 exact scheme).
func TestSignX402PaymentStructure(t *testing.T) {
	priv, addr, _ := GenerateKeypair()
	bw := &BaseWallet{Address: addr, PrivateKey: priv}
	req := PaymentRequirements{
		Scheme:            "exact",
		Network:           "base",
		MaxAmountRequired: "50000",
		PayTo:             "0x9a717EFF039622231C65ADbF7B2A002b544b06A9",
		Asset:             baseUSDC,
		MaxTimeoutSeconds: 60,
		Extra:             map[string]string{"name": "USD Coin", "version": "2"},
	}

	hdr, err := SignX402Payment(bw, req)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(hdr)
	if err != nil {
		t.Fatalf("header not base64: %v", err)
	}
	var p struct {
		X402Version int    `json:"x402Version"`
		Scheme      string `json:"scheme"`
		Network     string `json:"network"`
		Payload     struct {
			Signature     string        `json:"signature"`
			Authorization authorization `json:"authorization"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if p.Scheme != "exact" || p.Network != "base" {
		t.Errorf("scheme/network = %s/%s", p.Scheme, p.Network)
	}
	// 65-byte signature → 0x + 130 hex chars.
	if !strings.HasPrefix(p.Payload.Signature, "0x") || len(p.Payload.Signature) != 132 {
		t.Errorf("bad signature %q", p.Payload.Signature)
	}
	a := p.Payload.Authorization
	if !strings.EqualFold(a.From, addr) || !strings.EqualFold(a.To, req.PayTo) {
		t.Errorf("from/to = %s/%s", a.From, a.To)
	}
	if a.Value != "50000" || a.ValidAfter != "0" {
		t.Errorf("value/validAfter = %s/%s", a.Value, a.ValidAfter)
	}
	if !strings.HasPrefix(a.Nonce, "0x") || len(a.Nonce) != 66 {
		t.Errorf("bad nonce %q", a.Nonce)
	}
}

func TestSignX402PaymentRejectsBadInputs(t *testing.T) {
	bw := &BaseWallet{Address: "0xabc", PrivateKey: "00"}
	if _, err := SignX402Payment(bw, PaymentRequirements{Network: "solana"}); err == nil {
		t.Error("expected error for unsupported network")
	}
}
