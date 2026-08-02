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
		"Bearer YOUR_TOKEN",    // token-first
		`href="/token"`,        // and a way to get one
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the connect step is missing %q", want)
		}
	}

	// Paying per call is the alternative, not the headline: the token route has
	// to come first in the page.
	tokenAt, x402At := strings.Index(body, "/token"), strings.Index(body, "#paying")
	if tokenAt < 0 || x402At < 0 || tokenAt > x402At {
		t.Error("x402 leads over the token route; a person signing in takes the token route")
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
