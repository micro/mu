package routes

// Three questions about getting somewhere, and they are genuinely three.
//
// ETA is how long. Directions is which way. Nearest is which of these — the
// question behind what Google calls a route matrix, which is a name for the
// shape of the answer rather than the shape of the question. Nobody has ever
// wanted a matrix; they have wanted to know which of five offices is closest,
// which station to walk to, which of these addresses to visit first.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/geo"
	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for routes. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools. A journey
// between two public places is public, so these are available to guests too.
type Server struct{}

// ── How long ────────────────────────────────────────────────────

// ETARequest asks how long it takes to get from one place to another.
type ETARequest struct {
	From     string  `json:"from" description:"Where the journey starts, e.g. 'King's Cross, London' (or give from_lat/from_lon)"`
	To       string  `json:"to" description:"Where the journey ends, e.g. 'Heathrow Airport'"`
	FromLat  float64 `json:"from_lat" description:"Optional start latitude, if already known"`
	FromLon  float64 `json:"from_lon" description:"Optional start longitude, if already known"`
	ToLat    float64 `json:"to_lat" description:"Optional end latitude, if already known"`
	ToLon    float64 `json:"to_lon" description:"Optional end longitude, if already known"`
	Mode     string  `json:"mode" description:"How to travel: drive (default), walk, cycle or transit"`
	DepartAt string  `json:"depart_at,omitempty" description:"When the journey starts, as RFC3339 (e.g. 2026-08-13T08:00:00Z). Defaults to now. Traffic and timetables are read for this time"`
	ArriveBy string  `json:"arrive_by,omitempty" description:"Be there by this time, as RFC3339. Answers when to leave. Cannot be combined with depart_at"`
}

// ETAResponse is how long the journey takes, as model-ready text.
type ETAResponse struct {
	Text string `json:"text" description:"Journey time and distance between the two places"`
}

// ETA gives the travel time and distance between two places, by road rather
// than as the crow flies. Use it to answer whether somewhere is worth going to,
// or when to leave.
// @example {"from": "King's Cross, London", "to": "Heathrow Airport", "mode": "transit"}
func (Server) ETA(_ context.Context, req *ETARequest, rsp *ETAResponse) error {
	j, msg := plan(req)
	if msg != "" {
		rsp.Text = msg
		return nil
	}
	r, err := computeRoute(j.fromLat, j.fromLon, j.toLat, j.toLon, j.mode, j.when, summary)
	if err != nil {
		return err
	}

	rsp.Text = fmt.Sprintf("%s to %s by %s: %s, %s.",
		j.fromLabel, j.toLabel, spoken(j.mode), humanDuration(r.Duration), humanDistance(r.Metres))
	rsp.Text += timing(r, j.when)
	if r.Estimate {
		rsp.Text += estimateNote
	}
	return nil
}

// ── Which way ───────────────────────────────────────────────────

// DirectionsRequest asks for the way there.
type DirectionsRequest struct {
	From     string  `json:"from" description:"Where the journey starts, e.g. 'King's Cross, London' (or give from_lat/from_lon)"`
	To       string  `json:"to" description:"Where the journey ends, e.g. 'Heathrow Airport'"`
	FromLat  float64 `json:"from_lat" description:"Optional start latitude, if already known"`
	FromLon  float64 `json:"from_lon" description:"Optional start longitude, if already known"`
	ToLat    float64 `json:"to_lat" description:"Optional end latitude, if already known"`
	ToLon    float64 `json:"to_lon" description:"Optional end longitude, if already known"`
	Mode     string  `json:"mode" description:"How to travel: drive (default), walk, cycle or transit"`
	DepartAt string  `json:"depart_at,omitempty" description:"When the journey starts, as RFC3339. Defaults to now"`
	ArriveBy string  `json:"arrive_by,omitempty" description:"Be there by this time, as RFC3339. Cannot be combined with depart_at"`
}

// DirectionsResponse is the way there, turn by turn.
type DirectionsResponse struct {
	Text string `json:"text" description:"The journey summary followed by numbered turn-by-turn instructions"`
}

