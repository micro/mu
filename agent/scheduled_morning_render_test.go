package agent

import (
	"mu/service/events"
	"mu/service/markets"
	"strings"
	"testing"
	"time"
)

func TestBriefRendererOwnsStructure(t *testing.T) {
	off := false
	e := &events.Event{Zone: "Europe/London", WorldNews: &off}
	c := BriefContent{Day: []BriefItem{{Text: "Meeting\n## Surprise", URL: "javascript:alert(1)"}}, Headlines: []BriefItem{{Text: "Hidden news"}}, Priorities: []BriefItem{{Text: "Hidden plan"}}}
	result := renderMorningBrief(c, "Asim", e, time.Date(2026, 9, 27, 23, 30, 0, 0, time.UTC), "## Daily reminder\n\nA verse.\n\n# Spiritual Reflection\n\nReflection text.", false)
	if strings.Contains(result, "\n## Surprise") || strings.Contains(result, "javascript:") || strings.Contains(result, "Hidden") {
		t.Fatal(result)
	}
	if !strings.Contains(result, "Monday, 28 September 2026") {
		t.Fatal("timezone lost", result)
	}
	previous := -1
	for _, heading := range []string{"## Your day", "## Weather", "## Prayer times", "## Markets", "## Daily reminder", "## Spiritual Reflection"} {
		position := strings.Index(result, heading)
		if position <= previous {
			t.Fatal(result)
		}
		previous = position
	}
}

func TestMorningMarketsPreserveFreshness(t *testing.T) {
	now := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	facts := briefMarkets(map[string]markets.PriceData{
		"BTC":  {Price: 65000, Change24h: -2.5, UpdatedAt: now.Add(-time.Minute), Source: "provider"},
		"GOLD": {Price: 2500, UpdatedAt: now.Add(-24 * time.Hour)},
		"GBP":  {Price: 1.3}, // Undated prices cannot be presented as current.
	}, now)
	for _, want := range []string{"BTC: 65000 USD", "-2.50%", "stale cached", "2026-09-28T06:00:00Z"} {
		if !strings.Contains(facts, want) {
			t.Fatalf("missing %q: %s", want, facts)
		}
	}
	if strings.Contains(facts, "GBP") {
		t.Fatal("undated price included", facts)
	}
	content, err := parseBriefContent(`{"markets":[{"text":"BTC: 65000 USD, cached","url":"https://micro.mu/markets"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	result := renderMorningBrief(content, "", &events.Event{Zone: "UTC"}, now, "", false)
	if !strings.Contains(result, "## Markets\n\n- BTC: 65000 USD, cached") {
		t.Fatal(result)
	}
}
