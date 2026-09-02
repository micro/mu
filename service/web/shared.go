package web

// A search you can send somebody.
//
// # The problem the POST solved, and the one it made
//
// Searches are posted here, not put in the URL: what somebody searches the web
// for is the example everybody reaches for when they say "that is private", and
// a query string lands in the browser history and in the access log of whatever
// terminates TLS in front of this instance. See Handler.
//
// The cost was that a result page had no address. You could not bookmark one,
// could not send one to anybody, could not reload without the browser asking to
// re-submit, and the back button came out somewhere strange. A search engine
// whose answers have no address is missing half of what an answer is for.
//
// # An id, not a query
//
// So the post redirects. The search runs, its results are stored under a short
// random id, and the browser is sent to /web/r/<id> — an ordinary page with an
// ordinary address. Bookmarkable, shareable, reloadable, and the back button
// behaves.
//
// The id says nothing. That is the whole trick: the history entry, the log line
// and the link somebody pastes into a chat all carry eight random characters
// rather than what was typed. The query is on this instance's own disk, put
// there by the instance that just ran it, which is a different thing from
// leaking sideways into three systems that had no reason to see it.
//
// # It is a share, so it is readable by whoever has the link
//
// Anybody with the id can read the results, signed in or not. That is what
// "shareable" means, and it is the same bargain as any unguessable link. It
// costs nothing to serve — the results are already fetched, so a shared link
// never calls the provider again and never charges anybody.
//
// A share is not a search history. Nothing here is listed anywhere, nothing is
// keyed on who ran it, and they expire.

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"mu/internal/data"
)

// shareTTL is how long a shared link works.
//
// A week. Long enough that a link sent on Monday still opens on Friday, which
// is the point of being able to send one, and short enough that a page of
// results about a war is not still being served as current a year later.
const shareTTL = 7 * 24 * time.Hour

// maxShares is how many are kept.
//
// The oldest go first when it fills. A cap rather than nothing, because this
// grows with every search anybody runs and an unbounded map on disk is a slow
// leak nobody notices until the file is the biggest thing in the data
// directory.
const maxShares = 2000

// share is one set of results, as they were.
//
// The results are stored rather than the query re-run. A shared link that
// re-searched would show different results to the person who sent it and the
// person who opened it, would call the provider again on every open, and would
// be a way to spend somebody's credits by sending them a link.
type share struct {
	Query   string        `json:"query"`
	Results []BraveResult `json:"results"`
	At      time.Time     `json:"at"`
}

var (
	shareMu sync.Mutex
	shares  = map[string]share{}
)

// LoadShares restores shared results from disk.
//
// Persisted, because a link that stops working when the server restarts is not
// a link. A deploy is a normal event and the whole promise here is an address
// that keeps working.
func LoadShares() {
	var stored map[string]share
	if err := data.LoadJSON("web_shares.json", &stored); err != nil || stored == nil {
		return
	}
	now := time.Now()
	shareMu.Lock()
	defer shareMu.Unlock()
	for id, s := range stored {
		if now.Sub(s.At) < shareTTL {
			shares[id] = s
		}
	}
}

func persistShares() {
	shareMu.Lock()
	snapshot := make(map[string]share, len(shares))
	for id, s := range shares {
		snapshot[id] = s
	}
	shareMu.Unlock()
	data.SaveJSON("web_shares.json", snapshot) //nolint:errcheck
}

// shareID is eight random characters.
//
// Random and not a hash of the query. A hash would give the same id to the same
// search, which sounds like a feature — one address per query, cacheable — and
// is the leak coming back: anybody could confirm that a search had been run
// here by hashing a guess and asking for it. Six bytes is enough that guessing
// one is not a thing anybody does.
func shareID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Share stores a set of results and returns the id to redirect to.
//
// Empty when there is nothing worth an address: no results, or no randomness to
// make an id from. The caller renders the page directly in that case rather
// than sending somebody to a link that says nothing.
func Share(query string, results []BraveResult) string {
	if len(results) == 0 {
		return ""
	}
	id := shareID()
	if id == "" {
		return ""
	}

	shareMu.Lock()
	shares[id] = share{Query: query, Results: results, At: time.Now()}
	if len(shares) > maxShares {
		evictOldest()
	}
	shareMu.Unlock()

	go persistShares()
	return id
}

// evictOldest drops the oldest entries until the map is back under the cap.
// Called with the lock held.
func evictOldest() {
	now := time.Now()
	for id, s := range shares {
		if now.Sub(s.At) >= shareTTL {
			delete(shares, id)
		}
	}
	// Still over after expiring: drop the oldest by date until it fits.
	for len(shares) > maxShares {
		oldest, at := "", time.Time{}
		for id, s := range shares {
			if oldest == "" || s.At.Before(at) {
				oldest, at = id, s.At
			}
		}
		if oldest == "" {
			return
		}
		delete(shares, oldest)
	}
}

// Shared returns a stored set of results, and false when there is none.
func Shared(id string) (string, []BraveResult, bool) {
	if id == "" {
		return "", nil, false
	}
	shareMu.Lock()
	s, ok := shares[id]
	if ok && time.Since(s.At) >= shareTTL {
		delete(shares, id)
		ok = false
	}
	shareMu.Unlock()
	if !ok {
		return "", nil, false
	}
	return s.Query, s.Results, true
}

// resetShares is for the tests.
func resetShares() {
	shareMu.Lock()
	defer shareMu.Unlock()
	shares = map[string]share{}
}
