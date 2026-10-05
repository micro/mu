package agent

import (
	"mu/agent/brief"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBriefPageAndAttachedContext(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "brief-reader"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "Reader"})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	e := brief.Entry{Text: "Blast closes", Material: "Original evidence about Blast", Written: time.Now(), Stories: []brief.Story{{Title: "Blast closes", Sources: []string{"https://example.com/news/blast", "javascript:alert(1)"}}}}
	if err := brief.Pin(e); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/brief?id="+e.ID(), nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	BriefHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Ask about this brief") || strings.Contains(w.Body.String(), e.Material) {
		t.Fatalf("missing brief: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `href="https://example.com/news/blast" rel="noopener noreferrer">Blast closes</a>`) || strings.Contains(w.Body.String(), "javascript:") {
		t.Fatal("source title must link safely to the article")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private response cached")
	}
	bad := httptest.NewRequest("POST", "/brief", strings.NewReader("id="+e.ID()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	denied := httptest.NewRecorder()
	BriefHandler(denied, bad)
	if denied.Code != http.StatusForbidden {
		t.Fatal("missing CSRF accepted")
	}
	post := httptest.NewRequest("POST", "/brief", strings.NewReader("id="+e.ID()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	post.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	post.Header.Set("X-CSRF-Token", auth.CSRFToken(post))
	opened := httptest.NewRecorder()
	BriefHandler(opened, post)
	if opened.Code != http.StatusSeeOther {
		t.Fatalf("handoff failed: %d %s", opened.Code, opened.Body.String())
	}

	th := thread.Open(owner, thread.WebClient, "brief:"+e.ID())
	thread.SetAttachment(owner, th.ID, "brief:"+e.ID())
	if !strings.Contains(conversationReading(owner, th.ID), e.Material) {
		t.Fatal("source context missing")
	}
	if conversationReading("other-reader", th.ID) != "" {
		t.Fatal("foreign conversation readable")
	}
}
