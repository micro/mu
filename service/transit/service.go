package transit

// The three methods, and the Spec.
//
// All priced at zero: TfL costs this instance nothing, and quota.json's rule is
// that a call is charged when it costs us something to run. Free also means an
// anonymous agent can use them, which is right for public data.
//
// The doc strings used to say London on every method, because an agent that
// asks for Manchester and gets an empty list concludes the tool is broken
// rather than that the coverage is. A limit stated is a limit; a limit
// discovered is a bug. The limit is now narrower — live tracking is London,
// timetables are wherever an operator has loaded one — and the doc strings say
// exactly that.

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/gtfs"
	"mu/internal/service"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// ── Nearby ──────────────────────────────────────────────────────────────────

// NearbyRequest is a point to look around.
type NearbyRequest struct {
	Lat    float64 `json:"lat" required:"true" description:"Latitude"`
	Lon    float64 `json:"lon" required:"true" description:"Longitude"`
	Radius int     `json:"radius" description:"Metres to search, default 400, max 2000"`
}

// NearbyResponse lists the stops found.
type NearbyResponse struct {
	Text string `json:"text" description:"Stops near the point, nearest first"`
}

// Nearby lists bus stops and stations near a point.
// @example {"lat": 51.5154, "lon": -0.1410}
func (Server) Nearby(_ context.Context, req *NearbyRequest, rsp *NearbyResponse) error {
	if req.Lat == 0 && req.Lon == 0 {
		return fmt.Errorf("lat and lon are required")
	}
	stops, err := nearbyStops(req.Lat, req.Lon, req.Radius)
	if err != nil || len(stops) == 0 {
		// Outside TfL's city, or TfL is down. Either way the timetable knows
		// the same streets, so the caller gets an answer rather than a lesson
		// about which API we happen to speak.
		if idx, near := timetableNear(req.Lat, req.Lon, 12); idx != nil && len(near) > 0 {
			var b strings.Builder
			for _, s := range near {
				name := s.Name
				if name == "" {
					name = s.ID
				}
				fmt.Fprintf(&b, "%s, %.0fm (id %s)\n",
					name, gtfs.DistanceKm(req.Lat, req.Lon, s.Lat, s.Lon)*1000, s.ID)
			}
			rsp.Text = strings.TrimRight(b.String(), "\n")
			return nil
		}
		if err != nil {
			return err
		}
		rsp.Text = "No stops within range. London is covered live; elsewhere depends on " +
			"which timetables this instance has loaded."
		return nil
	}

	var b strings.Builder
	for i, s := range stops {
		if i >= 12 {
			break
		}
		fmt.Fprintf(&b, "%s — %s, %.0fm", s.Name, strings.Join(s.Modes, "/"), s.Distance)
		// The id is printed because it is what transit_arrivals takes, and a
		// caller that has just been shown a stop should not have to search for
		// it again to ask when the bus is.
		fmt.Fprintf(&b, " (id %s)\n", s.ID)
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

// ── Arrivals ────────────────────────────────────────────────────────────────

// ArrivalsRequest names the stop.
type ArrivalsRequest struct {
	Stop string `json:"stop" required:"true" description:"Stop name or id, e.g. 'Oxford Circus' or 940GZZLUOXC"`
}

// ArrivalsResponse is what is coming.
type ArrivalsResponse struct {
	Text string `json:"text" description:"Next arrivals, soonest first"`
}

// Arrivals says what is due at a stop and when.
// @example {"stop": "Oxford Circus"}
func (Server) Arrivals(_ context.Context, req *ArrivalsRequest, rsp *ArrivalsResponse) error {
	name := strings.TrimSpace(req.Stop)
	if name == "" {
		return fmt.Errorf("stop is required — a name or an id")
	}

	// A name is resolved to an id first. Anything with no spaces and some
	// digits is already an id, which avoids a search call for the common case
	// of a caller passing back what nearby just told it.
	id, label := name, name
	var findErr error
	if strings.ContainsAny(name, " ,") || !strings.ContainsAny(name, "0123456789") {
		found, err := findStop(name)
		if err != nil {
			findErr = err
		} else {
			id, label = found.ID, found.Name
		}
	}

	var arrs []arrival
	var err error
	if findErr == nil {
		arrs, err = arrivalsAt(id)
	}
	if findErr != nil || err != nil || len(arrs) == 0 {
		// The timetable, for stops TfL has never heard of. Scheduled rather
		// than live, and labelled as such: "due 23:03" and "4 min away" are
		// different promises and should not be dressed alike.
		if sched, ok := timetableAt(name, 10); ok {
			rsp.Text = renderScheduled(sched)
			return nil
		}
		if findErr != nil {
			return findErr
		}
		if err != nil {
			return err
		}
		rsp.Text = "Nothing due at " + label + " right now."
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", label)
	for i, a := range arrs {
		if i >= 10 {
			break
		}
		towards := a.Destination
		if towards == "" {
			towards = a.Towards
		}
		fmt.Fprintf(&b, "  %s to %s — %s", a.Line, towards, mins(a.Seconds))
		if p := strings.TrimSpace(a.Platform); p != "" && p != "null" {
			fmt.Fprintf(&b, " (%s)", p)
		}
		b.WriteString("\n")
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

// ── Status ──────────────────────────────────────────────────────────────────

// StatusRequest optionally narrows the modes.
type StatusRequest struct {
	Modes string `json:"modes" description:"Optional: tube, dlr, overground, elizabeth-line, tram — comma separated"`
}

// StatusResponse is how the network is doing.
type StatusResponse struct {
	Text string `json:"text" description:"Lines in trouble, and whether everything else is running"`
}

// Status reports which lines are disrupted.
// @example {}
func (Server) Status(_ context.Context, req *StatusRequest, rsp *StatusResponse) error {
	lines, err := statuses(req.Modes)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		rsp.Text = "No line information available."
		return nil
	}

	// The disrupted ones first and in full, then one line confirming the rest
	// are fine. Somebody asking about the network wants to know what is broken,
	// not to read eleven rows saying Good Service.
	var bad []lineStatus
	good := 0
	for _, l := range lines {
		if l.Disrupted {
			bad = append(bad, l)
			continue
		}
		good++
	}

	var b strings.Builder
	if len(bad) == 0 {
		fmt.Fprintf(&b, "Good service on all %d lines.", len(lines))
		rsp.Text = b.String()
		return nil
	}
	for _, l := range bad {
		fmt.Fprintf(&b, "%s — %s", l.Name, l.Status)
		if l.Reason != "" {
			fmt.Fprintf(&b, "\n  %s", l.Reason)
		}
		b.WriteString("\n")
	}
	if good > 0 {
		fmt.Fprintf(&b, "Good service on the other %d.", good)
	}
	rsp.Text = strings.TrimRight(b.String(), "\n")
	return nil
}

// Load registers the service and starts keeping timetables current.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("transit", "service register failed: %v", err)
	}
	StartFeeds()
}

var Spec = service.Spec{
	Name:        "transit",
	Handler:     new(Server),
	Description: "Public transport: stops near you, what is due, and which lines are down",
	Page:        "/transit",
	Icon:        "transit.svg",
	Card:        Card,
	Endpoints: map[string]service.Endpoint{
		"Nearby": {
			Doc: "Bus stops and stations near a point, nearest first, with the id each one " +
				"is called by. London live from TfL; elsewhere from whichever published " +
				"timetables this instance has loaded",
		},
		"Arrivals": {
			Doc: "What is due at a stop and when. Takes a stop name or an id. In London " +
				"this is live from TfL — buses, tube, DLR, Overground and Elizabeth line. " +
				"Elsewhere it is the published timetable, and says so",
		},
		"Status": {
			Doc: "Which lines are delayed, part-suspended or closed right now, and why. " +
				"London only",
		},
	},
}
