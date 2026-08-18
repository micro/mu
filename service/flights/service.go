package flights

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"mu/internal/geo"
	"mu/internal/service"
)

// Server is the go-micro service handler for flights. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools. Aircraft
// positions are public broadcasts, so these are available to guests too.
type Server struct{}

// OverheadRequest asks what is flying near somewhere.
type OverheadRequest struct {
	Near   string  `json:"near" description:"Where to look: a place name, or an airport name or code, e.g. 'Camden, London' or 'LHR'"`
	Lat    float64 `json:"lat" description:"Optional latitude, if the location is already known"`
	Lon    float64 `json:"lon" description:"Optional longitude, if the location is already known"`
	Radius int     `json:"radius" description:"Optional radius in nautical miles (default 30, maximum 250)"`
}

// OverheadResponse is a model-ready list of aircraft.
type OverheadResponse struct {
	Text string `json:"text" description:"Aircraft in the sky near the location, nearest first, with altitude, speed, heading and distance"`
}

// Overhead lists the aircraft currently in the sky near a location, nearest
// first.
// @example {"near": "Camden, London", "radius": 30}
func (Server) Overhead(_ context.Context, req *OverheadRequest, rsp *OverheadResponse) error {
	lat, lon, label, ok := resolve(req.Near, req.Lat, req.Lon)
	if !ok {
		rsp.Text = "Please say where to look — a place name, an airport code, or a latitude and longitude."
		return nil
	}
	radius := req.Radius
	if radius <= 0 {
		radius = 30
	}

	found, err := Near(lat, lon, radius)
	if err != nil {
		rsp.Text = problem(err)
		return nil
	}
	if len(found) == 0 {
		rsp.Text = fmt.Sprintf("Nothing in the air within %d nm of %s right now.%s", radius, label, coverageNote)
		return nil
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Distance < found[j].Distance })

	var b strings.Builder
	fmt.Fprintf(&b, "Aircraft within %d nm of %s:\n", radius, label)
	for i, a := range found {
		if i == 20 {
			fmt.Fprintf(&b, "…and %d more.\n", len(found)-20)
			break
		}
		b.WriteString("- " + a.Describe(true) + "\n")
	}
	rsp.Text = trim(&b)
	return nil
}

// TrackRequest asks where one aircraft is.
type TrackRequest struct {
	Flight string `json:"flight" required:"true" description:"A flight number ('BA117'), a radio callsign ('BAW117'), or an aircraft registration ('G-ZBKL')"`
}

// TrackResponse is a model-ready position report.
type TrackResponse struct {
	Text string `json:"text" description:"Where the aircraft is now: position, altitude, speed, heading and the nearest airport"`
}

// Track finds where an aircraft is right now, by flight number, callsign or
// registration.
// @example {"flight": "BA117"}
func (Server) Track(_ context.Context, req *TrackRequest, rsp *TrackResponse) error {
	q := strings.TrimSpace(req.Flight)
	if q == "" {
		rsp.Text = "Please give a flight number, callsign or registration."
		return nil
	}

	found, err := Lookup(q)
	if err != nil {
		rsp.Text = problem(err)
		return nil
	}
	if len(found) == 0 {
		rsp.Text = fmt.Sprintf("No aircraft is transmitting as %s right now. "+
			"This only sees aeroplanes in the air: one still at the gate, already landed, "+
			"or flying somewhere without receiver coverage will not appear, and none of "+
			"that means the flight is cancelled.", strings.ToUpper(q))
		return nil
	}

	var b strings.Builder
	for _, a := range found {
		b.WriteString(a.Describe(false))
		if near, dist := NearestAirport(a.Lat, a.Lon); near != nil {
			if dist < 5 {
				fmt.Fprintf(&b, "\nAt %s.", near.Label())
			} else {
				fmt.Fprintf(&b, "\n%.0f nm %s of %s (%.4f, %.4f).",
					dist, compass(bearingTo(near.Lat, near.Lon, a.Lat, a.Lon)), near.Label(), a.Lat, a.Lon)
			}
		}
		if a.Age > 60 {
			fmt.Fprintf(&b, "\nLast heard %.0f seconds ago.", a.Age)
		}
		b.WriteString("\n")
	}
	rsp.Text = trim(&b)
	return nil
}

// AirportRequest asks what is happening at an airport.
type AirportRequest struct {
	Code string `json:"code" required:"true" description:"An airport code or name, e.g. 'LHR', 'EGLL' or 'Heathrow'"`
}

// AirportResponse is a model-ready picture of an airport right now.
type AirportResponse struct {
	Text string `json:"text" description:"What is on the ground, on approach and departing, as the aircraft themselves report it"`
}

// Airport reports what is happening at an airport right now — on the ground, on
// approach, and climbing out.
// @example {"code": "LHR"}
func (Server) Airport(_ context.Context, req *AirportRequest, rsp *AirportResponse) error {
	ap := FindAirport(req.Code)
	if ap == nil {
		rsp.Text = fmt.Sprintf("No airport matches %q. Try a three-letter IATA code (LHR), "+
			"a four-letter ICAO code (EGLL), or the name.", strings.TrimSpace(req.Code))
		return nil
	}

	found, err := Near(ap.Lat, ap.Lon, airportRadiusNM)
	if err != nil {
		rsp.Text = problem(err)
		return nil
	}

	ground, arriving, departing := classify(found, ap)
	var b strings.Builder
	fmt.Fprintf(&b, "%s, right now:\n", ap.Label())
	section(&b, "On the ground", ground)
	section(&b, "On approach", arriving)
	section(&b, "Departing", departing)
	if len(ground)+len(arriving)+len(departing) == 0 {
		b.WriteString("Nothing transmitting within " +
			fmt.Sprintf("%d nm.%s", airportRadiusNM, coverageNote) + "\n")
	}
	b.WriteString("\nThis is live position, not the timetable: it says where aeroplanes " +
		"are, not when they were scheduled.")
	rsp.Text = trim(&b)
	return nil
}

