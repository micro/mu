package tiles

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
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/tiles"), "/"), "/")
	if len(parts) != 4 {
		Handler(w, r)
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
	style := styleName(r.URL.Query().Get("style"))

	var b strings.Builder
	b.WriteString(`<div class="tiles-page">`)
	b.WriteString(`<p class="svc-lead">` + Spec.Description + `. Free, and fetched once — a ` +
		`tile is served from here forever after, so a region costs this instance one look ` +
		`however many people use it.</p>`)

	if !Configured() {
		b.WriteString(app.Problem("This instance has no Ordnance Survey key, so it can only " +
			"serve tiles it already holds. An admin can set OS_MAPS_KEY under Maps in " +
			"Settings — the free tier at osdatahub.os.uk is enough."))
	}

	b.WriteString(`<div class="tiles-styles">`)
	for _, s := range StyleNames() {
		b.WriteString(app.PillLink(s, "/tiles?style="+s, s == style))
	}
	b.WriteString(`</div>`)

	// The map, drawn with no library at all: a grid of <img> at a fixed zoom,
	// centred on Britain. A pannable map means a JavaScript dependency and this
	// page is the demonstration rather than the product — what somebody builds
	// on top is their app, and they bring their own MapLibre.
	b.WriteString(preview(style))

	b.WriteString(`<h2 class="svc-h">Pointing a map at it</h2>`)
	// app.BaseURL, not r.Host. Mu runs behind a reverse proxy that forwards to a
	// loopback port, so r.Host is "localhost:8081" and the URL this page hands
	// somebody to paste into MapLibre named an address no client can reach. See
	// internal/origin, which is where this question has one answer.
	b.WriteString(`<pre class="tool-call">` + html.EscapeString(app.BaseURL(r)) + `/tiles/` +
		style + `/{z}/{x}/{y}.png</pre>`)
	b.WriteString(app.NoteHTML(`That is a raster tile URL — give it to MapLibre, Leaflet or ` +
		`OpenLayers as-is. Free: a tile is fetched once, ever, and served from here ` +
		`afterwards. Signing in is only needed for a tile this instance has never seen, ` +
		`and there is a limit of ` + strconv.Itoa(coldLimit()) + ` of those an hour per ` +
		`account so nobody can mirror Britain by accident. Ask <code>tiles_area</code> for ` +
		`the tiles covering a bounding box. ` +
		`Contains OS data © Crown copyright and database right.`))

	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Tiles", Description: Spec.Description, HTML: b.String()})
}

// previewAt is where the sample map is centred, and how far in.
//
// Scafell Pike, at a zoom where the Outdoor style shows what OS is for: paths,
// contours and access land, which is the whole reason to reach for it over a
// road map.
const (
	previewLat  = 54.4542
	previewLon  = -3.2119
	previewZoom = 13
	previewSpan = 3 // tiles either side of centre
)

func preview(style string) string {
	layer, err := styleOf(style)
	if err != nil {
		return ""
	}
	cx, cy := tileAt(previewLat, previewLon, previewZoom)
	_ = layer

	var b strings.Builder
	b.WriteString(`<div class="tiles-grid">`)
	for y := cy - previewSpan; y <= cy+previewSpan; y++ {
		for x := cx - previewSpan; x <= cx+previewSpan; x++ {
			if validZXY(previewZoom, x, y) != nil {
				b.WriteString(`<span class="tiles-blank"></span>`)
				continue
			}
			// loading=lazy, because this is twenty-five images and most of them
			// are below the fold on a phone — and each one that is not fetched
			// is one nobody paid for.
			fmt.Fprintf(&b, `<img class="tiles-img" loading="lazy" alt="" src="/tiles/%s/%d/%d/%d.png">`,
				styleName(style), previewZoom, x, y)
		}
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Card is the service at a glance: whether it can fetch, and how much it holds.
//
// Impersonal — a tile is the same tile for everybody, and what this card says
// is a fact about the instance rather than about the reader.
func Card() string {
	if !Configured() {
		return `<p class="note">No Ordnance Survey key set, so only tiles already held ` +
			`can be served. <a href="/tiles">Tiles →</a></p>`
	}
	return `<p class="note">Ordnance Survey raster tiles for Britain — road, outdoor and ` +
		`light. Fetched once, then free. <a href="/tiles">Tiles →</a></p>`
}
