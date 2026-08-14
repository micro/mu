package wallet

// The QR has to name the chain.
//
// It encoded a bare address, and a bare EVM address names no chain — so a
// wallet scanning it stays on whatever network it was already on, which is
// Ethereum mainnet for almost everybody. USDC then arrives at this same address
// on the wrong chain, where this instance cannot see it or move it. That
// happened with real money.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

func topupCard(t *testing.T, id string) string {
	t.Helper()
	prev := settings.Get("CRYPTO_TOPUP")
	settings.Set("CRYPTO_TOPUP", "true")
	t.Cleanup(func() { settings.Set("CRYPTO_TOPUP", prev) })
	return cryptoWalletCard(id)
}

func TestTheQRNamesTheChainAndTheToken(t *testing.T) {
	out := topupCard(t, "qrtest")
	if out == "" {
		t.Skip("no card rendered")
	}

	// EIP-681: ethereum:<token>@<chainId>/transfer?address=<recipient>
	if !strings.Contains(out, "data-uri=\"ethereum:") {
		t.Fatalf("the QR does not carry a payment URI:\n%s", out[:400])
	}
	for _, want := range []string{
		baseUSDC, // the token, so the wallet does not have to be told
		"@8453",  // the chain, which is the whole point
		"/transfer?address=0x",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the payment URI is missing %q", want)
		}
	}
}

func TestTheQRIsBuiltFromTheURINotTheBareAddress(t *testing.T) {
	out := topupCard(t, "qrtest")
	if out == "" {
		t.Skip("no card rendered")
	}
	// The script must prefer data-uri. Reading data-addr first would put us
	// straight back where we started.
	i := strings.Index(out, "var data=")
	if i < 0 {
		t.Fatal("the QR script no longer chooses its payload")
	}
	line := out[i:]
	if end := strings.Index(line, "\n"); end > 0 {
		line = line[:end]
	}
	uriAt, addrAt := strings.Index(line, "data-uri"), strings.Index(line, "data-addr")
	if uriAt < 0 {
		t.Fatal("the QR no longer reads the payment URI")
	}
	if addrAt >= 0 && addrAt < uriAt {
		t.Error("the QR prefers the bare address over the payment URI")
	}
}

func TestTheCardSaysBaseOnlyWhereItCanBeSeen(t *testing.T) {
	out := topupCard(t, "qrtest")
	if out == "" {
		t.Skip("no card rendered")
	}
	if !strings.Contains(out, "Base network only") {
		t.Error("the card does not state the network as a warning")
	}
	// And says what happens if you get it wrong, which is the part somebody
	// only wants to read once.
	if !strings.Contains(out, "cannot") || !strings.Contains(out, "Ethereum") {
		t.Error("the card does not say what sending on another chain costs")
	}

	// The warning has to come before the address, or it is read after the
	// address has already been copied.
	warn := strings.Index(out, "Base network only")
	addr := strings.Index(out, `class="cw-addr"`)
	if warn < 0 || addr < 0 || warn > addr {
		t.Error("the network warning sits below the address")
	}
}
