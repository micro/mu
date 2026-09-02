package web

// A search you can send somebody.
//
// The POST kept the query out of the URL and took the address away with it: no
// bookmark, no link, no reload without a re-submit prompt. This is the part
// that gives it back without giving the query back.

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func someResults() []BraveResult {
	return []BraveResult{
		{Title: "One", URL: "https://example.com/1", Description: "first"},
		{Title: "Two", URL: "https://example.com/2", Description: "second"},
	}
}

func TestASharedResultKeepsTheResultsAndNotTheQueryInTheURL(t *testing.T) {
	resetShares()

	id := Share("something private", someResults())
	if id == "" {
		t.Fatal("no id was issued, so a search has no address")
	}
	if strings.Contains(id, "private") || strings.Contains(id, "something") {
		t.Errorf("the id carries the query: %q — that is the leak the POST exists to prevent", id)
	}

	q, got, ok := Shared(id)
	if !ok {
		t.Fatal("the share cannot be read back")
	}
	if q != "something private" {
		t.Errorf("query came back as %q", q)
	}
	if len(got) != 2 || got[0].Title != "One" {
		t.Errorf("results came back as %+v", got)
	}
}

// Random, not a hash of the query. A hash would let anybody confirm that a
// search had been run here by hashing a guess and asking for it.
func TestTheSameSearchTwiceGetsTwoIds(t *testing.T) {
	resetShares()
	a := Share("same words", someResults())
	b := Share("same words", someResults())
	if a == "" || b == "" {
		t.Fatal("no id issued")
	}
	if a == b {
		t.Error("the id is derived from the query, so it can be guessed at:\n" +
			"hash a search, ask for it, and find out whether somebody ran it")
	}
}

// Nothing to show is nothing to link to.
func TestAnEmptyResultSetGetsNoAddress(t *testing.T) {
	resetShares()
	if id := Share("nothing found", nil); id != "" {
		t.Errorf("an empty result set was given the address %q", id)
	}
}

// A link that is gone is gone, rather than silently becoming somebody else's.
func TestAnUnknownIdIsNotAShare(t *testing.T) {
	resetShares()
	for _, id := range []string{"", "nope", "AAAAAAAA"} {
		if _, _, ok := Shared(id); ok {
			t.Errorf("%q resolved to a share", id)
		}
	}
}

func TestASharedLinkExpires(t *testing.T) {
	resetShares()
	id := Share("old news", someResults())

	shareMu.Lock()
	s := shares[id]
	s.At = time.Now().Add(-shareTTL - time.Minute)
	shares[id] = s
	shareMu.Unlock()

	if _, _, ok := Shared(id); ok {
		t.Error("a share past its life is still served, so a page of results\n" +
			"about a war is still being handed out as current a year later")
	}
}

// The map is capped, because it grows with every search anybody runs.
func TestTheOldestSharesAreDropped(t *testing.T) {
	resetShares()
	shareMu.Lock()
	for i := 0; i < maxShares+50; i++ {
		shares[shareID()] = share{Query: "q", Results: someResults(),
			At: time.Now().Add(-time.Duration(i) * time.Second)}
	}
	evictOldest()
	n := len(shares)
	shareMu.Unlock()

	if n > maxShares {
		t.Errorf("%d shares kept, cap is %d — the file grows without bound", n, maxShares)
	}
}

// And the page it serves needs no account: the results were paid for when the
// search ran, and a link only its author can open is not a link.
func TestASharedPageIsServedToAnybody(t *testing.T) {
	resetShares()
	id := Share("public enough", someResults())

	r := httptest.NewRequest("GET", "/web/r/"+id, nil)
	w := httptest.NewRecorder()
	ResultsHandler(w, r)

	if w.Code != 200 {
		t.Fatalf("a signed-out reader got %d for a shared link", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"One", "example.com/1", "public enough"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q is not on the shared page", want)
		}
	}
}

func TestAnExpiredLinkIs404(t *testing.T) {
	resetShares()
	r := httptest.NewRequest("GET", "/web/r/gone", nil)
	w := httptest.NewRecorder()
	ResultsHandler(w, r)
	if w.Code != 404 {
		t.Errorf("an unknown share returned %d, want 404 — an expired link and a\n"+
			"mistyped one look the same, and landing somebody on an empty search\n"+
			"box tells them neither", w.Code)
	}
}
