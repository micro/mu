package news

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowseFiltersAndPaginatesWithoutDuplicateHeadlines(t *testing.T) {
	posts := []*Post{}
	for i := 0; i < 25; i++ {
		posts = append(posts, &Post{ID: fmt.Sprintf("p%d", i), Title: fmt.Sprintf("Story %d", i), URL: fmt.Sprintf("https://example.com/%d", i), Category: "Tech", PostedAt: time.Now().Add(-time.Duration(i) * time.Minute)})
	}
	posts = append(posts, &Post{ID: "world", URL: "https://example.com/world", Title: "World story", Category: "World"})
	body := browse(httptest.NewRequest("GET", "/news?category=Tech&page=2", nil), posts)
	if !strings.Contains(body, `id="news-search"`) {
		t.Fatal("missing search control")
	}
	if strings.Count(body, `<article `) != 5 || strings.Contains(body, ">World story<") || strings.Contains(body, ">Story 0<") {
		t.Fatal("wrong filtered page")
	}
	if !strings.Contains(body, `/agent/micro?item=p20`) || !strings.Contains(body, `/bookmarks?item=p20`) {
		t.Fatal("missing private reading actions")
	}
	if strings.Contains(body, `/chat?id=`) {
		t.Fatal("private question points at a shared room")
	}
}

func TestBrowseKeepsLegacyNewsCache(t *testing.T) {
	old := newsBodyHtml
	newsBodyHtml = "legacy news"
	defer func() { newsBodyHtml = old }()
	if got := feedBody(httptest.NewRequest("GET", "/news", nil), nil); got != "legacy news" {
		t.Fatal("legacy cache lost")
	}
}

func TestOverviewKeepsSlowCategoriesAndPlaceholders(t *testing.T) {
	now := time.Now()
	posts := []*Post{
		{ID: "fast1", URL: "https://example.com/1", Title: "Fast newest", Category: "Tech", PostedAt: now},
		{ID: "fast2", URL: "https://example.com/2", Title: "Fast older", Category: "Tech", PostedAt: now.Add(-time.Hour)},
		{ID: "slow", URL: "https://example.com/3", Title: "Slow topic", Category: "World", PostedAt: now.Add(-48 * time.Hour)},
	}
	body := browse(httptest.NewRequest("GET", "/news", nil), posts)
	if strings.Count(body, "<article ") != 2 || !strings.Contains(body, "Slow topic") || strings.Contains(body, "Fast older") {
		t.Fatal("overview crowded out a topic")
	}
	if strings.Count(body, `class="news-reading-image"`) != 2 {
		t.Fatal("missing image placeholders")
	}
}
