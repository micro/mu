package api

// The doors that are not MCP, and the hand-over that supplies them.

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Given the rows, the connect card draws them.
//
// They were on /contact, which is a card about writing to an assistant the way
// you write to a person, and they ended it in a shell snippet. This is the page
// the CLI row has pointed at since it was written — and this page said nothing
// about the CLI at all, so the one client with real documentation sent you
// somewhere that did not have it.
func TestTheConnectCardCarriesTheDoorsThatAreNotMCP(t *testing.T) {
	prev := DevClientsFunc
	DevClientsFunc = func() []DevClient {
		return []DevClient{
			{Address: `mu ask "…"`, Note: "from a terminal"},
			{Address: "https://example.test/agent", Note: "needs a token",
				Example: "curl -X POST https://example.test/agent"},
		}
	}
	t.Cleanup(func() { DevClientsFunc = prev })

	got := connectSection(httptest.NewRequest("GET", "/tools", nil))
	for _, want := range []string{"mu ask", "from a terminal", "curl -X POST"} {
		if !strings.Contains(got, want) {
			t.Errorf("the connect card does not carry %q", want)
		}
	}
}

// And with nothing handed over it draws nothing, rather than an empty heading.
//
// nil is a real state: this package is underneath the product and cannot fetch
// the rows itself, so any build that has not wired the assembly has none.
func TestWithNoDoorsHandedOverThereIsNoSection(t *testing.T) {
	prev := DevClientsFunc
	DevClientsFunc = nil
	t.Cleanup(func() { DevClientsFunc = prev })

	if got := connectSection(httptest.NewRequest("GET", "/tools", nil)); strings.Contains(got, "talk to the agent directly") {
		t.Errorf("a heading with nothing under it:\n%s", got)
	}
}

// The server actually hands them over.
//
// A hand-over nobody assigns is a feature that compiles, renders nothing, and
// has a comment explaining what it does. The consuming side above proves the
// drawing; only the assembly can prove the supply, and it is asserted against
// the source because calling the route setup means standing up the server.
func TestTheServerWiresTheDoors(t *testing.T) {
	b, err := os.ReadFile("../server/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "api.DevClientsFunc = ") {
		t.Error("nothing assigns api.DevClientsFunc, so /tools draws no CLI and no API " +
			"however well this package renders them")
	}
}
