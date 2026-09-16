package ai

import (
	"fmt"
	"mu/internal/settings"
	"strings"
)

// BackgroundEnabled is an explicit opt-in to automatic model processing.
func BackgroundEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(settings.Get("AI_BACKGROUND_ENABLED")), "true")
}

func checkBackground(caller string) error {
	switch caller {
	case "brief", "daily-digest", "moderate", "social-judge", "news-sentiment", "notes-generate", "opinion-generate", "article-summary", "auto-tag-post", "auto-tag-note", "topic-summary", "topic-generation":
		if !BackgroundEnabled() {
			return fmt.Errorf("automatic AI processing is disabled")
		}
	}
	return nil
}
