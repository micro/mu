package places

import "testing"

func TestNamedSearchWithDigitsIgnoresGeocoderDrift(t *testing.T) {
	a := SavedSearch{Query: "coffee", Location: "TW12 2AB", Lat: 51.4, Lon: -0.3}
	b := a
	b.Lat = 51.401
	b.Lon = -0.301
	if !sameSearch(a, b) {
		t.Fatal("postcode search duplicated after geocoding")
	}
	b.Location = "TW12 2AC"
	if sameSearch(a, b) {
		t.Fatal("different postcodes collapsed")
	}
	a.Location = "51.4,-0.3"
	b = a
	b.Lat = 51.5
	if sameSearch(a, b) {
		t.Fatal("different coordinate searches collapsed")
	}
}
