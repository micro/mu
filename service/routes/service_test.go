package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withRoutes points the provider at a canned answer and returns the field mask
// it was asked for.
func withRoutes(t *testing.T, body string) *string {
	t.Helper()
	t.Setenv("GOOGLE_API_KEY", "a-key")
	mask := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*mask = r.Header.Get("X-Goog-FieldMask")
		w.Write([]byte(body)) //nolint:errcheck
	}))
	orig := googleRoutesURL
	googleRoutesURL = srv.URL
	t.Cleanup(func() { googleRoutesURL = orig; srv.Close() })
	return mask
}

// polylineExample is the encoding published with the format, for the points
// (38.5, -120.2), (40.7, -120.95), (43.252, -126.453). Interpreted rather than
// raw, because it contains a backtick.
const polylineExample = "_p~iF~ps|U_ulLnnqC_mqNvxq`@"

// A short walk with two turns, and a polyline of three points.
var walkWithSteps = `{"routes":[{
  "duration":"900s","staticDuration":"900s","distanceMeters":1200,
  "polyline":{"encodedPolyline":"` + polylineExample + `"},
  "legs":[{"steps":[
    {"distanceMeters":400,"staticDuration":"300s","navigationInstruction":{"instructions":"Head north on Euston Road"}},
    {"distanceMeters":800,"staticDuration":"600s","navigationInstruction":{"instructions":"Turn right onto Gower Street"}},
    {"distanceMeters":10,"staticDuration":"5s"}
  ]}]}]}`

