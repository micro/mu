package api

import (
	"strings"
	"testing"
)

// commandProbe registers a small set of tools shaped like the real ones, so the
// resolution rules can be tested without standing up the server.
func commandProbe(t *testing.T) {
	t.Helper()
	for _, tool := range []Tool{
		{Name: "probe_list", Description: "List things"},
		{Name: "probe_search", Description: "Search things", Params: []ToolParam{
			{Name: "query", Type: "string", Description: "What to search for", Required: true},
		}},
		{Name: "probe", Aliases: []string{"probealias"}, Description: "The probe itself"},
		{Name: "probechat", Description: "Chat", Params: []ToolParam{
			{Name: "prompt", Type: "string", Description: "Your question", Required: true},
		}},
		{Name: "probe_at", Description: "Needs two things", Params: []ToolParam{
			{Name: "lat", Type: "number", Description: "Latitude", Required: true},
			{Name: "lon", Type: "number", Description: "Longitude", Required: true},
		}},
	} {
		if _, exists := Lookup(tool.Name); !exists {
			RegisterTool(tool)
		}
	}
}

// "news list" and "news_list" are the same tool. Both spellings have to reach
// it, because one is how a person talks and the other is what every existing
// script and document says.
func TestResolveReadsBothSpellings(t *testing.T) {
	commandProbe(t)

	cases := []struct {
		words    []string
		wantTool string
		wantRest []string
	}{
		{[]string{"probe", "list"}, "probe_list", []string{}},
		{[]string{"probe_list"}, "probe_list", []string{}},
		{[]string{"probe", "search", "cats"}, "probe_search", []string{"cats"}},
		{[]string{"probe"}, "probe", []string{}},
		// A word that is not a method leaves the one-word tool with an argument.
		{[]string{"probe", "cats"}, "probe", []string{"cats"}},
	}
	for _, tc := range cases {
		got, rest, ok := Resolve(tc.words)
		if !ok {
			t.Errorf("Resolve(%v) found nothing", tc.words)
			continue
		}
		if got.Name != tc.wantTool || strings.Join(rest, " ") != strings.Join(tc.wantRest, " ") {
			t.Errorf("Resolve(%v) = %s %v, want %s %v", tc.words, got.Name, rest, tc.wantTool, tc.wantRest)
		}
	}

	if _, _, ok := Resolve([]string{"nosuchthing"}); ok {
		t.Error("resolved a tool that does not exist")
	}
}

func TestArgsPlacesWordsAndFlags(t *testing.T) {
	commandProbe(t)

	search, _ := Lookup("probe_search")
	args, ok := Args(search, []string{"open", "source", "email"})
	if !ok || args["query"] != "open source email" {
		t.Errorf("loose words did not become the query: %v ok=%v", args, ok)
	}

	args, ok = Args(search, []string{"--query", "bitcoin"})
	if !ok || args["query"] != "bitcoin" {
		t.Errorf("flag form failed: %v ok=%v", args, ok)
	}

	list, _ := Lookup("probe_list")
	if _, ok := Args(list, []string{"junk"}); ok {
		t.Error("a tool with no parameters accepted a loose word")
	}
}

// A tool with two required parameters has nowhere to put a loose word, and
// saying so is what lets a chat bot hand "/weather london" to the agent — which
// can turn a place into a latitude — instead of failing.
func TestToolsWithNoFreeTextParameterRefuseLooseWords(t *testing.T) {
	commandProbe(t)

	at, _ := Lookup("probe_at")
	if _, ok := Args(at, []string{"london"}); ok {
		t.Error("a two-parameter tool swallowed a bare word")
	}
	if args, ok := Args(at, []string{"--lat", "51.5", "--lon", "-0.12"}); !ok || !Ready(at, args) {
		t.Errorf("explicit flags should satisfy it: %v ok=%v", args, ok)
	}
}

// Ready is what stops a bot calling a tool that cannot run yet.
func TestReadyChecksRequiredParameters(t *testing.T) {
	commandProbe(t)

	at, _ := Lookup("probe_at")
	if Ready(at, map[string]any{"lat": 51.5}) {
		t.Error("half the required parameters counted as ready")
	}
	if !Ready(at, map[string]any{"lat": 51.5, "lon": -0.12}) {
		t.Error("both parameters should be ready")
	}

	list, _ := Lookup("probe_list")
	if !Ready(list, nil) {
		t.Error("a tool with no required parameters is always ready")
	}
}

func TestServiceAndMethod(t *testing.T) {
	if Service("news_list") != "news" || Method("news_list") != "list" {
		t.Error("service_method split is wrong")
	}
	if Service("agent") != "agent" || Method("agent") != "" {
		t.Error("a one-word tool is its own service with no method")
	}
}
