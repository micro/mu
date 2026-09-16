package ai

import "fmt"

// BackgroundEnabled is false: models run on requests and personal brief schedules.
// Retained as the common guard for automatic content producers.
func BackgroundEnabled() bool { return false }

func checkBackground(caller string) error {
	switch caller {
	case "arrival-gate", "agent.compact", "brief", "daily-digest", "moderate", "social-judge", "news-sentiment", "notes-generate", "opinion-generate", "article-summary", "auto-tag-post", "auto-tag-note", "topic-summary", "topic-generation":
		if !BackgroundEnabled() {
			return fmt.Errorf("automatic AI processing is disabled")
		}
	}
	return nil
}
