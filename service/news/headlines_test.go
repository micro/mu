package news

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHeadlinesReturnsOneStoryPerTopic(t *testing.T) {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	var posts []*Post
	for i := 11; i >= 0; i-- {
		for j := 1; j >= 0; j-- {
			posts = append(posts, &Post{ID: fmt.Sprintf("%d-%d", i, j), Title: fmt.Sprintf("story %d-%d", i, j), URL: fmt.Sprintf("https://example.test/%d/%d", i, j), Category: fmt.Sprintf("topic %02d", i), Description: "summary", PostedAt: at.Add(-time.Duration(i+j*24) * time.Hour)})
		}
	}
	mutex.Lock()
	was := feed
	feed = posts
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); feed = was; mutex.Unlock() })
	var rsp HeadlinesResponse
	if err := (Server{}).Headlines(context.Background(), &HeadlinesRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	if len(rsp.Items) != 10 {
		t.Fatalf("got %d headlines, want 10", len(rsp.Items))
	}
	for i, h := range rsp.Items {
		if h.Title != fmt.Sprintf("story %d-0", i) || h.Category != fmt.Sprintf("topic %02d", i) || h.URL != fmt.Sprintf("https://example.test/%d/0", i) || h.Description != "summary" {
			t.Errorf("headline %d: %+v", i, h)
		}
	}
	// The response and rendered card select the same stories, in the same order.
	html := generateHeadlinesHTML(cardPosts(GetFeed()))
	last := -1
	for _, h := range rsp.Items {
		pos := strings.Index(html, h.Title)
		if pos <= last {
			t.Fatalf("card differs at %q", h.Title)
		}
		last = pos
	}
	mutex.Lock()
	feed = nil
	mutex.Unlock()
	if err := (Server{}).Headlines(context.Background(), &HeadlinesRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rsp)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"items":[]}` {
		t.Fatalf("empty feed: %s", b)
	}
}
