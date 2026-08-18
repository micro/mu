package archive

// The page, which shows something before you ask.

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Unlike /recall, this shows the shape of the archive with no query. Recall is
// somebody's private correspondence and an unasked-for list of it would be a
// page that reads your mail at you; this is what the instance collected about
// the world, and how much of what is the interesting part.
func TestThePageSaysWhatIsInTheArchiveBeforeYouAsk(t *testing.T) {
	collected(t)

	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/archive", nil))
	body := w.Body.String()

	for _, want := range []string{"news", "video", "blog", `href="/archive?kind=news"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q", want)
		}
	}
}

// Searching keeps the filter, and the filter keeps the search. Losing either on
// a click is what makes a chip row feel broken.
func TestTheChipsAndTheQueryTravelTogether(t *testing.T) {
	collected(t)

	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/archive?q=rates&kind=news", nil))
	body := w.Body.String()

	if !strings.Contains(body, "Rates held") {
		t.Errorf("the result is missing:\n%s", body)
	}
	if strings.Contains(body, "Explaining interest rates") {
		t.Error("filtering to news showed a video")
	}
	// The search survives changing the kind.
	if !strings.Contains(body, "q=rates") {
		t.Error("a chip drops the query")
	}
	// And the box keeps the filter it was searched under.
	if !strings.Contains(body, `<input type="hidden" name="kind" value="news">`) {
		t.Error("searching again would drop the kind")
	}
}

// No session needed: everything it can show is public by construction.
func TestTheArchiveIsReadableWithoutSigningIn(t *testing.T) {
	collected(t)

	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/archive?q=rates", nil))
	if w.Code != 200 {
		t.Errorf("a signed-out reader got %d", w.Code)
	}
}
