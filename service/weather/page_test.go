package weather

// /weather is a page.
//
// It was http.NotFound for anybody not asking for JSON, so the service whose
// whole product is "what is it doing outside, here and elsewhere" had no
// address a person could open. Reported that way: there is no /weather, so I
// cannot get a forecast for any location.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// stubGeocoder points the lookup at a server this test controls, so nothing
// here depends on the internet or on somebody else's rate limit.
func stubGeocoder(t *testing.T, results []Place) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	was := geocodeURL
	geocodeURL = srv.URL
	t.Cleanup(func() { geocodeURL = was })
}

func TestTheWeatherPageAnswersInsteadOfFourOhFouring(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubGeocoder(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/weather", nil)
	w := httptest.NewRecorder()
	Handler(w, r)

	if w.Code == http.StatusNotFound {
		t.Fatal("/weather is still a 404 for somebody who opened it in a browser")
	}
	body := w.Body.String()
	// The box for anywhere else, which is the second question everybody has.
	if !strings.Contains(body, `action="/weather"`) || !strings.Contains(body, `name="q"`) {
		t.Errorf("no way to look up a place:\n%s", body)
	}
}

// A signed-out reader, or one with no location set, is told which it is rather
// than shown an empty page. Both used to be nothing at all.
func TestThePageSaysWhyItHasNoForecastForYou(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubGeocoder(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/weather", nil)
	w := httptest.NewRecorder()
	Handler(w, r)

	if !strings.Contains(w.Body.String(), "look up any place above") {
		t.Errorf("a reader with no location is given no explanation and no way on:\n%s",
			w.Body.String())
	}
}

// A name that matches several places offers the others. "Cambridge" is a real
// ambiguity and picking one silently is how somebody reads the wrong forecast
// and believes it.
func TestSeveralPlacesWithOneNameAreAllOffered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubGeocoder(t, []Place{
		{Name: "Cambridge", Admin1: "England", Country: "United Kingdom", Lat: 52.2, Lon: 0.12},
		{Name: "Cambridge", Admin1: "Massachusetts", Country: "United States", Lat: 42.3, Lon: -71.1},
	})

	r := httptest.NewRequest(http.MethodGet, "/weather?q=Cambridge", nil)
	w := httptest.NewRecorder()
	Handler(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "Massachusetts") {
		t.Errorf("the second Cambridge is not offered, so the first is presented as "+
			"the only one:\n%s", body)
	}
}

// And a name that matches nothing says so, rather than rendering an empty
// forecast that reads as "no weather".
func TestANameThatMatchesNothingSaysSo(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubGeocoder(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/weather?q=zzzznowhere", nil)
	w := httptest.NewRecorder()
	Handler(w, r)

	if !strings.Contains(w.Body.String(), "Nowhere called") {
		t.Errorf("an unmatched place does not say so:\n%s", w.Body.String())
	}
}

// The JSON half is untouched: it answered before this page existed and the
// tools, the card and every caller still use it.
func TestTheJSONCallerIsStillServed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	r := httptest.NewRequest(http.MethodGet, "/weather?lat=&lon=", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)

	if w.Code == http.StatusNotFound {
		t.Fatal("the JSON path now 404s")
	}
	if !strings.Contains(w.Body.String(), "lat and lon are required") {
		t.Errorf("the JSON path is not answering as JSON:\n%s", w.Body.String())
	}
}

// The link says where it goes, not what its href is.
//
// app.TextLink is TextLink(name, ref) — the text first — which is the opposite
// way round from every other link API and was written backwards here on the
// first try. The page then rendered "/account/place, or look up any place
// above", which reads as a path somebody is expected to type.
func TestTheSetYourLocationLinkReadsAsWords(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubGeocoder(t, nil)

	r := httptest.NewRequest(http.MethodGet, "/weather", nil)
	w := httptest.NewRecorder()
	Handler(w, r)

	body := w.Body.String()
	if strings.Contains(body, `>/account/place<`) {
		t.Errorf("the link's text is its own path:\n%s", body)
	}
}

// Both halves of the page render the same forecast.
//
// They did not. A search rendered the full page — conditions, rain, UV, air,
// the hours, the days — and "where you are" rendered serverCard, the three-fact
// version built for a column on Home. One string replacement missed by an
// indent, so the path most people arrive on was the thin one and the bug was
// invisible to anybody whose account had no location set.
func TestYourLocationGetsTheSamePageAsASearch(t *testing.T) {
	src, err := os.ReadFile("page.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "return serverCard(") {
		t.Error("the page still renders the card on one of its paths, so the " +
			"forecast you get depends on how you arrived")
	}
	if n := strings.Count(string(src), "forecastHTML(f,"); n < 2 {
		t.Errorf("forecastHTML is rendered on %d path(s); both arrivals should get it", n)
	}
}
