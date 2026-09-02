package app

// Who a request is from, when nobody has signed in.
//
// Two failures are pinned here and they are different in kind. The first is a
// hole: the address a limit counts against was taken from a header anybody can
// set, so every limit keyed on one — the guest allowance, and the three signups
// per address that is the only thing between a hundred welcome grants and one
// script — could be reset per request by typing a different number. The second
// is a fairness problem: an address is a building, not a person.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func req(remote string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// The header is a claim, and a claim from the open internet is worth nothing.
func TestAForwardedHeaderFromAStrangerIsIgnored(t *testing.T) {
	r := req("203.0.113.10:44321", map[string]string{
		"X-Forwarded-For": "198.51.100.1",
	})
	if got := ClientIP(r); got != "203.0.113.10" {
		t.Errorf("ClientIP = %q, want the address it actually came from.\n"+
			"A caller that can name its own address can name a new one per\n"+
			"request, which resets every limit keyed on it — the guest\n"+
			"allowance and the signups-per-address rule both.", got)
	}
	// X-Real-IP is the same claim in a different header.
	r = req("203.0.113.10:44321", map[string]string{"X-Real-IP": "198.51.100.1"})
	if got := ClientIP(r); got != "203.0.113.10" {
		t.Errorf("X-Real-IP from a stranger was believed: %q", got)
	}
}

// And believed from a hop we put there, or a real deployment reads every
// visitor as its own proxy.
func TestAForwardedHeaderFromTheProxyIsBelieved(t *testing.T) {
	for _, peer := range []string{"127.0.0.1:5000", "10.0.0.3:5000", "192.168.1.9:5000"} {
		r := req(peer, map[string]string{"X-Forwarded-For": "198.51.100.1, 10.0.0.3"})
		if got := ClientIP(r); got != "198.51.100.1" {
			t.Errorf("behind a proxy at %s, ClientIP = %q, want the client's own address.\n"+
				"Without this every visitor through nginx shares one bucket.", peer, got)
		}
	}
}

// An operator whose proxy has a public address says so, and then that is the
// whole list — naming one hop is not also saying "and any private one".
func TestNamedProxiesAreTheWholeList(t *testing.T) {
	t.Setenv("TRUSTED_PROXY", "203.0.113.7")
	resetProxies()

	if got := ClientIP(req("203.0.113.7:5000", map[string]string{
		"X-Forwarded-For": "198.51.100.1"})); got != "198.51.100.1" {
		t.Errorf("the named proxy was not believed: %q", got)
	}
	if got := ClientIP(req("10.0.0.3:5000", map[string]string{
		"X-Forwarded-For": "198.51.100.1"})); got != "10.0.0.3" {
		t.Errorf("a private peer was believed although a list was named: %q", got)
	}

	// And back to the default for whatever runs next.
	t.Setenv("TRUSTED_PROXY", "")
	resetProxies()
}

// Rubbish in the header is not an address and must not become one.
func TestRubbishInTheHeaderIsNotAnAddress(t *testing.T) {
	for _, junk := range []string{"not-an-ip", "", "  ", "127.0.0.1; drop table"} {
		r := req("10.0.0.3:5000", map[string]string{"X-Forwarded-For": junk})
		if got := ClientIP(r); got != "10.0.0.3" {
			t.Errorf("X-Forwarded-For %q became the client address %q", junk, got)
		}
	}
}

// ── The mark ────────────────────────────────────────────────────

func TestABrowserIsMarkedOnArrival(t *testing.T) {
	w := httptest.NewRecorder()
	r := req("203.0.113.10:1", nil)

	if ClientID(r) != "" {
		t.Fatal("an unmarked request already has a mark")
	}
	MarkClient(w, r)

	id := ClientID(r)
	if id == "" {
		t.Fatal("the request that was marked cannot read its own mark.\n" +
			"The first call a visitor makes would fall through to the address\n" +
			"limit alone, and on a page that answers on arrival that is most\n" +
			"first questions.")
	}
	if len(w.Result().Cookies()) == 0 {
		t.Error("nothing was sent to the browser, so the mark lasts one request")
	}

	// A browser that has one keeps it.
	w2 := httptest.NewRecorder()
	MarkClient(w2, r)
	if len(w2.Result().Cookies()) != 0 {
		t.Error("a marked browser was marked again, so the ration resets every request")
	}
	if ClientID(r) != id {
		t.Error("the mark changed on a request that already had one")
	}
}

