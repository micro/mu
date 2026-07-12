package discord

import (
	"strings"
	"testing"

	"mu/news"
)

// TestDiverseHeadlinesSpreadsAndDefersCrypto verifies the brief input is spread
// across categories and that a high-volume crypto feed can't dominate.
func TestDiverseHeadlinesSpreadsAndDefersCrypto(t *testing.T) {
	// A crypto-heavy feed (as the real feeds tend to be), newest first.
	feed := []*news.Post{
		{Title: "c1", Category: "Crypto"},
		{Title: "c2", Category: "Crypto"},
		{Title: "c3", Category: "Crypto"},
		{Title: "w1", Category: "World"},
		{Title: "t1", Category: "Tech"},
		{Title: "f1", Category: "Finance"},
		{Title: "c4", Category: "Crypto"},
		{Title: "p1", Category: "Politics"},
	}

	got := diverseHeadlines(feed, 2, 8)

	// First pick must NOT be crypto — crypto is deferred to last in each round.
	if strings.EqualFold(got[0].Category, "crypto") {
		t.Fatalf("brief led with crypto: %q", got[0].Title)
	}
	// No more than perCat (2) crypto stories even though 4 were available.
	crypto := 0
	for _, p := range got {
		if strings.EqualFold(p.Category, "crypto") {
			crypto++
		}
	}
	if crypto > 2 {
		t.Errorf("crypto count = %d, want <= 2", crypto)
	}
	// At least four distinct categories represented (spread, not one topic).
	cats := map[string]bool{}
	for _, p := range got {
		cats[strings.ToLower(p.Category)] = true
	}
	if len(cats) < 4 {
		t.Errorf("only %d categories represented, want >= 4: %v", len(cats), cats)
	}
}

func TestDiverseHeadlinesRespectsTotal(t *testing.T) {
	var feed []*news.Post
	for i := 0; i < 30; i++ {
		feed = append(feed, &news.Post{Title: "x", Category: "World"})
	}
	if got := diverseHeadlines(feed, 2, 8); len(got) > 2 {
		// only one category, capped at perCat
		t.Errorf("single-category cap: got %d, want <= 2", len(got))
	}
}
