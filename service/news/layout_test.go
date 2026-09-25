package news

import (
	"mu/internal/data"
	"strings"
	"testing"
)

func TestSearchResultAlwaysHasSourceCover(t *testing.T) {
	for _, image := range []string{"", "https://example.test/news.jpg"} {
		entry := &data.IndexEntry{ID: "layout-test", Title: "Headline <one>", Content: "Story", Metadata: map[string]interface{}{"url": "https://example.test/story", "category": "World", "image": image}}
		got := formatSearchResult(entry)
		if !strings.Contains(got, `class="reading-row news-reading-row"`) || !strings.Contains(got, `class="media-cover-label" aria-hidden="true">example.test</span>`) {
			t.Fatal("missing consistent media row and source fallback")
		}
		if strings.Contains(got, `data-cover-image`) != (image != "") {
			t.Fatal("unexpected publisher image")
		}
		if !strings.Contains(got, "Headline &lt;one&gt;") {
			t.Fatal("headline not escaped")
		}
		if strings.Index(got, "<h3>") > strings.Index(got, `class="reading-meta"`) {
			t.Fatal("category precedes headline")
		}
	}
}