// airportRadiusNM is how far around an airport counts as its airspace. Thirty
// nautical miles is roughly where an arrival is established on approach and a
// departure has finished its initial climb — near enough to be about this
// airport, far enough to catch both ends of a movement.
const airportRadiusNM = 30

const unavailable = "Live aircraft positions are unavailable right now."

// problem tells a caller which kind of nothing they got. Being throttled and
// being offline look identical from inside the code and are entirely different
// to somebody waiting for an answer: one of them is worth retrying.
func problem(err error) string {
	if errors.Is(err, errBusy) {
		return "Busy — this instance is pacing its requests to the position network. Try again in a few seconds."
	}
	return unavailable
}

const coverageNote = " Coverage comes from volunteer receivers, so it is thin over oceans and empty over parts of the world with none."

// classify sorts what is near an airport into what it is doing.
//
// None of this is told to us. An aeroplane broadcasts height, speed and rate of
// climb, and whether it is arriving or departing is inferred from those: sinking
// and close is an arrival, climbing is a departure, and a fast aircraft high
// above is neither — it is somebody else's flight passing over on the way
// somewhere. That last category is why the cruise threshold exists: without it
// every airport under a busy airway reports fifty arrivals.
func classify(all []Aircraft, ap *Airport) (ground, arriving, departing []Aircraft) {
	for _, a := range all {
		switch {
		case a.OnGround:
			if a.Distance < 5 {
				ground = append(ground, a)
			}
		case a.Altitude > 18000:
			// Overflying traffic, not this airport's.
		case a.Climb < -300:
			arriving = append(arriving, a)
		case a.Climb > 300:
			departing = append(departing, a)
		}
	}
	for _, s := range [][]Aircraft{ground, arriving, departing} {
		sort.Slice(s, func(i, j int) bool { return s[i].Distance < s[j].Distance })
	}
	return
}

// section writes one labelled group, capped so a busy airport does not return a
// hundred lines nobody reads.
func section(b *strings.Builder, title string, list []Aircraft) {
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s (%d):\n", title, len(list))
	for i, a := range list {
		if i == 10 {
			fmt.Fprintf(b, "…and %d more.\n", len(list)-10)
			break
		}
		b.WriteString("- " + a.Describe(true) + "\n")
	}
}

// Lookup finds an aircraft by whatever a caller typed.
//
// It decides what the query *is* before asking anything, rather than trying
// every reading in turn. Each attempt is a second of a free provider's rate
// limit, and "G-ZBKL" is not a callsign however patiently you ask.
func Lookup(q string) ([]Aircraft, error) {
	q = strings.TrimSpace(q)
	switch {
	case looksLikeHex(q):
		return ByHex(q)
	case looksLikeRegistration(q):
		return ByRegistration(q)
	}
	for _, cs := range Callsigns(q) {
		found, err := ByCallsign(cs)
		if err != nil {
			return nil, err
		}
		if len(found) > 0 {
			return found, nil
		}
	}
	return nil, nil
}

// resolve turns what a caller gave into a point and a name for it. Explicit
// coordinates win, then an airport, then the geocoder — an airport first because
// "LHR" and "Heathrow" are answered locally and correctly, where a geocoder
// might land on a hotel of the same name.
func resolve(near string, lat, lon float64) (float64, float64, string, bool) {
	if lat != 0 || lon != 0 {
		return lat, lon, fmt.Sprintf("%.4f, %.4f", lat, lon), true
	}
	n := strings.TrimSpace(near)
	if n == "" {
		return 0, 0, "", false
	}
	if ap := FindAirport(n); ap != nil {
		return ap.Lat, ap.Lon, ap.Label(), true
	}
	if glat, glon, err := geo.Geocode(n); err == nil {
		return glat, glon, n, true
	}
	return 0, 0, "", false
}

// trim finishes a built body: model-ready text should not end in blank lines.
func trim(b *strings.Builder) string {
	return strings.TrimRight(b.String(), "\n")
}

var Spec = service.Spec{
	Name:        "flights",
	Handler:     new(Server),
	Description: "Where aircraft are, live from ADS-B",
	Page:        "/flights",
	Icon:        "flights.svg",
	Card:        service.Glance(CardHTML),
	Endpoints: map[string]service.Endpoint{
		"Overhead": {Doc: "List the aircraft flying near a location right now, nearest first, with altitude, speed, heading and distance. Live positions broadcast by the aircraft themselves, not a schedule"},
		"Track":    {Doc: "Find where an aircraft is right now by flight number ('BA117'), radio callsign ('BAW117') or registration ('G-ZBKL'). Only sees aeroplanes that are airborne and in range of a receiver — not finding one does not mean the flight was cancelled"},
		"Airport":  {Doc: "Report what is happening at an airport right now: what is on the ground, what is on approach and what is climbing out. Live positions, not the timetable, so it says nothing about scheduled or delayed departures"},
	},
}