// Directions gives the turn-by-turn route between two places.
// @example {"from": "King's Cross, London", "to": "British Museum", "mode": "walk"}
func (Server) Directions(_ context.Context, req *DirectionsRequest, rsp *DirectionsResponse) error {
	j, msg := plan(&ETARequest{
		From: req.From, To: req.To, FromLat: req.FromLat, FromLon: req.FromLon,
		ToLat: req.ToLat, ToLon: req.ToLon, Mode: req.Mode,
		DepartAt: req.DepartAt, ArriveBy: req.ArriveBy,
	})
	if msg != "" {
		rsp.Text = msg
		return nil
	}
	r, err := computeRoute(j.fromLat, j.fromLon, j.toLat, j.toLon, j.mode, j.when, full)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s to %s by %s: %s, %s.",
		j.fromLabel, j.toLabel, spoken(j.mode), humanDuration(r.Duration), humanDistance(r.Metres))
	b.WriteString(timing(r, j.when))
	if r.Estimate {
		// No key means no route, and a list of turns is exactly what an estimate
		// cannot invent. Say so rather than return a heading and a distance
		// dressed up as directions.
		b.WriteString(" I cannot give turn-by-turn directions on this instance — " +
			"live routing is unavailable, so this is a straight-line estimate.")
		rsp.Text = b.String()
		return nil
	}
	if len(r.Steps) == 0 {
		b.WriteString(" No turn-by-turn instructions are available for this journey.")
		rsp.Text = b.String()
		return nil
	}

	b.WriteString("\n\n")
	for i, s := range r.Steps {
		fmt.Fprintf(&b, "%d. %s", i+1, s.Text)
		if s.Metres > 0 {
			fmt.Fprintf(&b, " (%s)", humanDistance(s.Metres))
		}
		b.WriteString("\n")
	}
	rsp.Text = strings.TrimSpace(b.String())
	return nil
}

// ── Which of these ──────────────────────────────────────────────

// NearestRequest asks which of several places is closest to travel to.
type NearestRequest struct {
	From    string   `json:"from" description:"Where you are starting from, e.g. 'Shoreditch, London' (or give from_lat/from_lon)"`
	FromLat float64  `json:"from_lat" description:"Optional start latitude, if already known"`
	FromLon float64  `json:"from_lon" description:"Optional start longitude, if already known"`
	To      []string `json:"to" required:"true" description:"The places to compare, e.g. ['Heathrow', 'Gatwick', 'Stansted']"`
	Mode    string   `json:"mode" description:"How to travel: drive (default), walk, cycle or transit"`
}

// NearestResponse is those places, sorted by how long they take to reach.
type NearestResponse struct {
	Text string `json:"text" description:"The places ordered by travel time, nearest first, with time and distance for each"`
}

// maxCompare bounds one Nearest call. Each destination is a routing request, so
// a list of two hundred is two hundred charges somebody did not mean to make.
const maxCompare = 10

// Nearest says which of several places is quickest to reach, in order.
// @example {"from": "Shoreditch, London", "to": ["Heathrow Airport", "Gatwick Airport", "Stansted Airport"]}
func (Server) Nearest(_ context.Context, req *NearestRequest, rsp *NearestResponse) error {
	fromLat, fromLon, ok := locate(req.From, req.FromLat, req.FromLon)
	if !ok {
		rsp.Text = "Please say where you are starting from (a place name or coordinates)."
		return nil
	}
	if len(req.To) == 0 {
		rsp.Text = "Please give the places to compare."
		return nil
	}
	if len(req.To) > maxCompare {
		rsp.Text = fmt.Sprintf("That is %d places. I compare up to %d at a time — "+
			"each one is a separate routing lookup.", len(req.To), maxCompare)
		return nil
	}
	mode, known := travelMode(req.Mode)
	if !known {
		rsp.Text = unknownMode(req.Mode)
		return nil
	}

	type result struct {
		label string
		r     route
		err   error
	}
	var found []result
	for _, name := range req.To {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		lat, lon, ok := locate(name, 0, 0)
		if !ok {
			found = append(found, result{label: name, err: fmt.Errorf("could not be found")})
			continue
		}
		r, err := computeRoute(fromLat, fromLon, lat, lon, mode, when{}, summary)
		found = append(found, result{label: name, r: r, err: err})
	}

	// Reachable first, in order; then the ones that could not be worked out, so
	// a name nobody could find is visible rather than silently dropped.
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i].err == nil) != (found[j].err == nil) {
			return found[i].err == nil
		}
		return found[i].r.Duration < found[j].r.Duration
	})

	var b strings.Builder
	fmt.Fprintf(&b, "From %s by %s, nearest first:\n", locationLabel(req.From, fromLat, fromLon), spoken(mode))
	estimated := false
	for _, f := range found {
		if f.err != nil {
			fmt.Fprintf(&b, "- %s — %s\n", f.label, f.err)
			continue
		}
		fmt.Fprintf(&b, "- %s — %s, %s", f.label, humanDuration(f.r.Duration), humanDistance(f.r.Metres))
		if d := f.r.Delay(); d > 0 {
			fmt.Fprintf(&b, " (%s of it traffic)", humanDuration(d))
		}
		b.WriteString("\n")
		estimated = estimated || f.r.Estimate
	}
	rsp.Text = strings.TrimSpace(b.String())
	if estimated {
		rsp.Text += estimateNote
	}
	return nil
}

