package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every listed tool carries a title and annotations.
//
// A client decides whether to confirm a call, whether it may retry one, and
// what to render on the button, from these. Mu published none of them while
// already knowing all of them, and the Anthropic Connectors Directory checks
// for them at submission.
func TestToolsListCarriesTitlesAndAnnotations(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	rec := httptest.NewRecorder()
	MCPHandler(rec, httptest.NewRequest("POST", "/mcp", strings.NewReader(body)))

	var out struct {
		Result struct {
			Tools []struct {
				Name        string          `json:"name"`
				Title       string          `json:"title"`
				Annotations *mcpAnnotations `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("tools/list did not parse: %v", err)
	}
	if len(out.Result.Tools) == 0 {
		t.Fatal("no tools listed")
	}
	for _, tool := range out.Result.Tools {
		if tool.Title == "" {
			t.Errorf("%s has no title", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("%s has no annotations", tool.Name)
		}
		if len(tool.Name) > 64 {
			t.Errorf("%s is longer than the 64 characters a client may assume", tool.Name)
		}
	}
}

// Reads and writes must be distinguishable, or the annotations are decoration.
func TestReadsAndWritesAreAnnotatedApart(t *testing.T) {
	for _, tc := range []struct {
		tool                  string
		readOnly, destructive bool
	}{
		{"news_list", true, false},
		{"web_search", true, false},
		{"mail_inbox", true, false},
		{"mail_send", false, false},
		{"blog_create", false, false},
		{"blog_delete", false, true},
		{"db_delete", false, true},
	} {
		a := annotate(Tool{Name: tc.tool})
		if a.ReadOnlyHint != tc.readOnly {
			t.Errorf("%s readOnlyHint = %v, want %v", tc.tool, a.ReadOnlyHint, tc.readOnly)
		}
		if a.DestructiveHint != tc.destructive {
			t.Errorf("%s destructiveHint = %v, want %v", tc.tool, a.DestructiveHint, tc.destructive)
		}
		if a.ReadOnlyHint && a.DestructiveHint {
			t.Errorf("%s is annotated as both read-only and destructive", tc.tool)
		}
	}
}

// The title says the same word the sidebar does, so a person meets one name for
// a service rather than two.
func TestTitlesUseTheServiceLabel(t *testing.T) {
	if got := annotate(Tool{Name: "agent"}).Title; got != "Agent" {
		t.Errorf("a bare name should title from itself, got %q", got)
	}
	if got := annotate(Tool{Name: "saved_list"}).Title; got != "Saved list" {
		t.Errorf("a two-part name with no service behind it should read whole, got %q", got)
	}
	// The card heading is a noun for a dashboard panel; a tool title is an
	// action. Taking the first for the second gave news_list the title "News".
	if got := annotate(Tool{Name: "news_list", Title: "News"}).Title; got == "News" {
		t.Error("the card heading was used as the tool title")
	}
}

// A response that does not parse must come back whole rather than be dropped.
func TestEnrichLeavesUnexpectedResponsesAlone(t *testing.T) {
	for _, body := range []string{
		`not json`,
		`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"nope"}}`,
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
	} {
		if got := string(enrichToolsList([]byte(body))); got != body {
			t.Errorf("enrich altered a response it does not understand:\n in: %s\nout: %s", body, got)
		}
	}
}
