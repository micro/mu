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
	if strings.Count(body, `<article `) != 5 || strings.Contains(body, ">World story<") || strings.Contains(body, ">Story 0<") {
		t.Fatal("wrong filtered page")
	}
	if !strings.Contains(body, `/agent/micro?item=p20`) || !strings.Contains(body, `/saved?item=p20`) {
		t.Fatal("missing private reading actions")
	}
	if strings.Contains(body, `/chat?id=`) {
		t.Fatal("private question points at a shared room")
	}
}
