package news

import (
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
	"mu/internal/data"
)

func TestIndexArticlePreservesSourceAlongsideSummary(t *testing.T) {
	id := "rag-source-preservation"
	defer data.Unindex(id)
	indexArticle(&Post{ID: id, Title: "Source article", URL: "https://example.com/source"}, &gofeed.Item{Content: "<p>Original evidence about Muse</p>"}, &Metadata{Summary: "Generated interpretation"})
	e := data.ByID(id)
	if e == nil || !strings.Contains(e.Content, "Original evidence") || strings.Contains(e.Content, "Generated interpretation") {
		t.Fatalf("source replaced: %+v", e)
	}
	if e.Metadata["summary"] != "Generated interpretation" || e.Metadata["content_kind"] != "source excerpt" {
		t.Fatalf("lost provenance: %+v", e.Metadata)
	}
}
