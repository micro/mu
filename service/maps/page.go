package maps

// Serving the images, and the page over them.

import (
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// maxTileBytes bounds one image. An OS raster tile is 20-60KB; anything past
// this is not a tile and should not be kept as one.
const maxTileBytes = 2 << 20

func readAll(resp *http.Response) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxTileBytes))
	if err != nil {
		return nil, fmt.Errorf("could not read the tile: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("Ordnance Survey returned an empty tile")
	}
	return b, nil
}

// tileAt is which tile a coordinate falls in, at a zoom.
//
// The standard Web Mercator formula. It is here rather than in internal/
// because nothing else needs it and one service using it is not a shared
// concern yet.
func tileAt(lat, lon float64, z int) (x, y int) {
	n := math.Exp2(float64(z))
	x = int(math.Floor((lon + 180) / 360 * n))
	rad := lat * math.Pi / 180
	y = int(math.Floor((1 - math.Log(math.Tan(rad)+1/math.Cos(rad))/math.Pi) / 2 * n))
	// A point exactly on the eastern or southern edge lands one past the grid.
	if last := int(n) - 1; x > last {
		x = last
	}
	if last := int(n) - 1; y > last {
		y = last
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return x, y
}

// TileHandler serves one image, at /tiles/{style}/{z}/{x}/{y}.png.
//
// A real image at a real URL, because that is the only thing a map library can
// use. Every client — MapLibre, Leaflet, OpenLayers, a plain <img> — takes a
// z/x/y template and nothing else, so a tool that returned base64 in JSON would
// be a tool nobody could point a map at.
func TileHandler(w http.ResponseWriter, r *http.Request) {
	// Under the service, like everything else this service serves. It was at
	// /tiles, kept there when the service was renamed on the argument that the
	// URL was pasted into map configs somewhere — which was a guess about a
	// service days old, and it left a top-level route with no service behind
	// it. That is exactly the orphan "service name == route" exists to prevent.
	// /tiles/ still answers, as a redirect: see routes.go.
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/maps/tiles"), "/"), "/")
	if len(parts) != 4 {
		Handler(w, r)
		return
	}
	if parts[0] == "world" {
		worldTile(w, r, parts[1:])
		return
	}
	layer, err := styleOf(parts[0])
	if err != nil {
		app.NotFound(w, r, err.Error())
		return
	}
	z, e1 := strconv.Atoi(parts[1])
	x, e2 := strconv.Atoi(parts[2])
	y, e3 := strconv.Atoi(strings.TrimSuffix(parts[3], ".png"))
	if e1 != nil || e2 != nil || e3 != nil {
		app.NotFound(w, r, "that is not a tile")
		return
	}
	if err := validZXY(z, x, y); err != nil {
		app.NotFound(w, r, err.Error())
		return
	}

	// Who pays for a cold tile. A held one is served to anybody, signed in or
	// not, because it has already been paid for and serving it again costs
	// nothing — which is the whole pricing idea, and it would be a strange one
	// if the second person still had to have an account.
	owner := ""
	if _, acc := auth.TrySession(r); acc != nil {
		owner = acc.ID
	}
	if owner == "" && !held(layer, z, x, y) {
		app.Unauthorized(w, r)
		return
	}

	b, err := fetch(owner, layer, z, x, y)
	if err != nil {
		app.Error(w, r, http.StatusBadGateway, err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/png")
	// A year, and immutable. OS does not redraw last week's Snowdonia, so this
	// is not an optimistic guess about staleness — it is what the data is.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Write(b) //nolint:errcheck
}

// Handler is the page: what this is, and a map you can actually pan.
//
// A service you look at and leave would be card plus tools — see
// api.ServicePage — and this is not that: the page is a map, which is the one
// thing a card cannot be, and the whole point of the service is that you can
// move around it. Rule 2 in that file, and the argument is the same as the one
// that kept /flights.
func Handler(w http.ResponseWriter, r *http.Request) {
	style := "world"
	if raw := r.URL.Query().Get("style"); raw != "" && raw != "world" {
		style = styleName(raw)
	}

	var b strings.Builder
	b.WriteString(`<div class="maps-page page-stack">`)

	if style != "world" && !Configured() {
		b.WriteString(app.Problem("This instance has no Ordnance Survey key, so it can only " +
			"serve tiles it already holds. An admin can set OS_MAPS_KEY under Maps in " +
			"Settings — the free tier at osdatahub.os.uk is enough."))
	}

	b.WriteString(`<div class="form-actions">`)
	b.WriteString(app.PillLink("World", "/maps", style == "world"))
	for _, s := range StyleNames() {
		b.WriteString(app.PillLink(s, "/maps?style="+s, s == style))
	}
	b.WriteString(`</div>`)

	// A map you can move, rather than a picture of one.
	//
	// It was a fixed grid of twenty-five images centred on Scafell Pike, and the
	// comment defended it: "A pannable map means a JavaScript dependency and
	// this page is the demonstration rather than the product." Both halves were
	// wrong. Every service's page is meant to *be* the capability — a page you
	// cannot use is a screenshot — and a slippy map is a hundred lines of plain
	// JavaScript, not a dependency. The convention in this repo is no external
	// dependencies, which argues for writing it rather than against having it.
	b.WriteString(mapPane(style))

	auth.SetCSRFCookie(w, r)
	b.WriteString(strings.Replace(directionsUI, "{{csrf}}", app.CSRFField(auth.CSRFToken(r)), 1))

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Maps", Description: Spec.Description, HTML: b.String()})
}

// homeAt is where the map opens when nothing better is known.
//
// The middle of Britain at a zoom that shows the country, so somebody who
// refuses the location prompt or is in Berlin gets a map rather than an
// apology. Geolocation moves it when the browser offers one and it lands
// inside the tiles that exist.
// A region rather than the whole country. Ordnance Survey holds Britain and
// nothing else, so the further out you go the more of the square is sea with no
// tile behind it — at zoom 6 most of what is on screen is water OS has never
// been asked about, which looks exactly like a map that failed to load. Nine is
// a county, which is a map.
const (
	homeLat  = 51.5074
	homeLon  = -0.1278
	homeZoom = 9
)

// mapPane is the map: a viewport, a layer of tiles positioned inside it, and
// the script that moves them.
//
// Every tile is one <img> at a computed offset, which is how every slippy map
// works and is the whole trick. Panning moves the layer and fills in whatever
// came into view; zooming recomputes the lot. No library, no canvas, no
// WebGL — a hundred lines and it is the same interaction anybody expects.
func mapPane(style string) string {
	min, max := minZoom, maxZoom
	if style == "world" {
		min, max = 1, 19
	} else {
		style = styleName(style)
	}
	var b strings.Builder
	b.WriteString(`<div class="map-wrap page-stack compact-stack">`)
	b.WriteString(`<div id="map" class="map" data-style="` + html.EscapeString(style) +
		`" data-lat="` + strconv.FormatFloat(homeLat, 'f', -1, 64) +
		`" data-lon="` + strconv.FormatFloat(homeLon, 'f', -1, 64) +
		`" data-zoom="` + strconv.Itoa(homeZoom) +
		`" data-min="` + strconv.Itoa(min) +
		`" data-max="` + strconv.Itoa(max) + `">`)
	b.WriteString(`<div id="map-layer" class="map-layer"></div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="form-actions">` +
		`<button type="button" id="map-in" aria-label="Zoom in">+</button>` +
		`<button type="button" id="map-out" aria-label="Zoom out">&minus;</button>` +
		`<button type="button" id="map-here" aria-label="Go to my location">Locate</button>` +
		`</div>`)
	if style == "world" {
		b.WriteString(`<p class="map-note">© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors</p>`)
	}
	b.WriteString(`<p class="map-note" id="map-where"></p>`)
	b.WriteString(`</div>`)
	return b.String()
}

// Card is the service at a glance: whether it can fetch, and how much it holds.
//
// Impersonal — a tile is the same tile for everybody, and what this card says
// is a fact about the instance rather than about the reader.
func Card() string {
	return `<p class="note"><a href="/maps">Explore the world map</a></p>`
}
