package places

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/service"
	"mu/service/wallet"
)

// Server is the go-micro service handler for places. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools. Places data is
// public, so these are available to guests too.
type Server struct{}

// SearchRequest looks for places by name or category, optionally near a place.
type SearchRequest struct {
	Query  string  `json:"query" description:"What to look for, e.g. 'ramen', 'pharmacy', 'Blue Bottle Coffee'"`
	Near   string  `json:"near" description:"Optional place to search near, e.g. 'Shoreditch, London' or 'SF'"`
	Lat    float64 `json:"lat" description:"Optional latitude, if the location is already known"`
	Lon    float64 `json:"lon" description:"Optional longitude, if the location is already known"`
	Radius int     `json:"radius" description:"Optional search radius in metres (default 2000)"`
}

// PlacesResponse is a model-ready list of places.
type PlacesResponse struct {
	Text string `json:"text" description:"Matching places: name, address, distance, and contact where known"`
}

// Search finds places by name or category. If a location is given (as a `near`
// name or explicit lat/lon) it searches nearby and sorts by distance; otherwise
// it does a global lookup.
// @example {"query": "ramen", "near": "Shoreditch, London"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *PlacesResponse) error {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		rsp.Text = "Please say what to search for (e.g. 'coffee', 'pharmacy')."
		return nil
	}
	radius := req.Radius
	if radius <= 0 {
		radius = 2000
	}

	lat, lon, hasLoc := resolveLocation(req.Near, req.Lat, req.Lon)
	if hasLoc {
		results, err := searchNearbyKeyword(q, lat, lon, radius)
		if err != nil {
			return err
		}
		rsp.Text = renderPlaces(fmt.Sprintf("%q near %s", q, locationLabel(req.Near, lat, lon)), results, true)
		return nil
	}

	results, err := searchNominatim(q)
	if err != nil {
		return err
	}
	rsp.Text = renderPlaces(fmt.Sprintf("%q", q), results, false)
	return nil
}

// NearbyRequest finds points of interest near a location.
type NearbyRequest struct {
	Near   string  `json:"near" description:"Place to look around, e.g. 'Camden, London' (or give lat/lon)"`
	Lat    float64 `json:"lat" description:"Optional latitude"`
	Lon    float64 `json:"lon" description:"Optional longitude"`
	Query  string  `json:"query" description:"Optional keyword to filter by, e.g. 'cafe'"`
	Radius int     `json:"radius" description:"Optional radius in metres (default 1000)"`
}

// Nearby lists points of interest near a location. A location is required, given
// as a `near` name or explicit lat/lon.
// @example {"near": "Camden, London", "query": "cafe"}
func (Server) Nearby(_ context.Context, req *NearbyRequest, rsp *PlacesResponse) error {
	radius := req.Radius
	if radius <= 0 {
		radius = 1000
	}
	lat, lon, hasLoc := resolveLocation(req.Near, req.Lat, req.Lon)
	if !hasLoc {
		rsp.Text = "Please give a location to look around (a place name or coordinates)."
		return nil
	}

	var results []*Place
	var err error
	if q := strings.TrimSpace(req.Query); q != "" {
		results, err = searchNearbyKeyword(q, lat, lon, radius)
	} else {
		results, err = findNearbyPlaces(lat, lon, radius)
	}
	if err != nil {
		return err
	}
	rsp.Text = renderPlaces("near "+locationLabel(req.Near, lat, lon), results, true)
	return nil
}

// ETARequest asks how long it takes to get from one place to another.
type ETARequest struct {
	From    string  `json:"from" description:"Where the journey starts, e.g. 'King's Cross, London' (or give from_lat/from_lon)"`
	To      string  `json:"to" description:"Where the journey ends, e.g. 'Heathrow Airport'"`
	FromLat float64 `json:"from_lat" description:"Optional start latitude, if already known"`
	FromLon float64 `json:"from_lon" description:"Optional start longitude, if already known"`
	ToLat   float64 `json:"to_lat" description:"Optional end latitude, if already known"`
	ToLon   float64 `json:"to_lon" description:"Optional end longitude, if already known"`
	Mode    string  `json:"mode" description:"How to travel: drive (default), walk, cycle or transit"`
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
	fromLat, fromLon, hasFrom := resolveLocation(req.From, req.FromLat, req.FromLon)
	if !hasFrom {
		rsp.Text = "Please say where the journey starts (a place name or coordinates)."
		return nil
	}
	toLat, toLon, hasTo := resolveLocation(req.To, req.ToLat, req.ToLon)
	if !hasTo {
		rsp.Text = "Please say where the journey ends (a place name or coordinates)."
		return nil
	}

	mode := travelMode(req.Mode)
	r, err := computeRoute(fromLat, fromLon, toLat, toLon, mode)
	if err != nil {
		return err
	}

	from := locationLabel(req.From, fromLat, fromLon)
	to := locationLabel(req.To, toLat, toLon)
	verb := strings.ToLower(mode)
	if verb == "bicycle" {
		verb = "cycle"
	}

	rsp.Text = fmt.Sprintf("%s to %s by %s: %s, %s.",
		from, to, verb, humanDuration(r.Duration), humanDistance(r.Metres))
	if r.Estimate {
		rsp.Text += " (Estimated from straight-line distance — this instance has no routing key, so treat it as approximate.)"
	}
	return nil
}

