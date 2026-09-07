package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/bookmarks"
	"mu/internal/thread"
)

func TestReadingIsPrivateAndAttachedToANewConversation(t *testing.T) {
	owner := "reading_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "fixture"}); err != nil {
		t.Fatal(err)
	}
	item, err := bookmarks.Add(owner, bookmarks.Item{URL: "https://example.com/material", Title: "Reading title", Note: "private annotation"})
	if err != nil {
		t.Fatal(err)
	}
	defer bookmarks.Clear(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/agent/micro?saved="+item.ID, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 200 {
		t.Fatalf("page: %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "private annotation") || !strings.Contains(body, "bookmark:"+item.ID) || !strings.Contains(body, "agent-"+owner+"-") {
		t.Fatal("selected material was not attached to an isolated private chat")
	}
	th := thread.Open(owner, thread.WebClient, "reading-test")
	thread.SetAttachment(owner, th.ID, "saved:"+item.ID)
	Said(owner, th.ID, "What does this mean?", "", "")
	if messages := thread.Messages(owner, th.ID, 10); len(messages) != 1 || messages[0].Text != "What does this mean?" {
		t.Fatal("attachment became user speech")
	}
	if got := conversationReading(owner, th.ID); !strings.Contains(got, "private annotation") {
		t.Fatal("reopened conversation lost selected material")
	}
	if got := conversationReading("another", th.ID); got != "" {
		t.Fatal("conversation attachment crossed accounts")
	}
	ctx, err := readingContext(owner, "saved:"+item.ID)
	if err != nil || !strings.Contains(ctx, "private annotation") {
		t.Fatal("server failed to resolve private material")
	}
	if _, err := readingContext("another", "saved:"+item.ID); err == nil {
		t.Fatal("resolved another account's material")
	}
	r = httptest.NewRequest("GET", "/agent/micro?saved="+item.ID, nil)
	w = httptest.NewRecorder()
	Handler(w, r)
	if strings.Contains(w.Body.String(), "private annotation") {
		t.Fatal("guest received private saved note")
	}
}
