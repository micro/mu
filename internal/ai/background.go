package ai

import (
	"fmt"
	"mu/internal/settings"
	"strings"
)

// BackgroundEnabled is false: other automatic content producers remain disabled.
// The short cached Home brief and personal brief schedules are separate.
// Retained as the common guard for automatic content producers.
func BackgroundEnabled() bool { return false }

// BlogEnabled enables only shared blog generation, not other background agents.
func BlogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("BLOG_DIGESTS"))) {
	case "off", "false", "no", "0":
		return false
	}
	return true
}

func checkBackground(caller string) error {
	switch caller {
	case "daily-digest", "opinion-generate", "publication-review":
		if !BlogEnabled() {
			return fmt.Errorf("blog digests are disabled")
		}
		return nil
	case "arrival-gate", "agent.compact", "moderate", "flagged", "social-judge", "news-sentiment", "article-summary", "auto-tag-post", "auto-tag-note", "topic-summary", "topic-generation":
		if !BackgroundEnabled() {
			return fmt.Errorf("automatic AI processing is disabled")
		}
	}
	return nil
}
