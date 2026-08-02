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
		`href="/token"`,        // and a fallback for clients that can't sign in
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the connect step is missing %q", want)
		}
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
