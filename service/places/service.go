package places

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for places. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools. Places data is
// public, so these are available to guests too.
type Server struct{}

// SearchRequest looks for places by name or category, optionally near a place.
type SearchRequest struct {
	Query  string  `json:"query" required:"true" description:"What to look for, e.g. 'ramen', 'pharmacy', 'Blue Bottle Coffee'"`
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

// AddressRequest asks what is at a point.
type AddressRequest struct {
	Lat float64 `json:"lat" required:"true" description:"Latitude, e.g. 51.5308"`
	Lon float64 `json:"lon" required:"true" description:"Longitude, e.g. -0.1238"`
}

// AddressResponse is the place at those coordinates.
type AddressResponse struct {
	Text string `json:"text" description:"The address at those coordinates"`
}

// Address names the place at a set of coordinates — the reverse of
// places_geocode. Use it whenever you have a latitude and longitude and need to
// say where that is.
// @example {"lat": 51.5308, "lon": -0.1238}
func (Server) Address(_ context.Context, req *AddressRequest, rsp *AddressResponse) error {
	// 0,0 is in the Gulf of Guinea and is almost always a field nobody filled
	// in. Saying so beats naming the ocean.
	if req.Lat == 0 && req.Lon == 0 {
		rsp.Text = "Please give a latitude and longitude."
		return nil
	}
	r, err := reverseGeocode(req.Lat, req.Lon)
	if err != nil {
		rsp.Text = err.Error() + "."
		return nil
	}
	// The display name, whole. Shortening it with buildAddress was tried and
	// dropped: that reads road/city/postcode, and a reverse lookup at building
	// zoom often has neither road nor city — King's Cross came back as "Greater
	// London, N1C 4DE" beside a full name that said King's Cross. A shorter
	// answer that has lost the thing you asked about is not shorter.
	rsp.Text = r.DisplayName
	return nil
}

// ElevationRequest asks how high somewhere is.
type ElevationRequest struct {
	Place string  `json:"place" description:"A place name or address, e.g. 'Denver' or 'Ben Nevis'"`
	Lat   float64 `json:"lat" description:"Optional latitude, if the location is already known"`
	Lon   float64 `json:"lon" description:"Optional longitude, if the location is already known"`
}

// ElevationResponse is the height above sea level.
type ElevationResponse struct {
	Text string `json:"text" description:"Height above sea level in metres and feet, with the coordinates it was measured at"`
}

// Elevation returns how high a place is above sea level.
// @example {"place": "Denver, Colorado"}
func (Server) Elevation(_ context.Context, req *ElevationRequest, rsp *ElevationResponse) error {
	lat, lon, ok := resolveLocation(req.Place, req.Lat, req.Lon)
	if !ok {
		if strings.TrimSpace(req.Place) == "" {
			rsp.Text = "Please give a place, or a latitude and longitude."
		} else {
			rsp.Text = fmt.Sprintf("Couldn't find a location for %q.", strings.TrimSpace(req.Place))
		}
		return nil
	}
	metres, err := Elevation(lat, lon)
	if err != nil {
		rsp.Text = "Elevation data is unavailable right now."
		return nil
	}
	rsp.Text = describeElevation(locationLabel(req.Place, lat, lon), lat, lon, metres)
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
		"Address":   {Doc: "Name the place at a latitude and longitude — the reverse of places_geocode. Use it whenever you have coordinates and need to say where that is"},
		"Elevation": {Doc: "How high a place is above sea level, in metres and feet. Sampled from a 90-metre global elevation model, so a summit reads a little under its surveyed height"},
		"Geocode":   {Doc: "Resolve a place name or address to coordinates"},
		"Nearby":    {Doc: "List points of interest near a location", Cost: quota.OpPlacesNearby},
		"Search":    {Doc: "Find places by name or category, optionally near a location", Cost: quota.OpPlacesSearch},
	},
}
