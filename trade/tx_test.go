package trade

import (
	"encoding/hex"
	"math/big"
	"testing"
)

func TestRLPEncodeBytes(t *testing.T) {
	tests := []struct {
		input []byte
		want  string
	}{
		{nil, "80"},
		{[]byte{}, "80"},
		{[]byte{0x00}, "00"},
		{[]byte{0x7f}, "7f"},
		{[]byte{0x80}, "8180"},
		{[]byte{1, 2, 3}, "83010203"},
	}
	for _, tt := range tests {
		got := hex.EncodeToString(rlpEncodeBytes(tt.input))
		if got != tt.want {
			t.Errorf("rlpEncodeBytes(%x) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestRLPEncodeList(t *testing.T) {
	// rlp([]) = 0xc0
	got := hex.EncodeToString(rlpEncodeList([]any{}))
	if got != "c0" {
		t.Errorf("rlpEncodeList([]) = %s, want c0", got)
	}

	// rlp(["cat", "dog"]) should start with list prefix
	items := []any{[]byte("cat"), []byte("dog")}
	encoded := rlpEncodeList(items)
	if len(encoded) == 0 {
		t.Error("empty encoding")
	}
}

func TestSignTransaction(t *testing.T) {
	// Simple transfer transaction — just verify it doesn't panic
	// and produces valid-looking output
	privKey := "0000000000000000000000000000000000000000000000000000000000000001"
	tx := &Transaction{
		ChainID:              big.NewInt(8453),
		Nonce:                0,
		MaxPriorityFeePerGas: big.NewInt(1000000),
		MaxFeePerGas:         big.NewInt(2000000),
		GasLimit:             21000,
		To:                   "0x0000000000000000000000000000000000000002",
		Value:                big.NewInt(1000000000000000), // 0.001 ETH
		Data:                 nil,
	}

	signed, err := SignTransaction(tx, privKey)
	if err != nil {
		t.Fatalf("SignTransaction error: %v", err)
	}
	if len(signed) == 0 {
		t.Fatal("empty signed transaction")
	}
	if signed[0] != 0x02 {
		t.Errorf("first byte = 0x%02x, want 0x02 (EIP-1559)", signed[0])
	}
}

func TestECDSASign(t *testing.T) {
	// Sign a known hash with private key = 1
	hash := make([]byte, 32)
	hash[31] = 1
	privKey := make([]byte, 32)
	privKey[31] = 1

	r, s, v, err := ecdsaSign(hash, privKey)
	if err != nil {
		t.Fatalf("ecdsaSign error: %v", err)
	}
	if r.Sign() == 0 || s.Sign() == 0 {
		t.Error("r or s is zero")
	}
	if v > 1 {
		t.Errorf("v = %d, want 0 or 1", v)
	}
	// s should be in lower half
	if s.Cmp(secp256k1HalfN) > 0 {
		t.Error("s not normalized to lower half")
	}
}

func TestBigIntBytes(t *testing.T) {
	if len(bigIntBytes(nil)) != 0 {
		t.Error("nil should produce empty")
	}
	if len(bigIntBytes(big.NewInt(0))) != 0 {
		t.Error("0 should produce empty")
	}
	got := hex.EncodeToString(bigIntBytes(big.NewInt(256)))
	if got != "0100" {
		t.Errorf("bigIntBytes(256) = %s, want 0100", got)
	}
}
