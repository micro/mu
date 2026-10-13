package ai

import "testing"

func TestOnlyHomeBriefIsRestored(t *testing.T) {
	if err := checkBackground("brief"); err != nil {
		t.Fatal(err)
	}
	for _, caller := range []string{"article-summary", "news-sentiment", "topic-generation", "auto-tag-post"} {
		if err := checkBackground(caller); err == nil {
			t.Errorf("%s must remain disabled", caller)
		}
	}
	if BackgroundEnabled() {
		t.Fatal("blanket background processing must stay disabled")
	}
}

func TestBlogGenerationEnabledSeparately(t *testing.T) {
	t.Setenv("BLOG_DIGESTS", "on")
	for _, caller := range []string{"daily-digest", "opinion-generate"} {
		if err := checkBackground(caller); err != nil {
			t.Fatal(err)
		}
	}
	if BackgroundEnabled() {
		t.Fatal("unrelated background enabled")
	}
}

func TestBlogGenerationCanBeDisabled(t *testing.T) {
	t.Setenv("BLOG_DIGESTS", "off")
	for _, caller := range []string{"daily-digest", "opinion-generate"} {
		if checkBackground(caller) == nil {
			t.Errorf("%s enabled", caller)
		}
	}
}
