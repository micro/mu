package docs

// The page can do what the tools can do.
//
// Two doors onto one service is the shape of the whole product — an agent calls
// the tool, a person opens the page — and a store you cannot correct by hand is
// one where the only remedy for a bad write is to write another agent. These
// tests hold the four verbs in place so the page cannot quietly become
// read-only.
//
// What they no longer hold is JSON. The page used to ask a person to type a
// record; now it asks for a title and a body, which is what it should have asked
// for all along.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
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
		t.Fatalf("expected a redirect after a write, got %d", w.Code)
	}
	return w.Header().Get("Location")
}

// shown reads the caller's documents back through the page's own JSON view, so
// the test asserts what a reader would be given rather than what the store holds.
func shown(t *testing.T, owner string) []*Doc {
	t.Helper()
	r := signedIn(t, owner, "GET", "/docs", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)
	var out struct {
		Docs []*Doc `json:"docs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("reading documents back: %v (%s)", err, w.Body.String())
	}
	return out.Docs
}

func TestThePageWritesReadsEditsAndDeletes(t *testing.T) {
	const owner = "page_user"

	loc := post(t, owner, url.Values{
		"title":   {"A plan"},
		"content": {"# A plan\n\nStep one."},
	})
	if !strings.Contains(loc, "/docs?id=") {
		t.Fatalf("after writing, went to %q — expected the new document", loc)
	}
	docs := shown(t, owner)
	if len(docs) != 1 || docs[0].Title != "A plan" {
		t.Fatalf("the page shows %d documents: %+v", len(docs), docs)
	}
	id := docs[0].ID

	// Reading renders the body rather than showing the raw markdown.
	w := httptest.NewRecorder()
	Handler(w, signedIn(t, owner, "GET", "/docs?id="+id, nil))
	if body := w.Body.String(); !strings.Contains(body, "Step one.") {
		t.Error("the document's body is not on its page")
	}

	// Editing keeps one document.
	post(t, owner, url.Values{"id": {id}, "title": {"A plan"}, "content": {"Step two."}})
	docs = shown(t, owner)
	if len(docs) != 1 {
		t.Fatalf("editing left %d documents", len(docs))
	}
	if docs[0].Content != "Step two." {
		t.Errorf("the edit did not stick: %q", docs[0].Content)
	}

	post(t, owner, url.Values{"delete": {id}})
	if got := shown(t, owner); len(got) != 0 {
		t.Errorf("%d documents survived deletion", len(got))
	}
}

// TestABadWriteKeepsWhatWasTyped — losing a document to a validation message is
// worse than the mistake that caused it.
func TestABadWriteKeepsWhatWasTyped(t *testing.T) {
	const owner = "careless"
	w := httptest.NewRecorder()
	Handler(w, signedIn(t, owner, "POST", "/docs", url.Values{
		"title":   {""},
		"content": {"an hour of writing"},
	}))
	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("a bad write returned %d", w.Code)
	}
	if !strings.Contains(body, "needs a title") {
		t.Error("the page did not say what was wrong")
	}
	if !strings.Contains(body, "an hour of writing") {
		t.Error("the page threw away what had been typed")
	}
}

func TestThePageCannotTouchSomebodyElsesDocuments(t *testing.T) {
	loc := post(t, "owner_a", url.Values{"title": {"Private"}, "content": {"mine"}})
	id := strings.TrimPrefix(loc, "/docs?id=")

	// Another account deleting by id must not work.
	post(t, "owner_b", url.Values{"delete": {id}})
	if got := shown(t, "owner_a"); len(got) != 1 {
		t.Fatal("another account deleted this document")
	}
	// Nor reading it.
	w := httptest.NewRecorder()
	Handler(w, signedIn(t, "owner_b", "GET", "/docs?id="+id, nil))
	if strings.Contains(w.Body.String(), "mine") {
		t.Error("another account can read this document's body")
	}
}

func TestThePageRefusesAGuest(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/docs", nil))
	if w.Code != http.StatusSeeOther && w.Code != http.StatusFound {
		t.Errorf("a guest got %d rather than a redirect to sign in", w.Code)
	}
}
