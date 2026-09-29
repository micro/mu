package blog

import (
	"net/http/httptest"
	"testing"
)

func TestPrivateShareReadOnlyAndRevocable(t *testing.T) {
	old, oldMap := posts, postsMap
	p := &Post{ID: "share-test", AuthorID: "owner", Title: "Private reading", Content: "A private reading", Private: true, ShareKey: "secret"}
	posts = []*Post{p}
	postsMap = map[string]*Post{p.ID: p}
	defer func() { posts, postsMap = old, oldMap }()
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/blog/post?id=share-test", 403},
		{"GET", "/blog/post?id=share-test&share=wrong", 403},
		{"GET", "/blog/post?id=share-test&share=secret", 200},
		{"PATCH", "/blog/post?id=share-test&share=secret", 403},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		PostHandler(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	p.ShareKey = ""
	w := httptest.NewRecorder()
	PostHandler(w, httptest.NewRequest("GET", "/blog/post?id=share-test&share=secret", nil))
	if w.Code != 403 {
		t.Fatal("revoked link still accessible")
	}
}
