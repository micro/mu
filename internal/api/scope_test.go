package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/service"
)

func TestParseScope(t *testing.T) {
	for raw, want := range map[string]string{
		"news,web,mail":   "news web mail",
		"news, web , web": "news web",
		"News,WEB":        "news web",
		"news web":        "news web",
		"":                "",
		" , ,":            "",
	} {
		if got := strings.Join(parseScope(raw), " "); got != want {
			t.Errorf("parseScope(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A scope names services, because that is the unit a person thinks in.
func TestScopeMatchesServicesToolsAndLabels(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		scope string
		want  bool
	}{
		{"news_list", "news", true},
		{"news_list", "news,web", true},
		{"web_search", "news", false},
		{"web_search", "web", true},
		// Naming one tool takes that tool, not its whole service.
		{"web_search", "web_search", true},
		{"web_fetch", "web_search", false},
		// A tool with no service behind it is only in scope when named.
		{"agent", "news", false},
		{"agent", "agent", true},
		// No scope is everything.
		{"prayer_qibla", "", true},
	} {
		got := inScope(Tool{Name: tc.tool}, parseScope(tc.scope))
		if got != tc.want {
			t.Errorf("inScope(%s, %q) = %v, want %v", tc.tool, tc.scope, got, tc.want)
		}
	}
}

// A service's nav label is what the sidebar calls it, and what somebody is
// likely to type: the web service is "Search" on the page, so ?tools=search
// should reach it. That path reads the registry, so this registers a probe
// rather than asserting against whatever happens to be loaded.
func TestScopeMatchesTheNavLabel(t *testing.T) {
	if err := service.Register(service.Spec{
		Name: "probesvc", Label: "Widgets", Handler: new(ScopeProbe),
	}); err != nil {
		t.Fatalf("registering the probe service: %v", err)
	}

	if !inScope(Tool{Name: "probesvc_list"}, parseScope("widgets")) {
		t.Error("a scope naming the service's label did not match it")
	}
	if !inScope(Tool{Name: "probesvc_list"}, parseScope("probesvc")) {
		t.Error("a scope naming the service itself did not match it")
	}
	if inScope(Tool{Name: "probesvc_list"}, parseScope("something-else")) {
		t.Error("an unrelated scope matched")
	}
}

// ScopeProbe is a handler that exists only to register a service, so the
// label-matching path can be tested against a known label rather than whatever
// happens to be loaded. Exported with one exported RPC-shaped method because
// go-micro will not register anything less.
type ScopeProbe struct{}

type ProbeRequest struct{}
type ProbeResponse struct {
	Text string `json:"text"`
}

func (ScopeProbe) List(_ context.Context, _ *ProbeRequest, rsp *ProbeResponse) error {
	rsp.Text = "probe"
	return nil
}

// The point of the feature: a client that asked for news is not shown a qibla
// compass, and the list it does get is materially shorter.
func TestScopedListIsShorter(t *testing.T) {
	full := listTools(t, "/mcp")
	scoped := listTools(t, "/mcp?tools=news")

	if len(full) == 0 || len(scoped) == 0 {
		t.Fatalf("expected tools in both lists, got %d and %d", len(full), len(scoped))
	}
	if len(scoped) >= len(full) {
		t.Errorf("scoping to news listed %d of %d tools — no narrowing happened", len(scoped), len(full))
	}
	for _, name := range scoped {
		if !strings.HasPrefix(name, "news") {
			t.Errorf("a news-scoped connection was shown %q", name)
		}
	}
}

// An unrecognised name is ignored, not fatal. Failing a whole connection
// because somebody typed "email" for "mail" would be worse than giving them
// the rest.
func TestUnknownScopeNamesAreIgnored(t *testing.T) {
	got := listTools(t, "/mcp?tools=news,not-a-service")
	if len(got) == 0 {
		t.Fatal("an unknown name in the scope emptied the whole list")
	}
	for _, name := range got {
		if !strings.HasPrefix(name, "news") {
			t.Errorf("unexpected tool %q", name)
		}
	}
}

// Scoping is about what an agent considers, not what it may do. A tool left out
// of the listing is still callable by name — the guards that matter are account,
// credits and rate limits, and none of them should start depending on a query
// parameter the caller chose.
func TestScopingDoesNotBlockCalls(t *testing.T) {
	// blog_read is registered statically in this package. prayer_qibla is not —
	// it comes from main.go, so naming it here tested nothing but its absence.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"blog_read","arguments":{"id":"nope"}}}`
	rec := httptest.NewRecorder()
	MCPHandler(rec, httptest.NewRequest("POST", "/mcp?tools=news", strings.NewReader(body)))

	if strings.Contains(rec.Body.String(), "Tool not found") {
		t.Errorf("a tool outside the scope became uncallable: %s", rec.Body.String())
	}
}

// The picker on the page is built from the registry, so a new service appears
// in it the moment it registers.
func TestScopeServicesComeFromTheRegistry(t *testing.T) {
	got := ScopeServices()
	if len(got) == 0 {
		t.Skip("no services registered in this package's tests")
	}
	seen := map[string]bool{}
	for _, s := range got {
		if seen[s] {
			t.Errorf("%q listed twice", s)
		}
		seen[s] = true
	}
}

func listTools(t *testing.T, url string) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	MCPHandler(rec, httptest.NewRequest("POST", url,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))

	var out struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("tools/list did not parse: %v", err)
	}
	names := make([]string, 0, len(out.Result.Tools))
	for _, tool := range out.Result.Tools {
		names = append(names, tool.Name)
	}
	return names
}
