package core

import (
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	// fresh state
	mu.Lock()
	caps = map[string]*Capability{}
	order = nil
	mu.Unlock()

	Register(Capability{ID: "markets", Title: "📈 Markets", Card: func() string { return "<b>BTC</b>" }, Tools: []string{"markets"}})
	Register(Capability{ID: "news", Title: "📰 News", Card: func() string { return "  " }, Tools: []string{"news_headlines"}})
	Register(Capability{ID: ""}) // ignored

	if len(All()) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(All()))
	}
	if Get("markets") == nil || Get("missing") != nil {
		t.Error("Get lookup wrong")
	}
	if c := ForTool("news_headlines"); c == nil || c.ID != "news" {
		t.Errorf("ForTool(news_headlines) should map to news, got %v", c)
	}
	if ForTool("nope") != nil {
		t.Error("ForTool unknown should be nil")
	}

	card := CardForTool("markets")
	if !strings.Contains(card, `class="card"`) || !strings.Contains(card, "📈 Markets") || !strings.Contains(card, "BTC") {
		t.Errorf("CardForTool wrong: %q", card)
	}
	// Empty body -> no card wrapper.
	if got := CardHTML("news"); got != "" {
		t.Errorf("empty body should render no card, got %q", got)
	}

	// Re-register replaces, doesn't duplicate.
	Register(Capability{ID: "markets", Title: "M2", Card: func() string { return "x" }})
	if len(All()) != 2 {
		t.Errorf("re-register should not grow registry, got %d", len(All()))
	}
}
