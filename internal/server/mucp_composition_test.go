package server

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/contacts"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func compositionCall(token, service, endpoint, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Micro-Protocol", "mucp/1")
	r.Header.Set("Micro-Id", "composition-test-123456")
	r.Header.Set("Micro-Service", service)
	r.Header.Set("Micro-Endpoint", endpoint)
	w := httptest.NewRecorder()
	MUCPHandler(w, r)
	return w
}
func TestCompositionIsolation(t *testing.T) {
	contacts.Load()
	owner := "composition_owner"
	other := "composition_other"
	for _, id := range []string{owner, other} {
		if err := auth.Create(&auth.Account{ID: id, Admin: true}); err != nil {
			t.Fatal(err)
		}
	}
	_, token, err := auth.CreateToken(owner, "composition", []string{"read", "write", "api:home", "api:inbox", "api:services"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := auth.CreateToken(owner, "contacts", []string{"read", "write", "service:contacts"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	foreign := thread.Open(other, "mail", "private")
	own := thread.Open(owner, "mail", "own")
	for _, tc := range []struct {
		token, service, endpoint, body string
		code                           int
	}{
		{token, "assistant", "Home.Overview", "{}", 200},
		{token, "assistant", "Inbox.List", "{}", 200},
		{token, "assistant", "Services.List", "{}", 200},
		{token, "assistant", "Inbox.Read", `{"id":"` + foreign.ID + `"}`, 404},
		{token, "assistant", "Inbox.MarkRead", `{"id":"` + foreign.ID + `"}`, 404},
		{token, "assistant", "Inbox.Read", `{"id":"` + own.ID + `"}`, 200},
		{token, "assistant", "Inbox.MarkRead", `{"id":"` + own.ID + `"}`, 200},
		{token, "contacts", "Contacts.List", "{}", 403},
		{raw, "assistant", "Home.Overview", "{}", 403},
		{raw, "assistant", "Inbox.List", "{}", 403},
		{raw, "contacts", "Contacts.Add", `{"name":"Alice"}`, 200},
		{raw, "contacts", "Contacts.List", `{}`, 200},
		{token, "assistant", "Inbox.List", `{"account":"` + other + `"}`, 400},
	} {
		w := compositionCall(tc.token, tc.service, tc.endpoint, tc.body)
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.endpoint, w.Code, w.Body.String())
		}
	}
	w := compositionCall(token, "assistant", "Inbox.Read", `{"id":"`+own.ID+`"}`)
	var out map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{`"account":`, `"key":`, `"evidence":`} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private routing exposed", w.Body.String())
		}
	}
}
