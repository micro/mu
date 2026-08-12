// Package social is the agent that decides what is worth surfacing.
//
// It was inside service/social, which made the social service import news: the
// thing choosing what to post lived in the thing that stores what was posted.
// Services answer questions about state; agents decide which questions to ask.
// The social service holds messages; this reads the news and calls
// SurfaceBreaking, which is the service's own front door.
//
// What it does is a heuristic and worth naming as one: the same story reported
// under two different categories is more likely to matter than one reported
// under one. That is not a fact about the world, it is a guess about editorial
// attention, and a guess belongs here rather than in a service, where things are
// supposed to be deterministic.
package social

import (
	"regexp"
	"strings"
	"time"

	"mu/internal/app"
	"mu/service/news"
	socialsvc "mu/service/social"
)

// Start begins watching for stories worth surfacing: this instance's own news,
// and — where an operator has turned it on — the open social network.
func Start() {
	go detectBreakingStories()

	// What the agent decides, handed to the service to store. Wired here rather
	// than imported inside the watcher so the filtering and the scoring can be
	// tested without standing up social.
	Surface = func(c *candidate) {
		socialsvc.SurfaceBreaking(c.Category, c.display(), c.Link)
	}
	go Watch()
}

// detectBreakingStories checks the news feed for stories covered by multiple
// categories/sources. If the same story appears across 2+ sources, it's
// significant enough to surface as a social thread.
func detectBreakingStories() {
	// Wait for news to load first
	time.Sleep(3 * time.Minute)

	for {
		surfaceBreakingFromNews()
		time.Sleep(time.Hour)
	}
}

func surfaceBreakingFromNews() {
	feed := news.GetFeed()
	if len(feed) == 0 {
		return
	}

	// Only consider articles from the last 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)

	// Extract keywords from each recent article
	type article struct {
		title    string
		url      string
		category string
		words    map[string]bool
	}
	var recent []article

	for _, p := range feed {
		if p.PostedAt.Before(cutoff) {
			continue
		}
		words := extractKeywords(p.Title)
		if len(words) < 2 {
			continue
		}
		recent = append(recent, article{
			title:    p.Title,
			url:      p.URL,
			category: p.Category,
			words:    words,
		})
	}

	// Find articles from different categories that share 2+ keywords
	surfaced := map[string]bool{}
	for i, a := range recent {
		for j := i + 1; j < len(recent); j++ {
			b := recent[j]
			if a.category == b.category {
				continue
			}
			// Count shared keywords
			shared := 0
			for w := range a.words {
				if b.words[w] {
					shared++
				}
			}
			if shared >= 2 {
				// Surface the first one (use URL as dedup key)
				if !surfaced[a.url] {
					surfaced[a.url] = true
					socialsvc.SurfaceBreaking(a.category, a.title, a.url)
					app.Log("social", "Breaking: %q matched across %s and %s", a.title, a.category, b.category)
				}
			}
		}
	}
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"has": true, "have": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "its": true, "it": true,
	"that": true, "this": true, "as": true, "not": true, "no": true,
	"new": true, "says": true, "said": true, "after": true, "over": true,
	"into": true, "up": true, "out": true, "about": true, "than": true,
	"how": true, "what": true, "when": true, "where": true, "who": true,
	"why": true, "more": true, "been": true, "being": true, "just": true,
}

// extractKeywords pulls significant words from a headline
func extractKeywords(title string) map[string]bool {
	title = strings.ToLower(title)
	title = regexp.MustCompile(`[^a-z0-9\s]`).ReplaceAllString(title, "")

	words := map[string]bool{}
	for _, w := range strings.Fields(title) {
		if !stopWords[w] && len(w) > 2 {
			words[w] = true
		}
	}
	return words
}
