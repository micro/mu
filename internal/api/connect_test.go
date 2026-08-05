package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The narrative has to actually connect: someone convinced by the landing needs
// a next move, and "point your agent at this" is it. Browsing a catalogue with
// no way to connect was the gap.
func TestToolsPageTellsYouHowToConnect(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	rec := httptest.NewRecorder()
	ToolsPageHandler(rec, httptest.NewRequest("GET", "/tools", nil))
	body := rec.Body.String()

	for _, want := range []string{
		`id="connect"`,         // the step exists
		"Connect your agent",   // and says what it is
		"mcpServers",           // a config to paste
		"https://micro.mu/mcp", // with this instance's real URL, not a placeholder
		"Authorization",        // and the header filled in, because a token is
		"Bearer YOUR_TOKEN",    // what an agent actually holds
		`href="/signup"`,       // a visitor with no account starts there
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the connect step is missing %q", want)
		}
	}
}

// An agent has no account to sign into — somebody issues it a credential. The
// page led with a token for that reason, and then described the OAuth
// alternative as the client "signing itself in", which is not what happens: the
// thing signing in is the person, in a browser the client opens.
//
// So if OAuth is mentioned it has to be called OAuth and it has to say what
// occurs, and the token has to be the path with the config next to it.
func TestConnectNamesOAuthAndLeadsWithTheToken(t *testing.T) {
	connect := connectSection(httptest.NewRequest("GET", "/tools", nil))

	if strings.Contains(connect, "sign itself in") || strings.Contains(connect, "signs itself in") {
		t.Error("the page still says a client signs itself in; the person signs in")
	}
	if !strings.Contains(connect, "OAuth") {
		t.Error("the second way in is not named")
	}
	if !strings.Contains(connect, "open a browser") {
		t.Error("the OAuth line does not say what actually happens")
	}
	// The token is the documented path, so its header is in the config block
	// rather than described in prose underneath it.
	if strings.Index(connect, "Bearer YOUR_TOKEN") > strings.Index(connect, "OAuth") {
		t.Error("OAuth is presented before the token")
	}
}

// One way in. Offering a second payment rail alongside credits, or a second
// protocol alongside MCP, is the thing this step stopped doing — a reader
// choosing between two of everything is a reader who connects to neither.
//
// This reads connectSection directly rather than the rendered page. The tool
// tiles below it carry whatever the tools say about themselves — `pay` really
// does settle in USDC on somebody else's server — and that is a description, not
// an on-ramp. Asserting over the whole page would either fail on those or, worse,
// pass here and not in the running binary, where main.go has registered them.
func TestConnectOffersOneWayIn(t *testing.T) {
	connect := connectSection(httptest.NewRequest("GET", "/tools", nil))

	for _, gone := range []string{"x402", "USDC", "HTTP 402", `href="/api"`} {
		if strings.Contains(connect, gone) {
			t.Errorf("the connect step still offers %q as a second way in", gone)
		}
	}
}

// The config must name the instance you are reading, not a hardcoded host — a
// self-hoster pasting micro.mu into their own client is a dead end.
func TestConnectConfigUsesThisInstance(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")

	rec := httptest.NewRecorder()
	ToolsPageHandler(rec, httptest.NewRequest("GET", "/tools", nil))
	body := rec.Body.String()

	if !strings.Contains(body, "https://example.test/mcp") {
		t.Error("the config does not point at this instance")
	}
	if strings.Contains(body, "https://micro.mu/mcp") {
		t.Error("the config hardcodes micro.mu")
	}
}
