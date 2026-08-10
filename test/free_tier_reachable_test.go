package test

// The front door has to open.
//
// /mcp is a public endpoint and four tools are priced at zero on purpose, so an
// agent that finds this instance mid-task should be able to try one without an
// account. It could not: the payment gate fired on "this tool has a wallet
// operation" rather than "this tool costs something", and every free tool
// answered an anonymous caller with an x402 challenge for USDC on Base.
//
// Signed-in callers and token holders were never affected, which is why it went
// unnoticed — everyone testing the thing had a token. What was broken was the
// no-signup path, which is the claim on the landing page.
//
// Two independent checks now have to fail before that can happen again: the
// gate asks wallet.Metered, and the challenge builder refuses to invent a price
// for a free operation. This test holds both in the source, because the
// behaviour itself needs a live HTTP stack to observe.

import (
	"os"
	"strings"
	"testing"
)

func TestThePaymentGateAsksWhetherTheToolCostsAnything(t *testing.T) {
	body := registrationSource(t)

	if !strings.Contains(body, `op != "" && billing.Metered(op)`) {
		t.Error("the x402 gate no longer asks whether the tool costs anything, " +
			"so free tools are paywalled to anonymous callers again")
	}
	// And when the challenge declines to be written, the request continues
	// rather than being refused with an empty 402.
	if !strings.Contains(body, "if billing.WritePaymentRequired(w, op, resource) {") {
		t.Error("the gate ignores whether a challenge was actually written")
	}
}

// A refused call is still a call. Recording happens inside the dispatcher,
// which the gate returns before reaching, so every refusal was missing from the
// figures — including the free ones being refused by mistake.
func TestRefusedCallsAreCounted(t *testing.T) {
	if !strings.Contains(registrationSource(t), `usage.Record("mcp-refused"`) {
		t.Error("calls turned away at the payment gate are invisible in usage, " +
			"so a gate refusing the wrong things cannot be seen doing it")
	}
}

// "You cannot post yet" belongs nowhere near somebody's money.
func TestTheVerifyBannerStaysOffTheWalletPages(t *testing.T) {
	src, err := os.ReadFile("../internal/app/app.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if !strings.Contains(body, `strings.HasPrefix(p, "/wallet/")`) {
		t.Error("the verify banner is exempted by exact path again, so /wallet/transfer " +
			"tells someone moving their own credit that they cannot post")
	}
}

// Free and open are different questions, and the tools that are one but not
// the other have to say so where a client can see it.
//
// web_fetch makes a request to any URL a caller names, on this server's behalf.
// video_search spends a YouTube quota shared across everyone. Both cost us
// nothing, so the price gate lets them through — and both were then refused one
// layer later by their own handler, having been advertised as available. They
// are marked AccountOnly so the refusal happens at the MCP layer, as a 401
// pointing at sign-in.
func TestFreeButAccountableToolsSaySoAtTheMCPLayer(t *testing.T) {
	src := registrationSource(t)
	for _, tool := range []string{"video_search", "web_fetch"} {
		i := strings.Index(src, `Name:        "`+tool+`"`)
		if i < 0 {
			t.Fatalf("%s is no longer registered anywhere", tool)
		}
		// The flag has to be inside this tool's own block, not somewhere else
		// in the file.
		block := src[i:min(i+1400, len(src))]
		if !strings.Contains(block, "AccountOnly: true") {
			t.Errorf("%s is offered to anonymous callers and then refused by its "+
				"own handler one layer later", tool)
		}
	}
}
