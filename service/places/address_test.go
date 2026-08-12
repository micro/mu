package places

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubReverse points the reverse geocoder at a canned answer.
func stubReverse(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body)) //nolint:errcheck
	}))
	orig := nominatimReverseURL
	nominatimReverseURL = srv.URL
	t.Cleanup(func() { nominatimReverseURL = orig; srv.Close() })
}

const kingsCross = `{
  "place_id": 1, "lat": "51.5308", "lon": "-0.1238",
  "display_name": "King's Cross Station, Euston Road, London, NW1 2AR, United Kingdom",
  "address": {"road": "Euston Road", "city": "London", "postcode": "NW1 2AR", "country": "United Kingdom"}
}`

// TestCoordinatesGetANameBack. places_geocode turned a name into a point and
// nothing turned a point back — which is the half you need more often, because
// coordinates are what everything else here hands you.
func TestCoordinatesGetANameBack(t *testing.T) {
	stubReverse(t, http.StatusOK, kingsCross)

	var rsp AddressResponse
	if err := (Server{}).Address(context.Background(),
		&AddressRequest{Lat: 51.5308, Lon: -0.1238}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Euston Road") {
		t.Errorf("the answer does not say where it is: %q", rsp.Text)
	}
	if !strings.Contains(rsp.Text, "London") {
		t.Errorf("the answer has no town in it: %q", rsp.Text)
	}
}

// TestNowhereIsAnAnswer — the middle of an ocean is a legitimate question with
// no answer, and saying so is better than an empty string or an error page.
func TestNowhereIsAnAnswer(t *testing.T) {
	stubReverse(t, http.StatusOK, `{"error":"Unable to geocode"}`)

	var rsp AddressResponse
	if err := (Server{}).Address(context.Background(),
		&AddressRequest{Lat: -40.0, Lon: -120.0}, &rsp); err != nil {
		t.Fatalf("an unmapped point produced an error rather than an answer: %v", err)
	}
	if !strings.Contains(rsp.Text, "nothing is mapped") {
		t.Errorf("did not say there is nothing there: %q", rsp.Text)
	}
}

// TestAPointHasToBeOnEarth. Nominatim answers 0,0 with the Gulf of Guinea,
// which is almost always a field nobody filled in rather than a question about
// the sea.
func TestAPointHasToBeOnEarth(t *testing.T) {
	var rsp AddressResponse
	if err := (Server{}).Address(context.Background(), &AddressRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "Please give") {
		t.Errorf("0,0 was looked up rather than questioned: %q", rsp.Text)
	}

	for _, p := range []AddressRequest{{Lat: 91, Lon: 0}, {Lat: 0, Lon: 181}, {Lat: -100, Lon: 5}} {
		if _, err := reverseGeocode(p.Lat, p.Lon); err == nil {
			t.Errorf("%+v is not a point on Earth and was accepted", p)
		}
	}
}

// TestBothDirectionsExist — the pair is the point. A service that can turn a
// name into coordinates and cannot turn them back is half a geocoder.
func TestBothDirectionsExist(t *testing.T) {
	var found int
	for name := range Spec.Endpoints {
		if name == "Geocode" || name == "Address" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("found %d of the two directions in the catalogue", found)
	}
}
