package blog

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicationViews(t *testing.T) {
	old := postsItems
	t.Cleanup(func() { postsItems = old })
	postsItems = []listItem{{Editorial: true, HTML: "<p>Editorial marker</p>"}, {HTML: "<p>Legacy marker</p>"}}
	for i := 0; i < 21; i++ {
		postsItems = append(postsItems, listItem{Community: true, HTML: fmt.Sprintf("<p>Community marker %02d</p>", i)})
	}
	for _, tc := range []struct{ path, want, absent string }{
		{"/blog", "Editorial marker", "Community marker"},
		{"/blog?view=archive", "Legacy marker", "Community marker"},
		{"/blog?view=community", "Community marker 00", "Legacy marker"},
		{"/blog?view=community&page=2", "Community marker 20", "Community marker 00"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			handleGetBlog(w, r)
			body := w.Body.String()
			if w.Code != 200 || !strings.Contains(body, tc.want) || strings.Contains(body, tc.absent) {
				t.Fatalf("unexpected view: status %d, want %q, exclude %q", w.Code, tc.want, tc.absent)
			}
			if !strings.Contains(body, `href="/blog?view=community"`) {
				t.Fatal("missing community navigation")
			}
			if tc.path == "/blog?view=community" && !strings.Contains(body, "view=community&amp;") && !strings.Contains(body, "&amp;view=community") {
				t.Fatal("pagination lost community view")
			}
		})
	}
}

func TestLegacyPublicationMetadata(t *testing.T) {
	var post Post
	if err := json.Unmarshal([]byte(`{"id":"old","title":"Legacy"}`), &post); err != nil {
		t.Fatal(err)
	}
	if publication(post.Editorial, post.Community) != "archive" {
		t.Fatal("legacy post moved out of archive")
	}
	post.Community = true
	encoded, err := json.Marshal(post)
	if err != nil {
		t.Fatal(err)
	}
	var restored Post
	if err = json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if publication(restored.Editorial, restored.Community) != "community" {
		t.Fatal("community placement did not survive serialization")
	}
	if returnTo("") != "/blog?view=community" || returnTo("https://example.com") != "/blog?view=community" {
		t.Fatal("new post must return to community")
	}
	if returnTo("/@alice") != "/@alice" {
		t.Fatal("explicit profile return changed")
	}
}
