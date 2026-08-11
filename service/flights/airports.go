package flights

// The airport table.
//
// A caller says "Heathrow" or "LHR" or "EGLL"; the provider only understands a
// point and a radius. Something has to hold the coordinates, and the choice is
// between asking a geocoder every time — a network round trip, per request, to
// learn a fact that has not changed since the runway was laid — and carrying the
// list. The list is 3,268 airports with scheduled service, about 450KB in the
// binary, and it makes every lookup local and offline.
//
// Sourced from OurAirports, which is public domain. Regenerating it is a matter
// of filtering their airports.csv to rows with an IATA code and scheduled
// service; see airports_test.go for what the file is required to contain.

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed airports.json
var airportsJSON []byte

// Airport is one aerodrome with scheduled service.
type Airport struct {
	IATA    string  `json:"iata"`    // three letters, what a boarding pass says
	ICAO    string  `json:"icao"`    // four letters, what a flight plan says
	Name    string  `json:"name"`    //
	City    string  `json:"city"`    //
	Country string  `json:"country"` // ISO 3166-1 alpha-2
	Lat     float64 `json:"lat"`     //
	Lon     float64 `json:"lon"`     //
	Big     bool    `json:"big"`     // a major airport, used to break ties
}

// Label is the airport as a person would name it.
func (a *Airport) Label() string {
	s := a.Name
	if a.City != "" && !strings.Contains(strings.ToLower(a.Name), strings.ToLower(a.City)) {
		s += ", " + a.City
	}
	if a.IATA != "" {
		s += " (" + a.IATA + ")"
	}
	return s
}

var (
	airportsOnce sync.Once
	airports     []*Airport
	byIATA       map[string]*Airport
	byICAO       map[string]*Airport
)

func loadAirports() {
	airportsOnce.Do(func() {
		if err := json.Unmarshal(airportsJSON, &airports); err != nil {
			return
		}
		byIATA = make(map[string]*Airport, len(airports))
		byICAO = make(map[string]*Airport, len(airports))
		for _, a := range airports {
			// The file is sorted major airports first, so the first claim on a
			// code is the one worth keeping when two rows collide.
			if a.IATA != "" {
				if _, taken := byIATA[a.IATA]; !taken {
					byIATA[a.IATA] = a
				}
			}
			if a.ICAO != "" {
				if _, taken := byICAO[a.ICAO]; !taken {
					byICAO[a.ICAO] = a
				}
			}
		}
	})
}

// Airports returns the whole table. Callers must not modify it.
func Airports() []*Airport {
	loadAirports()
	return airports
}

// FindAirport resolves what a caller typed to one airport.
//
// Codes win over names, and a three-letter query is read as IATA before ICAO,
// because a person who types three letters means the code on their ticket. Only
// when nothing matches a code does this fall back to matching words, and there
// a major airport beats a minor one — "London" should find Heathrow and not
// London Ashford.
func FindAirport(q string) *Airport {
	loadAirports()
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	up := strings.ToUpper(q)
	if a, ok := byIATA[up]; ok && len(up) == 3 {
		return a
	}
	if a, ok := byICAO[up]; ok && len(up) == 4 {
		return a
	}

	low := strings.ToLower(q)
	var exact, prefix, contains *Airport
	for _, a := range airports {
		name, city := strings.ToLower(a.Name), strings.ToLower(a.City)
		switch {
		case city == low || name == low:
			if exact == nil || (a.Big && !exact.Big) {
				exact = a
			}
		case strings.HasPrefix(name, low) || strings.HasPrefix(city, low):
			if prefix == nil || (a.Big && !prefix.Big) {
				prefix = a
			}
		case strings.Contains(name, low) || strings.Contains(city, low):
			if contains == nil || (a.Big && !contains.Big) {
				contains = a
			}
		}
	}
	for _, a := range []*Airport{exact, prefix, contains} {
		if a != nil {
			return a
		}
	}
	return nil
}

// NearestAirport returns the closest major airport to a point, and how far away
// it is in nautical miles. It exists to give a bare latitude and longitude a
// name: "over the North Sea, 40 nm north-east of Amsterdam" is an answer, and
// "53.4, 4.9" is not.
//
// Minor airports are skipped deliberately. The nearest aerodrome to a point over
// rural France is often an airstrip nobody has heard of, which locates the
// aircraft no better than the coordinates did.
func NearestAirport(lat, lon float64) (*Airport, float64) {
	loadAirports()
	var best *Airport
	bestDist := 0.0
	for _, a := range airports {
		if !a.Big {
			continue
		}
		d := distanceNM(lat, lon, a.Lat, a.Lon)
		if best == nil || d < bestDist {
			best, bestDist = a, d
		}
	}
	return best, bestDist
}
