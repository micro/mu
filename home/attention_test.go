package home

import (
	"mu/internal/auth"
	"mu/service/tasks"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAttentionDismissalIsOwnedAndVersioned(t *testing.T) {
	owner := "attention_owner"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create(owner, "Private item", "", tasks.Me, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	items := attentionItems(owner, time.Now())
	if len(items) != 1 {
		t.Fatalf("items = %v", items)
	}
	if len(attentionItems("attention_other", time.Now())) != 0 {
		t.Fatal("another account sees private task")
	}
	key := items[0].Key
	request := func(token bool, key, action string) *httptest.ResponseRecorder {
		body := url.Values{"attention_key": {key}, "attention_action": {action}}
		r := httptest.NewRequest("POST", "/home", strings.NewReader(body.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if token {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		attentionAction(w, r)
		return w
	}
	if w := request(false, key, "dismiss"); w.Code != 403 {
		t.Fatalf("missing CSRF = %d", w.Code)
	}
	if w := request(true, "not-owned", "dismiss"); w.Code != 409 {
		t.Fatalf("unknown item = %d", w.Code)
	}
	if w := request(true, key, "later"); w.Code != 303 {
		t.Fatalf("snooze = %d: %s", w.Code, w.Body)
	}
	if !attentionHidden(owner, time.Now())[key] || attentionHidden(owner, time.Now().Add(2*time.Hour))[key] {
		t.Fatal("snooze expiry incorrect")
	}
	if w := request(true, key, "dismiss"); w.Code != 303 {
		t.Fatalf("dismiss = %d", w.Code)
	}
	if !attentionHidden(owner, time.Now().Add(2*time.Hour))[key] {
		t.Fatal("dismissal not durable")
	}
	if attentionHidden("attention_other", time.Now())[key] {
		t.Fatal("dismissal leaked between accounts")
	}
	if _, err := tasks.Update(owner, task.ID, "Changed item", "", tasks.StatusTodo, tasks.Me, ""); err != nil {
		t.Fatal(err)
	}
	changed := attentionItems(owner, time.Now())
	if len(changed) != 1 || changed[0].Key == key {
		t.Fatal("changed item remains dismissed")
	}
}
