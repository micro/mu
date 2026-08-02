package stream

import (
	"strings"
	"testing"
)

// The agent answers in markdown. Before this, its replies arrived in the
// console as literal ** and - characters.
func TestAgentMarkdownIsRendered(t *testing.T) {
	e := &Event{Type: TypeAgent, Author: "micro", Content: "Here's **the plan**:\n\n- ship it\n- watch it"}
	out := renderEvent(e, "")

	for _, want := range []string{"<strong>the plan</strong>", "<li>ship it</li>", "<ul>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in rendered output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "**the plan**") {
		t.Error("markdown left literal")
	}
}

// A one-line message should not gain a paragraph wrapper — the console is
// conversational, and the bubble should stay tight.
func TestSingleLineIsNotWrappedInParagraph(t *testing.T) {
	e := &Event{Type: TypeUser, Author: "alice", Content: "what's up"}
	out := renderEvent(e, "")
	if strings.Contains(out, "<p>what") {
		t.Errorf("single line was wrapped in a paragraph:\n%s", out)
	}
	if !strings.Contains(out, "what&rsquo;s up") && !strings.Contains(out, "what's up") {
		t.Errorf("content missing:\n%s", out)
	}
}

// Console content includes anything a user typed, so the renderer has to be
// the safe one.
func TestUserContentCannotInjectMarkup(t *testing.T) {
	e := &Event{Type: TypeUser, Author: "mallory", Content: `<img src=x onerror=alert(1)> and [x](javascript:alert(1))`}
	out := renderEvent(e, "")
	for _, bad := range []string{"onerror", "javascript:"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Errorf("rendered output contains %q:\n%s", bad, out)
		}
	}
}

// Bare URLs still become links now that the manual linkify pass is gone.
func TestBareURLIsAutolinked(t *testing.T) {
	e := &Event{Type: TypeUser, Author: "alice", Content: "see https://example.com for more"}
	out := renderEvent(e, "")
	if !strings.Contains(out, `href="https://example.com"`) {
		t.Errorf("bare URL was not linked:\n%s", out)
	}
}
