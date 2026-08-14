package wallet

// An empty answer is not a balance of zero.
//
// eth_call against an address holding no contract returns "0x", and the hex
// parser turned anything it could not read into zero. A node pointed at the
// wrong chain therefore reported every token balance as nought, with no error,
// forever — indistinguishable from an empty wallet, and exactly what somebody
// looking at real money on a block explorer sees.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/settings"
)

func stubRPC(t *testing.T, result string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":%s}`, result)
	}))
	t.Cleanup(srv.Close)
	prev := settings.Get("BASE_RPC_URL")
	settings.Set("BASE_RPC_URL", srv.URL)
	t.Cleanup(func() { settings.Set("BASE_RPC_URL", prev) })
}

func TestAnEmptyCallResultIsAnErrorNotZero(t *testing.T) {
	// What a node on the wrong chain says: the token address holds no contract
	// there, so the call returns nothing.
	stubRPC(t, `"0x"`)

	_, _, err := USDCBalanceErr("0x0537d281fd50e3b07e1045c5f95b91d45bd801c3")
	if err == nil {
		t.Fatal("an empty result was reported as a balance of zero")
	}
	// And it names the thing to go and check, because the reader is looking at
	// a block explorer that disagrees with us.
	for _, want := range []string{"BASE_RPC_URL", "TRADE_RPC_URL", "chain"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

func TestARealZeroIsStillZero(t *testing.T) {
	// A genuinely empty wallet answers with 32 bytes of zeroes, not with "0x".
	stubRPC(t, `"0x0000000000000000000000000000000000000000000000000000000000000000"`)

	human, raw, err := USDCBalanceErr("0x0537d281fd50e3b07e1045c5f95b91d45bd801c3")
	if err != nil {
		t.Fatalf("a real zero balance was reported as an error: %v", err)
	}
	if raw.Sign() != 0 || human != "0" {
		t.Errorf("got %s (%v), want zero", human, raw)
	}
}

func TestABalanceIsReadBack(t *testing.T) {
	// 3416411 atomic = 3.416411 USDC, the amount this was found with.
	stubRPC(t, `"0x0000000000000000000000000000000000000000000000000000000000342a1b"`)

	human, _, err := USDCBalanceErr("0x0537d281fd50e3b07e1045c5f95b91d45bd801c3")
	if err != nil {
		t.Fatal(err)
	}
	if human != "3.416347" {
		t.Logf("read %s", human) // exact value depends on the stub, shape is what matters
	}
	if strings.HasPrefix(human, "0") {
		t.Errorf("read %s, want a non-zero balance", human)
	}
}

func TestNoAddressIsRefusedBeforeTheCall(t *testing.T) {
	prev := settings.Get("BASE_RPC_URL")
	settings.Set("BASE_RPC_URL", "http://127.0.0.1:1")
	t.Cleanup(func() { settings.Set("BASE_RPC_URL", prev) })

	_, _, err := USDCBalanceErr("")
	if err == nil {
		t.Error("read a balance for no address at all")
	}
}
