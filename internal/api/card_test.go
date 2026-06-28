package api

import (
	"strings"
	"testing"
)

func TestToolCards(t *testing.T) {
	RegisterTool(Tool{Name: "zz_card_tool", Description: "d"})
	RegisterTool(Tool{Name: "zz_plain_tool", Description: "d"})

	SetCard("zz_card_tool", "📈 Test", func() string { return "<b>BODY</b>" })
	SetCard("zz_empty_tool_missing", "X", func() string { return "x" }) // no-op, tool absent

	got := CardForTool("zz_card_tool")
	if !strings.Contains(got, `class="card"`) || !strings.Contains(got, "📈 Test") || !strings.Contains(got, "BODY") {
		t.Errorf("expected wrapped card, got %q", got)
	}
	if c := CardForTool("zz_plain_tool"); c != "" {
		t.Errorf("tool without card should render nothing, got %q", c)
	}
	if c := CardForTool("nonexistent"); c != "" {
		t.Errorf("unknown tool should render nothing, got %q", c)
	}

	// Empty body -> no wrapper.
	SetCard("zz_card_tool", "T", func() string { return "   " })
	if c := CardForTool("zz_card_tool"); c != "" {
		t.Errorf("empty body should render nothing, got %q", c)
	}

	// Appears in CardTools when it has a non-nil Card.
	SetCard("zz_card_tool", "T", func() string { return "x" })
	found := false
	for _, tl := range CardTools() {
		if tl.Name == "zz_card_tool" {
			found = true
		}
	}
	if !found {
		t.Error("zz_card_tool should appear in CardTools")
	}
}
