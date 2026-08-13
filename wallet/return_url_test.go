package wallet

// Where a payment sends somebody back to.
//
// Mu runs behind a reverse proxy that forwards to a loopback port, so r.Host is
// "localhost:8081" and any URL built from it names an address no client can
// reach. Subscribing did exactly that: it took the money, fired the webhook,
// recorded the plan, and returned the customer to https://localhost:8081/wallet.
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

// Both Stripe flows and the portal must agree about where "back" is, and the
// only way to be sure is that they all ask the same function.
func TestEveryStripeReturnURLComesFromTheSamePlace(t *testing.T) {
	src, err := os.ReadFile("handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)

	// One per flow: subscribe, one-off top-up, and the billing portal.
	if n := strings.Count(text, "app.BaseURL(r)"); n < 3 {
		t.Errorf("only %d of the Stripe flows ask app.BaseURL where this instance "+
			"is — subscribe, top-up and the portal all return somewhere and all "+
			"have to agree", n)
	}
}
