package agent

import (
	"mu/internal/auth"
	"mu/internal/saved"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadingIsPrivateAndAttachedToANewConversation(t *testing.T) {
	owner := "reading_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "fixture"}); err != nil {
		t.Fatal(err)
	}
	item, err := saved.Add(owner, saved.Item{URL: "https://example.com/material", Title: "Reading title", Note: "private annotation"})
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Clear(owner)
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
	if !strings.Contains(body, "private annotation") || !strings.Contains(body, "var attachment=") || !strings.Contains(body, "reading-"+owner) {
		t.Fatal("selected material was not attached to an isolated private chat")
	}
	r = httptest.NewRequest("GET", "/agent/micro?saved="+item.ID, nil)
	w = httptest.NewRecorder()
	Handler(w, r)
	if strings.Contains(w.Body.String(), "private annotation") {
		t.Fatal("guest received private saved note")
	}
}