// GeocodeRequest resolves a place name or address to coordinates.
type GeocodeRequest struct {
	Address string `json:"address" description:"A place name or address to locate"`
}

// GeocodeResponse is the resolved location.
type GeocodeResponse struct {
	Text string `json:"text" description:"The resolved place with coordinates"`
}

// Geocode resolves a place name or address to coordinates.
// @example {"address": "Eiffel Tower"}
func (Server) Geocode(_ context.Context, req *GeocodeRequest, rsp *GeocodeResponse) error {
	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		rsp.Text = "Please give a place or address to locate."
		return nil
	}
	results, err := searchNominatim(addr)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		rsp.Text = fmt.Sprintf("Couldn't find a location for %q.", addr)
		return nil
	}
	p := results[0]
	name := p.DisplayName
	if name == "" {
		name = p.Name
	}
	rsp.Text = fmt.Sprintf("%s — %.5f, %.5f", name, p.Lat, p.Lon)
	return nil
}

// resolveLocation turns a `near` name or explicit lat/lon into coordinates.
// Explicit coordinates win; otherwise the name is geocoded.
func resolveLocation(near string, lat, lon float64) (float64, float64, bool) {
	if lat != 0 || lon != 0 {
		return lat, lon, true
	}
	if n := strings.TrimSpace(near); n != "" {
		if glat, glon, err := geocode(n); err == nil {
			return glat, glon, true
		}
	}
	return 0, 0, false
}

// locationLabel prefers the human name the caller gave, falling back to coords.
func locationLabel(near string, lat, lon float64) string {
	if n := strings.TrimSpace(near); n != "" {
		return n
	}
	return fmt.Sprintf("%.4f, %.4f", lat, lon)
}

// renderPlaces formats up to 10 places as model-ready text.
func renderPlaces(label string, places []*Place, withDistance bool) string {
	if len(places) == 0 {
		return fmt.Sprintf("No places found for %s.", label)
	}
	if len(places) > 10 {
		places = places[:10]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Places for %s:\n", label)
	for _, p := range places {
		name := p.Name
		if name == "" {
			name = p.DisplayName
		}
		b.WriteString("- " + name)
		if p.Address != "" {
			b.WriteString(" — " + p.Address)
		}
		if withDistance && p.Distance > 0 {
			fmt.Fprintf(&b, " (%s away)", formatDistance(p.Distance))
		}
		var extra []string
		if p.OpeningHours != "" {
			extra = append(extra, p.OpeningHours)
		}
		if p.Phone != "" {
			extra = append(extra, p.Phone)
		}
		if p.Website != "" {
			extra = append(extra, p.Website)
		}
		if len(extra) > 0 {
			b.WriteString(" · " + strings.Join(extra, " · "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// formatDistance renders metres as a short human string.
func formatDistance(m float64) string {
	if m < 1000 {
		return fmt.Sprintf("%.0fm", m)
	}
	return fmt.Sprintf("%.1fkm", m/1000)
}

var Spec = service.Spec{
	Name:        "places",
	Handler:     new(Server),
	Description: "Places, points of interest and geocoding",
	Page:        "/places",
	Icon:        "places.svg",
	Endpoints: map[string]service.Endpoint{
		"ETA":     {Doc: "How long it takes to travel between two places, by road rather than as the crow flies", Cost: wallet.OpPlacesETA},
		"Geocode": {Doc: "Resolve a place name or address to coordinates"},
		"Nearby":  {Doc: "List points of interest near a location", Cost: wallet.OpPlacesNearby},
		"Search":  {Doc: "Find places by name or category, optionally near a location", Cost: wallet.OpPlacesSearch},
	},
}
