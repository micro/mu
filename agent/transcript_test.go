package agent

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscriptSwitchIsPrivateAndPreservesParagraphs(t *testing.T) {
	const who = "transcriptowner"
	id := waiting(t, who)
	thread.Add(thread.Message{Thread: id, Account: who, Role: thread.RoleAgent, Text: "First paragraph.\n\nSecond paragraph."})
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	read := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/agent/micro?session="+id, nil)
		r.Header.Set("X-Mu-Transcript", "1")
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: token})
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
	w := read(sess.Token)
	var d struct {
		ID   string `json:"id"`
		HTML string `json:"html"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil || d.ID != id {
		t.Fatalf("transcript response: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(d.HTML, "<p>First paragraph.</p>") || !strings.Contains(d.HTML, "<p>Second paragraph.</p>") {
		t.Fatal(d.HTML)
	}
	auth.Create(&auth.Account{ID: "transcriptother"})
	other, err := auth.CreateSession("transcriptother")
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(other.Token)
	if w := read(other.Token); w.Code != 404 {
		t.Fatalf("other account read transcript: %d", w.Code)
	}
}
