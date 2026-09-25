package ai

import "testing"

func TestOnlyHomeBriefIsRestored(t *testing.T) {
	if err := checkBackground("brief"); err != nil {
		t.Fatal(err)
	}
	for _, caller := range []string{"daily-digest", "article-summary", "news-sentiment", "topic-generation", "auto-tag-post"} {
		if err := checkBackground(caller); err == nil {
			t.Errorf("%s must remain disabled", caller)
		}
	}
	if BackgroundEnabled() {
		t.Fatal("blanket background processing must stay disabled")
	}
}
