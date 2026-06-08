package trade

import (
	"math/big"
	"strings"
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		amount   string
		decimals int
		want     string
	}{
		{"100", 6, "100000000"},
		{"0.5", 18, "500000000000000000"},
		{"1.23", 6, "1230000"},
		{"0.000001", 6, "1"},
	}
	for _, tt := range tests {
		got, err := ParseAmount(tt.amount, tt.decimals)
		if err != nil {
			t.Errorf("ParseAmount(%q, %d) error: %v", tt.amount, tt.decimals, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("ParseAmount(%q, %d) = %s, want %s", tt.amount, tt.decimals, got.String(), tt.want)
		}
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		raw      string
		decimals int
		want     string
	}{
		{"100000000", 6, "100"},
		{"500000000000000000", 18, "0.5"},
		{"1230000", 6, "1.23"},
	}
	for _, tt := range tests {
		raw, _ := new(big.Int).SetString(tt.raw, 10)
		got := FormatAmount(raw, tt.decimals)
		if got != tt.want {
			t.Errorf("FormatAmount(%s, %d) = %q, want %q", tt.raw, tt.decimals, got, tt.want)
		}
	}
}

func TestGenerateKeypair(t *testing.T) {
	privKey, addr, err := generateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if len(privKey) != 64 {
		t.Errorf("private key length = %d, want 64", len(privKey))
	}
	if !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
		t.Errorf("address = %q, want 0x... with length 42", addr)
	}
}

func TestAddressFromKnownKey(t *testing.T) {
	// Known test vector: private key 1 → well-known address
	privKey := make([]byte, 32)
	privKey[31] = 1
	addr := addressFromPrivateKey(privKey)
	want := "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
	if addr != want {
		t.Errorf("addressFromPrivateKey(1) = %q, want %q", addr, want)
	}
}

func TestListTokens(t *testing.T) {
	tokens := ListTokens()
	if len(tokens) == 0 {
		t.Error("ListTokens() returned empty")
	}
	found := false
	for _, tok := range tokens {
		if tok.Symbol == "USDC" {
			found = true
		}
	}
	if !found {
		t.Error("USDC not found in ListTokens()")
	}
}
