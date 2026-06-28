package trade

import (
	"math/big"
	"strings"
	"testing"
)

func TestGetTradesHandlesNonPositiveLimit(t *testing.T) {
	walletMu.Lock()
	trades = map[string][]*Trade{
		"acct": {
			{ID: "oldest"},
			{ID: "newest"},
		},
	}
	walletMu.Unlock()

	if got := GetTrades("acct", 0); len(got) != 0 {
		t.Fatalf("GetTrades limit 0 returned %d trades, want 0", len(got))
	}
	if got := GetTrades("acct", -1); len(got) != 0 {
		t.Fatalf("GetTrades negative limit returned %d trades, want 0", len(got))
	}
}

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
		{".5", 6, "500000"},
		{"1.", 6, "1000000"},
		{" 2.5 ", 6, "2500000"},
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

func TestParseAmountRejectsInvalidAmounts(t *testing.T) {
	tests := []struct {
		name     string
		amount   string
		decimals int
	}{
		{"empty", "", 6},
		{"just decimal", ".", 6},
		{"negative", "-1", 6},
		{"positive sign", "+1", 6},
		{"invalid fractional", "1.a", 6},
		{"too many decimal separators", "1.2.3", 6},
		{"too many fractional digits", "0.0000001", 6},
		{"negative decimals", "1", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := ParseAmount(tt.amount, tt.decimals); err == nil {
				t.Fatalf("ParseAmount(%q, %d) = %s, want error", tt.amount, tt.decimals, got)
			}
		})
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
	initTokens()
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
