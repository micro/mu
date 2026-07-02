package wallet

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"
)

func mustBig(s string) *big.Int {
	v, _ := new(big.Int).SetString(s, 10)
	return v
}

// TestAddressFromKnownKey checks secp256k1 + Keccak address derivation against
// the canonical vector: private key 1 → 0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf.
func TestAddressFromKnownKey(t *testing.T) {
	key := make([]byte, 32)
	key[31] = 1
	got := addressFromPrivateKey(key)
	want := "0x7e5f4552091a69125d5dfcb7b8c2659029395bdf"
	if !strings.EqualFold(got, want) {
		t.Fatalf("addressFromPrivateKey(1) = %s, want %s", got, want)
	}
}

func TestGenerateKeypairRoundTrips(t *testing.T) {
	priv, addr, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if len(priv) != 64 {
		t.Fatalf("private key hex len = %d, want 64", len(priv))
	}
	if _, err := hex.DecodeString(priv); err != nil {
		t.Fatalf("private key not hex: %v", err)
	}
	if again, ok := AddressFromPrivateKeyHex(priv); !ok || !strings.EqualFold(again, addr) {
		t.Fatalf("address mismatch: GenerateKeypair=%s AddressFromPrivateKeyHex=%s ok=%v", addr, again, ok)
	}
}

func TestFormatUnits(t *testing.T) {
	for _, c := range []struct {
		raw      string
		decimals int
		want     string
	}{
		{"1500000", 6, "1.5"},
		{"1000000", 6, "1"},
		{"0", 6, "0"},
		{"10000", 6, "0.01"},
		{"123456", 6, "0.123456"},
	} {
		got := FormatUnits(mustBig(c.raw), c.decimals)
		if got != c.want {
			t.Errorf("FormatUnits(%s,%d)=%s want %s", c.raw, c.decimals, got, c.want)
		}
	}
}
