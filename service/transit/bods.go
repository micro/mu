package transit

// Buses, live, from the Bus Open Data Service.
//
// Every bus in England reports its position, and the DfT publishes the lot.
// Until now this service could say where the stops are and what the timetable
// promises, and outside London it could not say where anything actually is.
//
// # SIRI-VM rather than GTFS-RT
//
// BODS publishes the same vehicle positions twice: as SIRI-VM, which is XML,
// and as GTFS Realtime, which is protobuf. They carry the same facts.
//
// XML, because encoding/xml is in the standard library and a protobuf reader is
// not. This repository already hand-rolls secp256k1 and RLP rather than take a
// dependency, so writing a wire-format decoder would be in keeping — but those
// exist because there was no alternative, and here there is one that costs
// nothing. A hundred and fifty lines of protobuf parser to read the same
// numbers is a hundred and fifty lines to get wrong.
//
// # What this does not do
//
// It does not predict. A position is where a bus is, not when it will reach
// you, and turning one into the other means matching a vehicle to a trip in the
// timetable and projecting along the shape — which is a real piece of work and
// is what makes an arrival board. What it answers is "what is moving near me",
// which is a question with a true answer and is the one nobody could ask.

import (
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/settings"
)

// bodsEndpoint is the bus location feed, filtered on the way out.
const bodsEndpoint = "https://data.bus-data.dft.gov.uk/api/v1/datafeed/"

// BusesConfigured reports whether this instance can ask where the buses are.
func BusesConfigured() bool { return settings.Get("BODS_API_KEY") != "" }

// bus is one vehicle, now.
type bus struct {
	Line     string
	Towards  string
	Operator string
	Lat, Lon float64
	KM       float64 // from where the question was asked
	Seen     time.Time
}

// siriVM is the response, read down to what a position needs.
type siriVM struct {
	ServiceDelivery struct {
		VehicleActivity []struct {
			RecordedAt       string `xml:"RecordedAtTime"`
			MonitoredVehicle struct {
				LineRef           string `xml:"LineRef"`
				PublishedLineName string `xml:"PublishedLineName"`
				DirectionRef      string `xml:"DirectionRef"`
				OperatorRef       string `xml:"OperatorRef"`
				DestinationName   string `xml:"DestinationName"`
				Location          struct {
					Lon float64 `xml:"Longitude"`
					Lat float64 `xml:"Latitude"`
				} `xml:"VehicleLocation"`
			} `xml:"MonitoredVehicleJourney"`
		} `xml:"VehicleMonitoringDelivery>VehicleActivity"`
	} `xml:"ServiceDelivery"`
}

// busesNear asks BODS what is moving in a box around a point.
//
// A box rather than a radius because that is what the feed filters on, and
// asking for the whole country and filtering here would be tens of megabytes to
// answer a question about one street.
func busesNear(lat, lon, withinKm float64, limit int) ([]bus, error) {
	key := strings.TrimSpace(settings.Get("BODS_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("this instance has no Bus Open Data key, so it cannot say " +
			"where the buses are — an admin can set BODS_API_KEY under Transit in Settings")
	}
	if lat == 0 && lon == 0 {
		return nil, fmt.Errorf("where? give a latitude and longitude")
	}
	if withinKm <= 0 {
		withinKm = 2
	}
	if withinKm > 25 {
		withinKm = 25
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	// Degrees per kilometre. Latitude is constant; longitude narrows towards
	// the poles, and at Britain's latitude ignoring that makes the box half as
	// wide as asked for.
	dLat := withinKm / 111.0
	dLon := withinKm / (111.0 * math.Cos(lat*math.Pi/180))

	q := url.Values{}
	q.Set("api_key", key)
	// minLongitude, minLatitude, maxLongitude, maxLatitude — the order BODS
	// documents, which is not the order anybody says a bounding box in.
	q.Set("boundingBox", fmt.Sprintf("%.5f,%.5f,%.5f,%.5f",
		lon-dLon, lat-dLat, lon+dLon, lat+dLat))

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(bodsEndpoint + "?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("could not reach the Bus Open Data Service: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("the Bus Open Data Service rejected the key — check " +
			"BODS_API_KEY under Transit in Settings")
	default:
		return nil, fmt.Errorf("the Bus Open Data Service returned %d", resp.StatusCode)
	}

	var feed siriVM
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("could not read the bus positions: %w", err)
	}

	out := make([]bus, 0, len(feed.ServiceDelivery.VehicleActivity))
	for _, v := range feed.ServiceDelivery.VehicleActivity {
		m := v.MonitoredVehicle
		if m.Location.Lat == 0 && m.Location.Lon == 0 {
			continue
		}
		line := strings.TrimSpace(m.PublishedLineName)
		if line == "" {
			line = strings.TrimSpace(m.LineRef)
		}
		b := bus{
			Line:     line,
			Towards:  strings.TrimSpace(m.DestinationName),
			Operator: strings.TrimSpace(m.OperatorRef),
			Lat:      m.Location.Lat,
			Lon:      m.Location.Lon,
		}
		b.KM = distanceKm(lat, lon, b.Lat, b.Lon)
		// The box is square and the question was round, so a corner is further
		// away than asked for.
		if b.KM > withinKm {
			continue
		}
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(v.RecordedAt)); err == nil {
			b.Seen = t
		}
		out = append(out, b)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].KM < out[j].KM })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// distanceKm is the great-circle distance between two points.
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp := (lat2 - lat1) * math.Pi / 180
	dl := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// renderBuses is what is moving, nearest first.
func renderBuses(buses []bus, withinKm float64) string {
	if len(buses) == 0 {
		return fmt.Sprintf("No buses reporting within %.0f km right now. Not every "+
			"operator publishes live positions, so a quiet answer is not always an "+
			"empty road.", withinKm)
	}
	var b strings.Builder
	b.WriteString("Buses moving nearby, nearest first:\n")
	for _, v := range buses {
		line := fmt.Sprintf("- %s", v.Line)
		if v.Towards != "" {
			line += " towards " + v.Towards
		}
		line += fmt.Sprintf(" — %.1f km away", v.KM)
		if !v.Seen.IsZero() {
			if age := time.Since(v.Seen); age > 0 && age < time.Hour {
				line += fmt.Sprintf(", reported %.0fs ago", age.Seconds())
			}
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
