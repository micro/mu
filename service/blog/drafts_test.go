package blog

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDraftsAreScopedAndOwnerCanRead(t *testing.T) {
	owner := "blog-drafts-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "Writer"})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	own := &Post{ID: "draft-owned", AuthorID: owner, Title: "My private draft", Content: "This is the draft body.", Private: true, CreatedAt: time.Now()}
	foreign := &Post{ID: "draft-other", AuthorID: "another-writer", Title: "Another writer secret", Content: "Private foreign body", Private: true, CreatedAt: time.Now()}
	public := &Post{ID: "draft-public", AuthorID: owner, Title: "Already published", CreatedAt: time.Now()}
	mutex.Lock()
	oldPosts, oldMap := posts, postsMap
	posts = []*Post{own, foreign, public}
	postsMap = map[string]*Post{own.ID: own, foreign.ID: foreign, public.ID: public}
	mutex.Unlock()
	defer func() { mutex.Lock(); posts, postsMap = oldPosts, oldMap; mutex.Unlock() }()
	for _, format := range []string{"text/html", "application/json"} {
		r := httptest.NewRequest("GET", "/blog?view=drafts", nil)
		r.Header.Set("Accept", format)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Handler(w, r)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, own.Title) || strings.Contains(body, foreign.Title) || strings.Contains(body, public.Title) {
			t.Fatalf("wrong draft listing: %d %s", w.Code, body)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private list cached")
		}
	}
	guest := httptest.NewRecorder()
	Handler(guest, httptest.NewRequest("GET", "/blog?view=drafts", nil))
	if guest.Code == 200 || strings.Contains(guest.Body.String(), own.Title) {
		t.Fatal("guest saw draft list")
	}
	for _, id := range []string{own.ID, foreign.ID} {
		r := httptest.NewRequest("GET", "/blog/post?id="+id, nil)
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		PostHandler(w, r)
		if id == own.ID && w.Code != 200 {
			t.Fatal("owner could not open draft", w.Code)
		}
		if id == foreign.ID && w.Code != http.StatusForbidden {
			t.Fatal("foreign draft readable", w.Code)
		}
	}
	var rsp ReadResponse
	if err := (Server{}).Read(service.WithAccount(context.Background(), owner), &ReadRequest{ID: own.ID}, &rsp); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Read(context.Background(), &ReadRequest{ID: own.ID}, &rsp); err == nil {
		t.Fatal("guest tool read private post")
	}
	if err := (Server{}).Read(service.WithAccount(context.Background(), "another-writer"), &ReadRequest{ID: own.ID}, &rsp); err == nil {
		t.Fatal("foreign tool read private post")
	}
}
