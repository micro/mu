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
		`id="connect"`,           // the step exists
		"Connect your agent",     // and says what it is
		"mcpServers",             // a config to paste
		"https://micro.mu/mcp",   // with this instance's real URL, not a placeholder
		"Authorization",          // and the header filled in, because a token is
		"Bearer ${env:MU_TOKEN}", // what a config-file client actually holds
		`href="/signup"`,         // a visitor with no account starts there
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the connect step is missing %q", want)
		}
	}
}

// The two ways in are split by client, because that is how the world divides.
//
// Cursor reads a url and headers out of mcp.json, so it takes a token. Claude
// Desktop's config file only validates stdio servers, so a url pasted there
// silently does nothing — it connects through Settings > Connectors, which runs
// OAuth and has nowhere to put a token.
//
// The page used to show one config and call OAuth the fallback for a client
// that "can't sign itself in". Nothing signs itself in — a browser opens and
// the person does — and for Claude Desktop OAuth is not the fallback, it is the
// only way in. Someone following that copy would have edited a JSON file and
// watched it do nothing.
func TestConnectSplitsTheTwoClientPaths(t *testing.T) {
	connect := connectSection(httptest.NewRequest("GET", "/tools", nil))

	if strings.Contains(connect, "sign itself in") || strings.Contains(connect, "signs itself in") {
		t.Error("the page still says a client signs itself in; the person signs in")
	}
	for _, want := range []string{
		"Cursor",                     // named, with the file it reads
		".cursor/mcp.json",           //
		"Claude Desktop",             // named, with the route that works
		"Connectors",                 //
		"claude_desktop_config.json", // and the one that does not
	} {
		if !strings.Contains(connect, want) {
			t.Errorf("the connect step is missing %q", want)
		}
	}
	// The token config belongs to the client that can use one, and the Claude
	// Desktop path must not imply a token goes anywhere.
	if strings.Index(connect, "MU_TOKEN") > strings.Index(connect, "Claude Desktop") {
		t.Error("the token config is presented under the client that cannot take a token")
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
