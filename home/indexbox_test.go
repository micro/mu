package home

// The front page draws the same box as everywhere else.
//
// It drew its own — .lsearch, its own input, its own arrow, its own stylesheet
// — beside app.SearchBox on Home. Two implementations of one control, which was
// survivable only while the control did one thing. The moment it grew a second
// button the copies would have had to be kept in step by hand, which is the
// failure this codebase keeps finding: one implementation, many doors, and the
// doors drift.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/app"
)

func TestTheFrontPageUsesTheOneSearchBox(t *testing.T) {
	app.AgentReady = func() bool { return true }
	t.Cleanup(func() { app.AgentReady = nil })

	got := indexBody()

	if !strings.Contains(got, `id="mu-search-input"`) {
		t.Errorf("the front page is not using app.SearchBox:\n%s", got)
	}
	// The old private copy, by the names only it had.
	for _, gone := range []string{"lsearch-in", "lsearch-go", `class="lsearch"`} {
		if strings.Contains(got, gone) {
			t.Errorf("the front page still carries its own search box (%s)", gone)
		}
	}
	// And it gets the second button from the same place Home does.
	if !strings.Contains(got, `formaction="/agent"`) {
		t.Error("the front page offers no way to ask, on an instance with a model")
	}
}

// A stranger with no account can still press it. /agent refuses without a
// session and redirects to /login carrying the whole URL — query included — so
// they sign in and the question they typed is asked on arrival.
//
// Asserted here because it is the reason the button is allowed on a page nobody
// is signed in to. If /agent ever starts dropping the query on the way through
// login, this button becomes one that loses what you typed.
func TestTheFrontPageAskButtonCarriesTheQuestion(t *testing.T) {
	app.AgentReady = func() bool { return true }
	t.Cleanup(func() { app.AgentReady = nil })

	got := indexBody()
	// A GET form: what was typed is in the URL, which is what survives the
	// login redirect. A POST would be gone by the time anybody signed in.
	if !strings.Contains(got, `method="GET"`) {
		t.Error("the box posts, so a signed-out question cannot survive signing in")
	}
	if !strings.Contains(got, `name="q"`) {
		t.Error("the field is not named q, which is what /agent reads as a prefill")
	}
}

// No model, no offer — on the front page too. A stranger's first impression
// should not be a button that leads to a page saying the instance cannot do it.
func TestTheFrontPageOffersNoAgentWithoutAModel(t *testing.T) {
	app.AgentReady = func() bool { return false }
	t.Cleanup(func() { app.AgentReady = nil })

	got := indexBody()
	if strings.Contains(got, "Ask agent") {
		t.Errorf("an instance with no model invites a stranger to ask it something:\n%s", got)
	}
	if !strings.Contains(got, `>Search</button>`) {
		t.Error("and search has gone with it, so the page does nothing at all")
	}
}

// The whole signed-out path, in one test, because it spans three packages and
// each of them looks correct on its own.
//
// A stranger types a question on the front page and presses Ask agent. /agent
// has no session, so it redirects to /login carrying the URL it was asked for.
// Login sends them on to that URL after they sign in. If any link in that
// chain drops the query string, the button loses what they typed and there is
// nothing on screen to say so — they just arrive at an empty agent page.
func TestASignedOutQuestionSurvivesTheTripThroughLogin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/agent?q=when+is+the+next+train", nil)
	w := httptest.NewRecorder()
	app.RedirectToLogin(w, r)

	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("not a URL: %q", loc)
	}
	back := u.Query().Get("redirect")
	if back != "/agent?q=when is the next train" && back != "/agent?q=when+is+the+next+train" {
		t.Fatalf("login was handed %q, which has lost the question", back)
	}

	// And what login will do with it: same-site, so it is followed rather than
	// dropped for /home. (safeRedirect's rule — a single leading slash.)
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		t.Errorf("%q will be refused as an off-site redirect and the question lost", back)
	}
}
