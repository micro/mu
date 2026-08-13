package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The landing has to let you use the thing, then lead somewhere.
//
// It used to be three buttons and three cards: everything about the tools and
// nothing you could do with them. "See it working" was a link — a promise
// rather than a demonstration, and one more click between arriving and seeing
// anything work.
//
// ollama is the shape to copy. Its product is running models locally, and what
// makes it land is that `ollama run llama3` puts you in a conversation on the
// first command, because nobody wants a model, they want to use one. Nobody
// wants tools either. So the landing carries a working input, and a guest gets
// three queries a day against the public services.
func TestTheLandingLetsYouUseItBeforeExplainingIt(t *testing.T) {
	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	// The demonstration itself: a real input, wired to the agent.
	if !strings.Contains(body, "mu-chat-form") && !strings.Contains(body, "mu-chat-input") {
		t.Error("the landing has nothing to type into — it describes the tools " +
			"without letting anybody use one")
	}
	// And it comes before the explanation, because the explanation is the
	// caption.
	try, cards := strings.Index(body, `class="ltry"`), strings.Index(body, `class="lcards"`)
	if try < 0 || (cards >= 0 && try > cards) {
		t.Error("the pitch comes before the thing you can try")
	}

	// Still leads on: what is here, and how to point a client at it.
	for _, want := range []string{`href="/tools"`, `href="/mcp"`, `href="/signup"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the landing has no path to %s", want)
		}
	}

	// "See it working" as a link is what the input replaced. If it comes back,
	// it should not point at the card screen, which does not show tool-calling.
	if strings.Contains(body, `href="/home">See it working`) {
		t.Error(`"See it working" points at the home screen, which shows cards rather than tool-calling`)
	}
}

// The landing leads with how to connect, never with how to pay. It once
// explained a payment protocol before it explained the endpoint, which left a
// reader choosing a rail when all they wanted was the URL.
//
// This used to be enforced by banning the words outright. That was the right
// guard for the wrong rule: paying per call is a real way in, and the only one
// an agent with no human attached can use, so the page is worse for not
// mentioning it. What must not come back is the ordering — the endpoint first,
// the rail after it, for a reader who has already decided to connect.
func TestLandingLeadsWithConnectingNotPaying(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	rec := httptest.NewRecorder()
	Landing(rec, httptest.NewRequest("GET", "/", nil))
	body := rec.Body.String()

	endpoint := strings.Index(body, "https://micro.mu/mcp")
	if endpoint < 0 {
		t.Fatal("the landing does not name the endpoint to connect to")
	}
	for _, rail := range []string{"x402", "USDC", "Payment Required"} {
		if i := strings.Index(body, rail); i >= 0 && i < endpoint {
			t.Errorf("%q is explained before the endpoint is named, so a reader "+
				"meets a payment rail while still looking for the URL", rail)
		}
	}
}
