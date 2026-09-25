package inbox

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBulkReadScopeCSRFAndLaterArrivals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "bulk-read-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Hour)
	first := thread.OpenAt(owner, "mail", "first", at)
	second := thread.OpenAt(owner, "mail", "second", at)
	later := thread.OpenAt(owner, "mail", "later", time.Now())
	foreign := thread.OpenAt("bulk-other", "mail", "foreign", at)
	held := thread.OpenAt(owner, "mail", "held", at)
	thread.Hold(owner, held.ID)
	cutoff := time.Now().Add(-time.Minute)
	send := func(scope string, csrf bool, ids ...string) *httptest.ResponseRecorder {
		values := url.Values{"action": {"mark_read"}, "scope": {scope}, "filter": {"unread"}, "reviewed": {cutoff.Format(time.RFC3339Nano)}, "id": ids}
		r := httptest.NewRequest("POST", "/inbox", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
	if w := send("all", false); w.Code != http.StatusForbidden {
		t.Fatalf("CSRF: %d", w.Code)
	}
	if !thread.Unread(*thread.Get(owner, first.ID)) {
		t.Fatal("invalid request changed read state")
	}
	if w := send("selected", true, first.ID, foreign.ID, held.ID); w.Code != 303 || w.Header().Get("Location") != "/inbox?filter=unread" {
		t.Fatalf("selected: %d %s", w.Code, w.Body.String())
	}
	if thread.Unread(*thread.Get(owner, first.ID)) || !thread.Unread(*thread.Get(owner, second.ID)) {
		t.Fatal("selected scope changed wrong rows")
	}
	if w := send("all", true); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if thread.Unread(*thread.Get(owner, second.ID)) {
		t.Fatal("all did not clear old unread")
	}
	for _, pair := range [][2]string{{owner, later.ID}, {owner, held.ID}, {"bulk-other", foreign.ID}} {
		if !thread.Unread(*thread.Get(pair[0], pair[1])) {
			t.Fatal("cleared new, held or foreign conversation")
		}
	}
	r := httptest.NewRequest("GET", "/inbox?filter=unread", nil)
	rows := filterUnread(r, inboxThreads(owner, "/inbox"))
	if len(rows) != 1 || rows[0].ID != later.ID {
		t.Fatalf("unread rows: %+v", rows)
	}
	n, _ := Waiting(owner)
	if n != len(rows) {
		t.Fatalf("brief %d != inbox %d", n, len(rows))
	}
}