// ── Shared ──────────────────────────────────────────────────────

// journey is a request that has been read: both ends found, mode understood,
// times parsed.
type journey struct {
	fromLat, fromLon float64
	toLat, toLon     float64
	fromLabel        string
	toLabel          string
	mode             string
	when             when
}

// plan reads a request, or returns the sentence to say instead. Every refusal
// here is something the caller can fix, so it comes back as an answer rather
// than an error.
func plan(req *ETARequest) (journey, string) {
	fromLat, fromLon, ok := locate(req.From, req.FromLat, req.FromLon)
	if !ok {
		return journey{}, "Please say where the journey starts (a place name or coordinates)."
	}
	toLat, toLon, ok := locate(req.To, req.ToLat, req.ToLon)
	if !ok {
		return journey{}, "Please say where the journey ends (a place name or coordinates)."
	}
	mode, known := travelMode(req.Mode)
	if !known {
		return journey{}, unknownMode(req.Mode)
	}
	w, err := journeyTime(req.DepartAt, req.ArriveBy)
	if err != nil {
		return journey{}, err.Error()
	}
	return journey{
		fromLat: fromLat, fromLon: fromLon, toLat: toLat, toLon: toLon,
		fromLabel: locationLabel(req.From, fromLat, fromLon),
		toLabel:   locationLabel(req.To, toLat, toLon),
		mode:      mode, when: w,
	}, ""
}

// timing is the sentence about traffic and clock times, shared by ETA and
// Directions because they answer the same question with different detail.
func timing(r route, w when) string {
	var s string
	if d := r.Delay(); d > 0 {
		s += fmt.Sprintf(" %s of that is traffic.", humanDuration(d))
	}
	// "When do I leave" is the question behind arrive_by, so answer that rather
	// than making the caller do the subtraction.
	switch {
	case !w.Arrive.IsZero():
		s += fmt.Sprintf(" To arrive by %s, leave at %s.",
			w.Arrive.Format("15:04"), w.Arrive.Add(-r.Duration).Format("15:04"))
	case !w.Depart.IsZero():
		s += fmt.Sprintf(" Leaving at %s, arriving %s.",
			w.Depart.Format("15:04"), w.Depart.Add(r.Duration).Format("15:04"))
	}
	return s
}

// locate turns whatever the caller gave into a point.
func locate(name string, lat, lon float64) (float64, float64, bool) {
	if lat != 0 || lon != 0 {
		return lat, lon, true
	}
	if n := strings.TrimSpace(name); n != "" {
		if glat, glon, err := geo.Geocode(n); err == nil {
			return glat, glon, true
		}
	}
	return 0, 0, false
}

// locationLabel prefers the human name the caller gave, falling back to coords.
func locationLabel(name string, lat, lon float64) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return fmt.Sprintf("%.4f, %.4f", lat, lon)
}

// spoken is the mode as a person says it. The provider's word for a bicycle is
// BICYCLE, and nobody says "by bicycle" when they mean cycling.
func spoken(mode string) string {
	if mode == "BICYCLE" {
		return "cycle"
	}
	return strings.ToLower(mode)
}

func unknownMode(word string) string {
	return fmt.Sprintf("I do not know how to travel by %q. Try one of: %s.", word, modeWords())
}

// estimateNote is deliberately vague about why. From here it may be a missing
// key, a key without the Routes API enabled, or an outage, and none of those is
// the caller's problem. The operator gets the real reason in the log.
const estimateNote = " (Estimated from straight-line distance — live routing is " +
	"unavailable on this instance, so treat it as approximate.)"

// Load registers the routes service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("routes", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:    "routes",
	Handler: new(Server),
	// Not "routing": that is what the thing does, and a service is named for a
	// domain. The same rule is why search became web — search.Search derived the
	// tool name search_search.
	Label:       "Routes",
	Description: "How to get from one place to another — time, traffic and turn-by-turn",
	Page:        "/routes",
	Icon:        "routes.svg",
	Endpoints: map[string]service.Endpoint{
		"ETA": {
			Doc: "How long it takes to travel between two places, by road rather than as the crow flies. " +
				"Can be asked about a future departure, or told when you need to arrive and answer when to leave",
			Cost:    quota.OpRoutesETA,
			Aliases: []string{"places_eta"},
		},
		"Directions": {
			Doc: "The turn-by-turn route between two places, with the distance for each instruction. " +
				"Use ETA instead when only the travel time is wanted — this asks the provider for more and costs more",
			Cost: quota.OpRoutesDirections,
		},
		"Nearest": {
			Doc: "Given a starting point and several destinations, say which is quickest to reach and " +
				"put them in order. Each destination is a separate lookup, so it is charged per destination",
			Cost: quota.OpRoutesETA,
		},
	},
}
