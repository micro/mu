package wallet

import (
	"encoding/hex"
	"strings"
	"testing"
)

// Recovery is the whole security property: an address is only trustworthy
// because none but the key holder could produce a signature that recovers to
// it. Round-tripping against our own signer is the check that matters.
func TestRecoverRoundTrip(t *testing.T) {
	for i := 0; i < 8; i++ {
		privHex, addr, err := GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		priv, err := hex.DecodeString(strings.TrimPrefix(privHex, "0x"))
		if err != nil {
			t.Fatal(err)
		}

		hash := keccak256([]byte("mu-auth:micro.mu:1786598400:abc123"), []byte{byte(i)})
		sig, err := signHash(hash, priv)
		if err != nil {
			t.Fatal(err)
		}

		got, err := ecdsaRecover(hash, sig)
		if err != nil {
			t.Fatalf("recover failed: %v", err)
		}
		if !strings.EqualFold(got, addr) {
			t.Fatalf("recovered %s, signed with %s", got, addr)
		}
	}
}

// A signature over a different message must not recover to the signer. This is
// the attack the whole scheme rests on: replaying somebody's signature against
// other content.
func TestRecoverRejectsATamperedMessage(t *testing.T) {
	privHex, addr, _ := GenerateKeypair()
	priv, _ := hex.DecodeString(privHex)

	sig, err := signHash(keccak256([]byte("the real message")), priv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ecdsaRecover(keccak256([]byte("a different message")), sig)
	if err == nil && strings.EqualFold(got, addr) {
		t.Fatal("a signature verified against a message it was not made for")
	}
}

// Malformed input is refused rather than guessed at.
func TestRecoverRejectsMalformed(t *testing.T) {
	hash := keccak256([]byte("x"))
	cases := map[string][]byte{
		"empty":      {},
		"too short":  make([]byte, 64),
		"bad recid":  append(make([]byte, 64), 9),
		"zero r and s": func() []byte {
			s := make([]byte, 65)
			s[64] = 27
			return s
		}(),
	}
	for name, sig := range cases {
		if _, err := ecdsaRecover(hash, sig); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ecdsaRecover(make([]byte, 31), make([]byte, 65)); err == nil {
		t.Error("short hash: accepted")
	}
}
