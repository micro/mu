package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// One name for a search term, across every tool that takes one.
//
// This was found by using the endpoint rather than reading it: learn "query"
// from news_search, call web_search with it, get "No query provided." Both
// tools advertised a schema that was correct for themselves — news_search,
// index_search, social_search and video_search took "query"; web_search,
// apps_search, images_search, places_search and quran_search took "q" — so
// there was nothing a caller could inspect to find the rule, and the rule was
// that there wasn't one.
func TestSearchToolsAgreeOnTheParamName(t *testing.T) {
	for _, tool := range sortedTools() {
		for _, p := range tool.Params {
			if p.Name == "q" {
				t.Errorf("%s takes %q; the search term is called \"query\" everywhere else", tool.Name, p.Name)
			}
		}
	}
}

// The old name still works, because clients and configs already use it.
func TestTheOldParamNameStillResolves(t *testing.T) {
	tool := Tool{
		Name:   "probe_search",
		Params: []ToolParam{{Name: "query", Type: "string", Required: true}},
	}

	got := normaliseArgs(tool, map[string]any{"q": "hello"})
	if got["query"] != "hello" {
		t.Errorf(`a caller sending "q" did not reach the handler: %v`, got)
	}

	// A supplied value is never overwritten by the alias.
	got = normaliseArgs(tool, map[string]any{"query": "real", "q": "stale"})
	if got["query"] != "real" {
		t.Errorf("the alias overwrote the caller's own value: %v", got)
	}
}

// A missing required parameter is an error, not a successful string.
//
// web_search answered a wrong call with HTTP 200 and the text "No query
// provided." — indistinguishable, to an agent, from a search that found
// nothing. One of those is worth retrying differently and the other is not.
func TestMissingRequiredParamIsAnError(t *testing.T) {
	tool := Tool{
		Name:   "probe_search",
		Params: []ToolParam{{Name: "query", Type: "string", Required: true}},
	}

	for _, args := range []map[string]any{nil, {}, {"query": ""}, {"query": "   "}} {
		if missingRequired(tool, args) != "query" {
			t.Errorf("args %v were accepted despite no query", args)
		}
	}
	if got := missingRequired(tool, map[string]any{"query": "something"}); got != "" {
		t.Errorf("a valid call was refused for %q", got)
	}
}

// End to end through the dispatcher: the error must reach the caller flagged as
// one, not as content.
func TestToolCallWithNoQueryReportsAnError(t *testing.T) {
	RegisterTool(Tool{
		Name:        "probe_needs_query",
		Description: "probe",
		Params:      []ToolParam{{Name: "query", Type: "string", Required: true}},
		Handle: func(map[string]any) (string, error) {
			t.Error("the handler ran despite a missing required param")
			return "", nil
		},
	})

	text, isErr, err := ExecuteTool(httptest.NewRequest("POST", "/mcp", nil),
		"probe_needs_query", map[string]any{})
	if !isErr || err == nil {
		t.Fatalf("a call with no query succeeded: text=%q isErr=%v err=%v", text, isErr, err)
	}
	if !strings.Contains(text, "query") {
		t.Errorf("the error does not name the missing param: %q", text)
	}
}
