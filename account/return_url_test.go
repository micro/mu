package account

// Where a payment sends somebody back to.
//
// Mu runs behind a reverse proxy that forwards to a loopback port, so r.Host is
// "localhost:8081" and any URL built from it names an address no client can
// reach. Subscribing did exactly that: it took the money, fired the webhook,
// recorded the plan, and returned the customer to https://localhost:8081/account.
//
// The failure is entirely on the way back, which is what let it stand. Nothing
// is wrong except the one screen that tells somebody it worked, so no log, no
// error and no ledger entry disagrees.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No handler here may assemble a public URL from the request's own host.
//
// A source check rather than a probe, because the bug is a handler doing its
// own thing: a test that exercised the two we know about would say nothing
// about the third somebody adds. internal/origin is the one answer, reached
// through app.BaseURL.
func TestNoHandlerBuildsAReturnURLFromTheRequestHost(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`+ r.Host`, `+r.Host`, `"://" + r.Host`} {
			if strings.Contains(string(src), bad) {
				t.Errorf("%s builds a URL from r.Host, which behind the proxy is "+
					"localhost — use app.BaseURL, which is internal/origin", f)
			}
		}
	}
}

// Topping up returns somewhere, and that somewhere must be this instance's
// public address rather than whatever host the request happened to carry.
func TestTheTopUpReturnURLComesFromOrigin(t *testing.T) {
	src, err := os.ReadFile("balance.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "app.BaseURL(r)") {
		t.Error("no Stripe flow asks app.BaseURL where this instance is, so a " +
			"return URL is being built from something else")
	}
}