// TestDirectionsAreTurnsNotANumber — the whole reason this is a second endpoint
// rather than a flag on ETA.
func TestDirectionsAreTurnsNotANumber(t *testing.T) {
	mask := withRoutes(t, walkWithSteps)

	var rsp DirectionsResponse
	if err := (Server{}).Directions(context.Background(), &DirectionsRequest{
		FromLat: 51.5308, FromLon: -0.1238, ToLat: 51.5194, ToLon: -0.1270, Mode: "walk",
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "1. Head north on Euston Road") {
		t.Errorf("no numbered turns:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "2. Turn right onto Gower Street") {
		t.Errorf("the second turn is missing:\n%s", rsp.Text)
	}
	// A step with no instruction is a joining segment, not a turn to announce.
	if strings.Contains(rsp.Text, "3.") {
		t.Errorf("an instruction-less step was numbered as a turn:\n%s", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "15 min") {
		t.Errorf("the summary is missing:\n%s", rsp.Text)
	}
	if !strings.Contains(*mask, "navigationInstruction") || !strings.Contains(*mask, "encodedPolyline") {
		t.Errorf("directions did not ask for steps and the shape: %q", *mask)
	}
}

// TestAnETADoesNotPayForTheShapeOfTheRoad. Google prices this API by what you
// request, so the cheap question has to send the cheap request.
func TestAnETADoesNotPayForTheShapeOfTheRoad(t *testing.T) {
	mask := withRoutes(t, walkWithSteps)

	var rsp ETAResponse
	if err := (Server{}).ETA(context.Background(), &ETARequest{
		FromLat: 51.5308, FromLon: -0.1238, ToLat: 51.5194, ToLon: -0.1270,
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	for _, expensive := range []string{"encodedPolyline", "navigationInstruction", "legs"} {
		if strings.Contains(*mask, expensive) {
			t.Errorf("an ETA asked for %s: %q", expensive, *mask)
		}
	}
}

// TestDirectionsSayWhenTheyCannotDirect — without a key there is no route, and
// a list of turns is exactly what a straight-line estimate cannot invent.
func TestDirectionsSayWhenTheyCannotDirect(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	var rsp DirectionsResponse
	if err := (Server{}).Directions(context.Background(), &DirectionsRequest{
		FromLat: 51.5308, FromLon: -0.1238, ToLat: 51.47, ToLon: -0.4543,
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "cannot give turn-by-turn") {
		t.Errorf("an estimate was passed off as directions: %q", rsp.Text)
	}
}

// TestNearestPutsThemInOrder — the question behind what Google calls a route
// matrix. Nobody wants a matrix; they want to know which one is closest.
func TestNearestPutsThemInOrder(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	var rsp NearestResponse
	// From central London: Heathrow is far, Camden is near. Coordinates would be
	// geocoded from names, so this leans on the no-key estimate, which is
	// straight-line and still gets the order right.
	if err := (Server{}).Nearest(context.Background(), &NearestRequest{
		FromLat: 51.5194, FromLon: -0.1270,
		To: []string{"Heathrow Airport", "Camden Town"},
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	camden := strings.Index(rsp.Text, "Camden")
	heathrow := strings.Index(rsp.Text, "Heathrow")
	if camden < 0 || heathrow < 0 {
		t.Fatalf("both places should be listed:\n%s", rsp.Text)
	}
	if camden > heathrow {
		t.Errorf("the further place came first:\n%s", rsp.Text)
	}
}

// TestNearestIsBounded — each destination is a separate charged lookup, so a
// list of two hundred is two hundred charges somebody did not mean to make.
func TestNearestIsBounded(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")

	many := make([]string, maxCompare+1)
	for i := range many {
		many[i] = "London"
	}
	var rsp NearestResponse
	if err := (Server{}).Nearest(context.Background(), &NearestRequest{
		FromLat: 51.5, FromLon: -0.1, To: many,
	}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "at a time") {
		t.Errorf("an unbounded list was accepted: %q", rsp.Text)
	}
}

// TestPolylineDecodes against the example published with the format.
func TestPolylineDecodes(t *testing.T) {
	got := decodePolyline(polylineExample)
	want := []point{{38.5, -120.2}, {40.7, -120.95}, {43.252, -126.453}}
	if len(got) != len(want) {
		t.Fatalf("decoded %d points, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if diff(got[i].Lat, want[i].Lat) > 1e-5 || diff(got[i].Lon, want[i].Lon) > 1e-5 {
			t.Errorf("point %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Nothing in is nothing out, rather than a point at 0,0 in the Gulf of
	// Guinea joined to the rest of the route by a line across Africa.
	if len(decodePolyline("")) != 0 {
		t.Error("an empty polyline produced points")
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestTheRouteIsDrawnInProportion. One scale for both axes: a straight road
// stretched to fill the box would say the journey turns when it does not.
func TestTheRouteIsDrawnInProportion(t *testing.T) {
	// A journey twice as wide as it is tall, at the equator where longitude is
	// not squeezed.
	svg := draw([]point{{Lat: 0, Lon: 0}, {Lat: 1, Lon: 2}})
	if svg == "" {
		t.Fatal("nothing drawn")
	}
	if !strings.Contains(svg, `viewBox="0 0 640 320"`) {
		t.Errorf("unexpected canvas: %s", svg)
	}
	// Both ends are marked, and one of them is filled so you can tell which.
	if strings.Count(svg, "<circle") != 2 {
		t.Errorf("the ends are not both marked: %s", svg)
	}
	if !strings.Contains(svg, `fill="currentColor"`) {
		t.Error("neither end is filled, so the drawing does not say which way it goes")
	}

	// A journey that goes nowhere has no shape.
	if draw([]point{{Lat: 51.5, Lon: -0.1}}) != "" {
		t.Error("a single point was drawn as a route")
	}
	if draw(nil) != "" {
		t.Error("an absent route was drawn")
	}
}

// TestThePageAsksBeforeItAnswers.
func TestThePageAsksBeforeItAnswers(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest(http.MethodGet, "/routes", nil))
	body := w.Body.String()
	if !strings.Contains(body, `name="from"`) || !strings.Contains(body, `name="to"`) {
		t.Error("the page has nowhere to type the two ends")
	}
	if !strings.Contains(body, `value="transit"`) {
		t.Error("the page cannot be asked about public transport")
	}
	if w.Code != http.StatusOK {
		t.Errorf("a guest got %d — a journey between two public places is public", w.Code)
	}
}

// TestThePageDrawsTheJourney — the drawing is why this is a page rather than a
// paragraph, so it has to survive a rewrite of the handler.
func TestThePageDrawsTheJourney(t *testing.T) {
	withRoutes(t, walkWithSteps)

	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest(http.MethodGet, "/routes?from=51.5308,-0.1238&to=Gower+Street&mode=walk", nil))
	body := w.Body.String()
	if !strings.Contains(body, "rt-map") {
		t.Error("the route was not drawn")
	}
	if !strings.Contains(body, "Head north on Euston Road") {
		t.Error("the turns are not on the page")
	}
}
