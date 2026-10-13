package blog

import (
	"testing"
	"time"
)

func TestDefaultOpinionScheduleCoversTopics(t *testing.T) {
	t.Setenv("OPINIONS_PER_DAY", "8")
	want := len(opinionCategories())
	if want > maxDailyOpinions {
		want = maxDailyOpinions
	}
	if defaultDailyOpinions != 8 || dailyOpinions() != want {
		t.Fatalf("expected one per topic, got %d", dailyOpinions())
	}
	if opinionInterval(8) != 2*time.Hour {
		t.Fatal("eight topics should be spaced two hours apart")
	}
	categories := []string{"Tech", "UK"}
	if got := nextCategory(categories, map[string]bool{"tech": true}); got != "UK" {
		t.Fatalf("already published topic chosen: %s", got)
	}
	if got := nextCategory(categories, map[string]bool{"tech": true, "uk": true}); got != "" {
		t.Fatalf("topics repeated: %s", got)
	}
}