// It is a session cookie and nothing is stored against it. A persistent id on a
// product whose claim is no tracking would be a tracking cookie however
// honestly it was used.
func TestTheMarkDoesNotOutliveTheBrowser(t *testing.T) {
	w := httptest.NewRecorder()
	MarkClient(w, req("203.0.113.10:1", nil))

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no cookie was set")
	}
	c := cookies[0]
	if c.MaxAge != 0 || !c.Expires.IsZero() {
		t.Errorf("the mark outlives the browser: MaxAge=%d Expires=%v", c.MaxAge, c.Expires)
	}
	if !c.HttpOnly {
		t.Error("a script can read the mark")
	}
}

// ── The two ceilings ────────────────────────────────────────────

// One browser gets a share sized for a person, and running out of it does not
// depend on who else is behind the same address.
func TestOneBrowserHasItsOwnShare(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_CLIENT", "2")
	t.Setenv("GUEST_MAX_PER_IP", "100")
	resetRates()

	w := httptest.NewRecorder()
	mine := req("203.0.113.10:1", nil)
	MarkClient(w, mine)

	if !GuestAllowed(mine) || !GuestAllowed(mine) {
		t.Fatal("the first two calls from one browser were refused")
	}
	if GuestAllowed(mine) {
		t.Error("a browser is not limited at all")
	}

	// Somebody else at the same address is untouched — the cafe case, which is
	// the whole reason the mark exists.
	w2 := httptest.NewRecorder()
	theirs := req("203.0.113.10:2", nil)
	MarkClient(w2, theirs)
	if !GuestAllowed(theirs) {
		t.Error("a second person behind the same address was refused because of\n" +
			"the first one's calls — which is every cafe, campus and phone network")
	}
}

// And dropping the mark to get a new share still meets the address ceiling.
func TestClearingTheMarkStillMeetsTheAddressLimit(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_CLIENT", "50")
	t.Setenv("GUEST_MAX_PER_IP", "3")
	resetRates()

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		fresh := req("203.0.113.11:1", nil)
		MarkClient(w, fresh) // a new mark every time, as a cleared browser gets
		if !GuestAllowed(fresh) {
			t.Fatalf("call %d was refused inside the address limit", i+1)
		}
	}
	w := httptest.NewRecorder()
	fresh := req("203.0.113.11:1", nil)
	MarkClient(w, fresh)
	if GuestAllowed(fresh) {
		t.Error("a caller who throws the mark away has no ceiling at all, which\n" +
			"is why the address limit still exists behind it")
	}
}

// Localhost is never limited: a self-hosted instance is one person on their own
// machine, and rationing them against their own server is absurd.
func TestLocalhostIsNeverLimited(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_CLIENT", "1")
	t.Setenv("GUEST_MAX_PER_IP", "1")
	resetRates()

	for _, peer := range []string{"127.0.0.1:1", "[::1]:1"} {
		r := req(peer, nil)
		for i := 0; i < 5; i++ {
			if !GuestAllowed(r) {
				t.Errorf("%s was rate limited on call %d", peer, i+1)
				break
			}
		}
	}
}

// The default has to be generous. Somebody wiring up an agent tries the same
// call repeatedly while they get it working, and a limit that trips during
// evaluation is indistinguishable from a broken endpoint.
func TestTheDefaultAllowanceIsGenerous(t *testing.T) {
	resetRates()
	w := httptest.NewRecorder()
	r := req("192.0.2.99:1", nil)
	MarkClient(w, r)
	for i := 0; i < 40; i++ {
		if !GuestAllowed(r) {
			t.Fatalf("the default refused call %d from one browser — too tight to "+
				"evaluate against", i+1)
		}
	}
}

// A bucket per caller is a leak per caller unless the map is swept.
func TestExpiredBucketsAreSweptAway(t *testing.T) {
	t.Setenv("GUEST_MAX_PER_IP", "1")
	resetRates()

	const n = 20100
	for i := 0; i < n; i++ {
		GuestAllowed(req(fmt.Sprintf("198.51.%d.%d:1", i/250, i%250), nil))
	}
	rateMu.Lock()
	before := len(rates)
	for _, b := range rates {
		b.resetAt = time.Now().Add(-time.Minute)
	}
	rateMu.Unlock()
	if before < n {
		t.Fatalf("only %d buckets were created, expected %d", before, n)
	}

	GuestAllowed(req("203.0.113.200:1", nil))

	rateMu.Lock()
	after := len(rates)
	rateMu.Unlock()
	if after >= before {
		t.Errorf("%d buckets before, %d after expiring them all — nothing is swept, "+
			"so the map grows with every caller that ever called", before, after)
	}
}
