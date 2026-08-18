package blog

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The blog page has an upper bound.
//
// It did not. Every post ever written was joined into one string when the posts
// last changed, and that string was what /blog served — to every visitor, on
// every load, for the life of the instance. A list nothing ever removes from,
// rendered whole, is a page that gets slower every day somebody uses it.
func TestTheBlogPageShowsOnePageOfPosts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	posts = nil
	postsMap = map[string]*Post{}
	for i := 0; i < postsPerPage*3; i++ {
		p := &Post{
			ID:        "post-" + string(rune('a'+i/26)) + string(rune('a'+i%26)),
			Title:     "Post number " + string(rune('a'+i%26)),
			Content:   "Something somebody wrote down.",
			Author:    "ann",
			AuthorID:  "ann",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute),
		}
		posts = append(posts, p)
		postsMap[p.ID] = p
	}
	updateCacheUnlocked()
	mutex.Unlock()

	count := func(url string) (int, string) {
		w := httptest.NewRecorder()
		Handler(w, httptest.NewRequest("GET", url, nil))
		body := w.Body.String()
		return strings.Count(body, `class="post-item"`), body
	}

	shown, body := count("/blog")
	if shown != postsPerPage {
		t.Fatalf("first page shows %d posts, want %d", shown, postsPerPage)
	}
	if !strings.Contains(body, "Older") {
		t.Error("no way to reach the rest of the blog")
	}

	shown, body = count("/blog?page=3")
	if shown != postsPerPage {
		t.Fatalf("last page shows %d posts, want %d", shown, postsPerPage)
	}
	if !strings.Contains(body, "Newer") {
		t.Error("no way back from the last page")
	}
	if strings.Contains(body, "Older") {
		t.Error("the last page offers a page after it")
	}
}

// And a JSON caller gets the same amount of answer, for the same reason.
func TestTheBlogAPIDoesNotReturnEveryPostEverWritten(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	posts = nil
	postsMap = map[string]*Post{}
	for i := 0; i < postsPerPage*2; i++ {
		p := &Post{
			ID:        "json-post-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Title:     "Post",
			Content:   "Body",
			Author:    "ann",
			AuthorID:  "ann",
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute),
		}
		posts = append(posts, p)
		postsMap[p.ID] = p
	}
	updateCacheUnlocked()
	mutex.Unlock()

	r := httptest.NewRequest("GET", "/blog", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)

	if got := strings.Count(w.Body.String(), `"id"`); got != postsPerPage {
		t.Fatalf("the API returned %d posts, want one page of %d", got, postsPerPage)
	}
}
