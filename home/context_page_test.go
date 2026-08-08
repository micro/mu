package home

// You can see what the agent remembers, and delete it.
//
// memory.Set was written by the agent from your conversations and read back
// into the system prompt of every question you asked, with no page anywhere to
// show one, correct one or remove one. A product that keeps notes on you and
// cannot show you the notes is asking for trust it has not earned.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/memory"
)

func ctxRequest(t *testing.T, owner, method, target string, form url.Values) *http.Request {
	t.Helper()
	if _, err := auth.GetAccount(owner); err != nil {
		if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	var r *http.Request
	if form != nil {
		r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return r
}

func remembered(t *testing.T, owner string) []memory.Entry {
	t.Helper()
	r := ctxRequest(t, owner, "GET", "/context", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	ContextHandler(w, r)
	var out struct {
		Memory []memory.Entry `json:"memory"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("reading context: %v (%s)", err, w.Body.String())
	}
	return out.Memory
}

func TestContextShowsAndForgetsWhatTheAgentRemembers(t *testing.T) {
	const owner = "ctx-owner"
	t.Cleanup(func() { memory.Clear(owner) })

	memory.Set(owner, "location", "London")
	memory.Set(owner, "style", "prefers short answers")

	got := remembered(t, owner)
	if len(got) != 2 {
		t.Fatalf("expected both memories to be shown, got %+v", got)
	}

	// Forget one.
	w := httptest.NewRecorder()
	ContextHandler(w, ctxRequest(t, owner, "POST", "/context",
		url.Values{"action": {"forget"}, "key": {"location"}}))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected a redirect after forgetting, got %d", w.Code)
	}
	got = remembered(t, owner)
	if len(got) != 1 || got[0].Key != "style" {
		t.Fatalf("the wrong memory was forgotten: %+v", got)
	}

	// Forget everything.
	w = httptest.NewRecorder()
	ContextHandler(w, ctxRequest(t, owner, "POST", "/context", url.Values{"action": {"forget-all"}}))
	if got = remembered(t, owner); len(got) != 0 {
		t.Fatalf("forget-all left %+v", got)
	}
}

// Memory is per account, and the page must not be a way to read somebody
// else's — it is the most personal thing the product stores.
func TestOneAccountCannotSeeAnothersMemory(t *testing.T) {
	const mine, theirs = "ctx-mine", "ctx-theirs"
	t.Cleanup(func() { memory.Clear(mine) })
	memory.Set(mine, "secret", "something private")

	if got := remembered(t, theirs); len(got) != 0 {
		t.Fatalf("a stranger saw somebody else's memory: %+v", got)
	}
	// And cannot delete it either.
	w := httptest.NewRecorder()
	ContextHandler(w, ctxRequest(t, theirs, "POST", "/context",
		url.Values{"action": {"forget"}, "key": {"secret"}}))
	if got := remembered(t, mine); len(got) != 1 {
		t.Fatal("a stranger deleted somebody else's memory")
	}
}

// A signed-out visitor is sent to sign in, not shown an empty page that reads
// as "nothing is remembered about you".
func TestContextRefusesAGuest(t *testing.T) {
	w := httptest.NewRecorder()
	ContextHandler(w, httptest.NewRequest("GET", "/context", nil))
	if w.Code != http.StatusSeeOther && w.Code != http.StatusUnauthorized {
		t.Fatalf("expected a guest to be turned away, got %d", w.Code)
	}
}
