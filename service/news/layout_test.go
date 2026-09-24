package news

import (
	"mu/internal/data"
	"strings"
	"testing"
)

func TestSearchResultUsesMediaRowOnlyWithImage(t *testing.T) {
	for _, image := range []string{"", "https://example.test/news.jpg"} {
		entry := &data.IndexEntry{ID: "layout-test", Title: "Headline <one>", Content: "Story", Metadata: map[string]interface{}{"url": "https://example.test/story", "category": "World", "image": image}}
		got := formatSearchResult(entry)
		if strings.Contains(got, `class="reading-row news-reading-row"`) != (image != "") {
			t.Fatalf("wrong container for image %q", image)
		}
		if image == "" && !strings.Contains(got, `class="record-card"`) {
			t.Fatal("missing vertical record layout")
		}
		if !strings.Contains(got, "Headline &lt;one&gt;") {
			t.Fatal("headline not escaped")
		}
		if strings.Index(got, "<h3>") > strings.Index(got, `class="reading-meta"`) {
			t.Fatal("category precedes headline")
		}
	}
}
