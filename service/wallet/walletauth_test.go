package wallet

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testWallet(t *testing.T) *BaseWallet {
	t.Helper()
	priv, addr, err := GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return &BaseWallet{Address: addr, PrivateKey: priv}
}

// The point of the whole scheme: a key can say who it is without paying, so the
// free account-scoped tools stop being unreachable to an agent.
func TestSignedWalletProvesItsAddress(t *testing.T) {
	bw := testWallet(t)
	header, err := SignAuth(bw, "micro.mu")
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyAuth(header, "micro.mu")
	if err != nil {
		t.Fatalf("a freshly signed header did not verify: %v", err)
	}
	if !strings.EqualFold(got, bw.Address) {
		t.Errorf("proved %s, signed by %s", got, bw.Address)
	}
}

// A header is good once. Without this every other check is decoration: anyone
// who saw the header could resend it inside the window.
func TestAHeaderCannotBeReplayed(t *testing.T) {
	bw := testWallet(t)
	header, _ := SignAuth(bw, "micro.mu")

	if _, err := VerifyAuth(header, "micro.mu"); err != nil {
		t.Fatalf("first use failed: %v", err)
	}
	if _, err := VerifyAuth(header, "micro.mu"); err == nil {
		t.Fatal("the same header authenticated twice")
	}
}

// A signature made for one instance must not work at another, or an operator
// could collect headers from their own users and spend them elsewhere as those
// users.
func TestAHeaderIsBoundToItsHost(t *testing.T) {
	bw := testWallet(t)
	header, _ := SignAuth(bw, "someone-elses-instance.example")

	if _, err := VerifyAuth(header, "micro.mu"); err == nil {
		t.Fatal("a header signed for another host was accepted")
	}
}

// Old headers die. This is what bounds the damage of one leaking.
func TestAnOldHeaderIsRefused(t *testing.T) {
	bw := testWallet(t)
	stale := forgeAuth(t, bw, "micro.mu", time.Now().Add(-authWindow-time.Minute).Unix(), "staleNonce")

	if _, err := VerifyAuth(stale, "micro.mu"); err == nil {
		t.Fatal("a header from outside the window was accepted")
	}
}

// Claiming one address while signing with another must fail — the address is
// whatever the signature recovers to, never what the body says.
func TestAClaimedAddressCannotBeFaked(t *testing.T) {
	bw := testWallet(t)
	victim := testWallet(t)

	header, _ := SignAuth(bw, "micro.mu")
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, AuthScheme)))
	var auth WalletAuth
	if err := json.Unmarshal(raw, &auth); err != nil {
		t.Fatal(err)
	}
	auth.Address = victim.Address // same signature, somebody else's name on it
	tampered, _ := json.Marshal(auth)

	if _, err := VerifyAuth(AuthScheme+" "+base64.StdEncoding.EncodeToString(tampered), "micro.mu"); err == nil {
		t.Fatal("a header claiming another address was accepted")
	}
}

// Nothing that is not a wallet authorization should be mistaken for one — a
// bearer token in the same header must fall through, not fail open.
func TestNonWalletAuthorizationIsNotAnIdentity(t *testing.T) {
	for _, h := range []string{
		"",
		"Bearer abc123",
		"Wallet",
		"Wallet not-base64!!",
		AuthScheme + " " + base64.StdEncoding.EncodeToString([]byte("not json")),
	} {
		if addr, err := VerifyAuth(h, "micro.mu"); err == nil || addr != "" {
			t.Errorf("%q was treated as an identity (%q)", h, addr)
		}
	}
}

// forgeAuth builds a correctly signed header with a chosen timestamp, so the
// window can be tested without waiting for it.
func forgeAuth(t *testing.T, bw *BaseWallet, host string, issuedAt int64, nonce string) string {
	t.Helper()
	key := mustKey(t, bw)
	auth := WalletAuth{Address: bw.Address, Host: host, IssuedAt: issuedAt, Nonce: nonce}
	sig, err := signHash(keccak256(authMessage(auth.Host, auth.Address, auth.IssuedAt, auth.Nonce)), key)
	if err != nil {
		t.Fatal(err)
	}
	auth.Signature = "0x" + toHex(sig)
	b, _ := json.Marshal(auth)
	return AuthScheme + " " + base64.StdEncoding.EncodeToString(b)
}

func mustKey(t *testing.T, bw *BaseWallet) []byte {
	t.Helper()
	k, ok := decodeKey(bw.PrivateKey)
	if !ok {
		t.Fatal("bad test key")
	}
	return k
}
