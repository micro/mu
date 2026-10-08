package blog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestCompletedPrivateReadingsAreOwnerOnly(t *testing.T) {
	owner := "reading-test-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "Reader"})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	reading := &Post{ID: "reading-old", Title: "Existing completed reading", AuthorID: owner, Private: true, Tags: "Research", Content: "A finished piece.", CreatedAt: time.Now()}
	newer := &Post{ID: "new-reading", Title: "New completed reading", AuthorID: owner, Private: true, Published: true, Tags: "Research", Content: "Another finished piece.", CreatedAt: time.Now()}
	draft := &Post{ID: "unfinished", Title: "Unfinished secret", AuthorID: owner, Private: true}
	foreign := &Post{ID: "foreign", Title: "Someone else's secret", AuthorID: "other", Private: true, Published: true, Tags: "Research"}
	mutex.Lock()
	oldPosts, oldMap, oldItems := posts, postsMap, postsItems
	posts = []*Post{newer, reading, draft, foreign}
	postsMap = map[string]*Post{}
	postsItems = nil
	for _, p := range posts {
		postsMap[p.ID] = p
	}
	mutex.Unlock()
	defer func() { mutex.Lock(); posts, postsMap, postsItems = oldPosts, oldMap, oldItems; mutex.Unlock() }()
	for _, accept := range []string{"text/html", "application/json"} {
		for _, signedIn := range []bool{false, true} {
			r := httptest.NewRequest("GET", "/blog?tag=research", nil)
			r.Header.Set("Accept", accept)
			if signedIn {
				r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
			}
			w := httptest.NewRecorder()
			Handler(w, r)
			body := w.Body.String()
			if strings.Contains(body, reading.Title) != signedIn || strings.Contains(body, newer.Title) != signedIn {
				t.Fatalf("reading visibility incorrect (%s, signed in %v)", accept, signedIn)
			}
			if strings.Contains(body, draft.Title) || strings.Contains(body, foreign.Title) {
				t.Fatal("private content leaked into listing")
			}
			if signedIn && w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("personal list cacheable")
			}
		}
	}
	r := httptest.NewRequest("GET", "/blog?view=drafts", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	Handler(w, r)
	if strings.Contains(w.Body.String(), reading.Title) || strings.Contains(w.Body.String(), newer.Title) || !strings.Contains(w.Body.String(), draft.Title) {
		t.Fatal("completed readings still in drafts")
	}
	if !strings.Contains(PrivatePreview(owner), newer.Title) || strings.Contains(PrivatePreview(owner), foreign.Title) || PrivatePreview("") != "" {
		t.Fatal("wrong private feed preview")
	}
	if strings.Contains(Preview(), newer.Title) {
		t.Fatal("reading leaked into shared preview")
	}
}

func TestReadingMetadataRepair(t *testing.T) {
	p := &Post{ID: "reading-old", Title: "Islam", Content: "# Patience and agency\n\nAn essay.", Tags: "evening-reading", Private: true}
	repairReading(p)
	if p.Title != "Patience and agency" || p.Tags != "Research" || p.isDraft() || !p.Private {
		t.Fatalf("%+v", p)
	}
	repairReading(p)
	p.Title = "My edit"
	p.Tags = "evening-reading"
	p.UpdatedAt = time.Now()
	repairReading(p)
	if p.Title != "My edit" {
		t.Fatal("overwrote edited title")
	}
	ordinary := &Post{ID: "ordinary", Title: "Original", Tags: "evening-reading", Content: "# Another"}
	repairReading(ordinary)
	if ordinary.Title != "Original" || ordinary.Tags != "evening-reading" {
		t.Fatal("changed ordinary post")
	}
}

func TestTagLinksAndExactMatching(t *testing.T) {
	if !hasTag("Islam, Research", "research") || hasTag("Researcher", "research") {
		t.Fatal("tag matching")
	}
	if html := formatTags(`Research, <script>`); !strings.Contains(html, "/blog?tag=Research") || strings.Contains(html, "<script>") {
		t.Fatal(html)
	}
}
