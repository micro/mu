package docs

// The page can do what the tools can do.
//
// It shipped as a viewer, on the argument that records are written by an agent
// and a form here would be a fourth way to do the same thing. But two doors onto
// one service is the shape of the whole product — an agent calls the tool, a
// person opens the page — and a store you cannot correct by hand is one where
// the only remedy for a bad write is to write another agent. These tests hold
// the four verbs in place so the page cannot quietly become read-only again.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/userdb"
)

// signedIn returns a request carrying a real session for owner, plus the CSRF
// token that session's forms would carry.
func signedIn(t *testing.T, owner, method, target string, form url.Values) *http.Request {
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

// post drives the page's write half and returns where it redirected to.
func post(t *testing.T, owner string, form url.Values) string {
	t.Helper()
	w := httptest.NewRecorder()
	Handler(w, signedIn(t, owner, "POST", "/docs", form))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected a redirect after a write, got %d: %s", w.Code, w.Body.String())
	}
	return w.Header().Get("Location")
}

// records reads a collection back through the page's own JSON view, so the test
// asserts what a reader would actually be shown rather than what the store holds.
func records(t *testing.T, owner, collection string) []userdb.Record {
	t.Helper()
	r := signedIn(t, owner, "GET", "/docs?collection="+collection, nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)
	var out struct {
		Records []userdb.Record `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("reading %s back: %v (%s)", collection, err, w.Body.String())
	}
	return out.Records
}

func TestThePageCreatesReadsEditsAndDeletes(t *testing.T) {
	const owner = "page-crud"

	// Create — and with it the collection, which has no separate existence.
	if loc := post(t, owner, url.Values{
		"action": {"save"}, "collection": {"leads"},
		"data": {`{"name":"Sam","stage":"contacted"}`},
	}); loc != "/docs?collection=leads" {
		t.Fatalf("expected to land on the collection, got %q", loc)
	}

	got := records(t, owner, "leads")
	if len(got) != 1 || got[0].Data["name"] != "Sam" {
		t.Fatalf("expected the record just written, got %+v", got)
	}
	id := got[0].ID

	// Edit — the same form with an id, which is what docs_create does with an id.
	post(t, owner, url.Values{
		"action": {"save"}, "collection": {"leads"}, "id": {id},
		"data": {`{"name":"Sam","stage":"closed"}`}, "public": {"1"},
	})
	got = records(t, owner, "leads")
	if len(got) != 1 {
		t.Fatalf("an edit created a second record: %+v", got)
	}
	if got[0].Data["stage"] != "closed" || !got[0].Public {
		t.Fatalf("the edit did not take: %+v", got[0])
	}

	// Delete.
	post(t, owner, url.Values{"action": {"delete"}, "collection": {"leads"}, "id": {id}})
	if got = records(t, owner, "leads"); len(got) != 0 {
		t.Fatalf("expected the record gone, got %+v", got)
	}
}

// The query controls are docs_list's arguments, so they have to behave like them.
func TestThePageQueriesLikeDbList(t *testing.T) {
	const owner = "page-query"
	for _, rec := range []string{
		`{"name":"Sam","priority":2}`,
		`{"name":"Alex","priority":5}`,
		`{"name":"Jo","priority":9}`,
	} {
		post(t, owner, url.Values{"action": {"save"}, "collection": {"q"}, "data": {rec}})
	}

	ask := func(qs string) []userdb.Record {
		r := signedIn(t, owner, "GET", "/docs?collection=q&"+qs, nil)
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		Handler(w, r)
		var out struct {
			Records []userdb.Record `json:"records"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out.Records
	}

	if got := ask(`where=` + url.QueryEscape(`{"name":"Alex"}`)); len(got) != 1 {
		t.Fatalf("an equality filter returned %d records", len(got))
	}
	if got := ask(`where=` + url.QueryEscape(`{"priority":{"gte":5}}`)); len(got) != 2 {
		t.Fatalf("an operator filter returned %d records", len(got))
	}
	if got := ask(`limit=1`); len(got) != 1 {
		t.Fatalf("limit returned %d records", len(got))
	}
	got := ask(`sort=priority&order=asc`)
	if len(got) != 3 || got[0].Data["name"] != "Sam" {
		t.Fatalf("sort did not order ascending: %+v", got)
	}

	// A filter that is not JSON must not read as an empty collection. Showing
	// nothing and saying nothing is the failure mode that would send someone
	// looking for lost data.
	if got := ask(`where=` + url.QueryEscape(`{nope`)); len(got) != 3 {
		t.Fatalf("a malformed filter hid records: got %d, want all 3", len(got))
	}
}

// The page is not a way around ownership. It writes through the same userdb
// calls with the session's account as the owner, which is what the tools do.
func TestThePageCannotTouchSomebodyElsesRecords(t *testing.T) {
	const mine, theirs = "page-owner", "page-stranger"
	post(t, mine, url.Values{"action": {"save"}, "collection": {"private"}, "data": {`{"secret":"mine"}`}})
	id := records(t, mine, "private")[0].ID

	w := httptest.NewRecorder()
	Handler(w, signedIn(t, theirs, "POST", "/docs",
		url.Values{"action": {"delete"}, "collection": {"private"}, "id": {id}}))
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Fatalf("a stranger's delete was not refused, redirected to %q", loc)
	}
	if got := records(t, mine, "private"); len(got) != 1 {
		t.Fatal("a stranger deleted a record they do not own")
	}
	if got := records(t, theirs, "private"); len(got) != 0 {
		t.Fatalf("a stranger can read a private collection: %+v", got)
	}
}

// A signed-out visitor gets nothing, and gets told to sign in rather than
// silently seeing an empty database.
func TestThePageRefusesAGuest(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/docs", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected a guest to be refused, got %d", w.Code)
	}
}
