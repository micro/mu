// Package tiles is the map under a spatial app: Ordnance Survey raster tiles,
// fetched once and served from disk forever after.
//
// The one thing Mu deliberately did not have. service/routes says so in its own
// comment — "the page draws the route from the polyline Google already
// returned, so there is no map tile to buy" — which was the right call for a
// page that draws one route and the wrong one for anybody building something
// spatial, because a map is the first thing they need and the only thing here
// they would have had to go elsewhere for.
//
// Going elsewhere means an OS Data Hub account, a key to rotate and a second
// bill. That is precisely the barrier this product exists to remove, and the
// test in CLAUDE.md is whether the caller is spared an account rather than
// whether we wrote the backend.
//
// # Why a tile is a service
//
// By the definition: request in, response out, deterministic given the data,
// callable by anything. z/x/y names exactly one image and always the same one.
//
// # Free, and why that is affordable
//
// Tiles cost nothing. Not per request and not on the first fetch either, which
// was the first design and was too clever: a price that depends on whether
// somebody else happened to look at Snowdonia first is a price nobody can
// predict, and a basemap you have to think about the cost of is one you build
// something else on.
//
// It is affordable because tiles are immutable in a way almost nothing else
// here is. OS does not redraw last week's Snowdonia. So a region is fetched
// once, ever, by anybody, and served from disk forever after — which means the
// bill is bounded by how much of Britain has been looked at rather than by how
// often. An instance that has served the Lake District has finished paying for
// the Lake District.
//
// What stands in for the price is a limit on cold fetches per account per hour,
// because free means nothing else throttles what this instance spends at
// Ordnance Survey. That is CLAUDE.md's own division of labour — credits price
// real cost, rate limits stop bots — applied to a case where the real cost is
// bounded and the bot is the risk. See limit.go.
package tiles

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/service"
	"mu/internal/settings"
)

// Server is the service handler.
type Server struct{}

// maxZoom is as far in as the OS raster styles go. Past it a client should
// scale the last tile rather than ask for one that does not exist.
const maxZoom = 20

// styles are the OS raster layers, by the name a caller uses.
//
// Three rather than all of them: the ones that answer different questions. Road
// is the default street map, Outdoor adds rights of way and contours — which is
// the reason to reach for OS at all — and Light is the quiet basemap you draw
// your own data on top of.
var styles = map[string]string{
	"road":    "Road_3857",
	"outdoor": "Outdoor_3857",
	"light":   "Light_3857",
}

// StyleNames is what a caller may ask for, in a stable order.
func StyleNames() []string { return []string{"road", "outdoor", "light"} }

// Configured reports whether this instance can fetch a tile it has not got.
//
// It can still serve every tile it already holds without a key, which is worth
// knowing: an instance whose key has lapsed keeps working over the region it
// has already seen rather than going blank.
func Configured() bool { return settings.Get("OS_MAPS_KEY") != "" }

// TileRequest names one image.
type TileRequest struct {
	Z     int    `json:"z" description:"Zoom level, 0 to 20"`
	X     int    `json:"x" description:"Tile column at that zoom"`
	Y     int    `json:"y" description:"Tile row at that zoom"`
	Style string `json:"style" description:"road, outdoor or light — outdoor has rights of way and contours"`
}

// TileResponse is where the image is, rather than the image.
//
// A URL and not bytes, deliberately. A tile is 20-60KB of PNG and a map asks
// for forty of them; handing that back through a JSON tool response as base64
// would be a third larger again and would make an agent's context useless. What
// a caller wants is a URL their map library can put in a <img> or a raster
// source, and that is what this is.
type TileResponse struct {
	URL    string `json:"url" description:"Where to fetch the image — usable directly as a raster tile source"`
	Cached bool   `json:"cached" description:"True when this instance already held the tile, which means it cost nothing"`
	Style  string `json:"style" description:"The style that was served"`
}

// Tile resolves one tile to a URL that serves it.
//
// @example {"z": 14, "x": 8146, "y": 5443, "style": "outdoor"}
func (Server) Tile(_ context.Context, req *TileRequest, rsp *TileResponse) error {
	style, err := styleOf(req.Style)
	if err != nil {
		return err
	}
	if err := validZXY(req.Z, req.X, req.Y); err != nil {
		return err
	}
	rsp.Style, rsp.Cached = styleName(req.Style), held(style, req.Z, req.X, req.Y)
	rsp.URL = fmt.Sprintf("/tiles/%s/%d/%d/%d.png", styleName(req.Style), req.Z, req.X, req.Y)
	return nil
}

// AreaRequest is a bounding box.
type AreaRequest struct {
	North float64 `json:"north" description:"Northern edge, in degrees latitude"`
	South float64 `json:"south" description:"Southern edge"`
	East  float64 `json:"east" description:"Eastern edge, in degrees longitude"`
	West  float64 `json:"west" description:"Western edge"`
	Zoom  int     `json:"zoom" description:"Zoom level, 0 to 20"`
	Style string  `json:"style" description:"road, outdoor or light"`
}

// AreaResponse is what covers it.
type AreaResponse struct {
	URLs   []string `json:"urls" description:"Every tile covering the box, row by row from the north-west"`
	Cols   int      `json:"cols" description:"How many tiles across, so the list can be laid out as a grid"`
	Rows   int      `json:"rows" description:"How many tiles down"`
	Cached int      `json:"cached" description:"How many of them this instance already holds"`
	Text   string   `json:"text" description:"A sentence saying what was covered and at what cost"`
}

// Area lists the tiles covering a bounding box.
//
// The method an agent actually wants: "the map around here" is a box, not a
// z/x/y triple, and working out which tiles cover a box is arithmetic nobody
// should have to write twice.
//
// Bounded at areaMax tiles. A whole country at zoom 18 is millions of them, and
// an unbounded answer is a way to spend somebody's balance by mistake.
//
// @example {"north": 54.46, "south": 54.42, "east": -3.18, "west": -3.24, "zoom": 14, "style": "outdoor"}
func (Server) Area(_ context.Context, req *AreaRequest, rsp *AreaResponse) error {
	style, err := styleOf(req.Style)
	if err != nil {
		return err
	}
	if req.Zoom < 0 || req.Zoom > maxZoom {
		return fmt.Errorf("zoom is 0 to %d", maxZoom)
	}
	north, south := req.North, req.South
	if north < south {
		north, south = south, north
	}
	east, west := req.East, req.West
	if east < west {
		east, west = west, east
	}

	x0, y0 := tileAt(north, west, req.Zoom)
	x1, y1 := tileAt(south, east, req.Zoom)
	cols, rows := x1-x0+1, y1-y0+1
	if cols < 1 || rows < 1 {
		return fmt.Errorf("that box does not cover anything")
	}
	if cols*rows > areaMax {
		return fmt.Errorf("that box is %d tiles at zoom %d, and %d is the most this "+
			"will list at once — ask for a smaller box or a lower zoom", cols*rows, req.Zoom, areaMax)
	}

	name := styleName(req.Style)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if held(style, req.Zoom, x, y) {
				rsp.Cached++
			}
			rsp.URLs = append(rsp.URLs, fmt.Sprintf("/tiles/%s/%d/%d/%d.png", name, req.Zoom, x, y))
		}
	}
	rsp.Cols, rsp.Rows = cols, rows
	rsp.Text = fmt.Sprintf("%d tiles (%d×%d) at zoom %d in the %s style. %d are already held "+
		"and cost nothing; the other %d are charged once each and then free for everybody.",
		len(rsp.URLs), cols, rows, req.Zoom, name, rsp.Cached, len(rsp.URLs)-rsp.Cached)
	return nil
}

// areaMax bounds one Area answer.
//
// 400 is a 20×20 grid, which is a large desktop map at one zoom. Past that a
// caller is downloading a region rather than looking at one, and should say so
// by asking again.
const areaMax = 400

// styleOf resolves a caller's style name to the OS layer.
func styleOf(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return styles["road"], nil
	}
	layer, ok := styles[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("no style called %q — there is %s", name,
			strings.Join(StyleNames(), ", "))
	}
	return layer, nil
}

// styleName is the caller's word for a style, defaulted.
func styleName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if _, ok := styles[n]; !ok {
		return "road"
	}
	return n
}

// validZXY refuses a tile that cannot exist, before anything is charged.
func validZXY(z, x, y int) error {
	if z < 0 || z > maxZoom {
		return fmt.Errorf("zoom is 0 to %d", maxZoom)
	}
	n := 1 << uint(z)
	if x < 0 || x >= n || y < 0 || y >= n {
		return fmt.Errorf("there is no tile %d/%d at zoom %d — the grid is %d by %d", x, y, z, n, n)
	}
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("tiles", "service register failed: %v", err)
	}
}

// Spec declares the service.
var Spec = service.Spec{
	Name:        "tiles",
	Handler:     new(Server),
	Description: "Ordnance Survey map tiles for Britain — the basemap under anything spatial",
	Page:        "/tiles",
	Icon:        "tiles.svg",
	Endpoints: map[string]service.Endpoint{
		// Neither declares a Cost, because tiles are free — see the package
		// comment. What bounds them is a limit on cold fetches per account per
		// hour, which is a different mechanism for a different job.
		"Tile": {
			Doc: "The URL for one map tile, by zoom, column and row. Styles: road, " +
				"outdoor (rights of way and contours), light (a quiet basemap to draw on). " +
				"Free. Britain only",
		},
		"Area": {
			Doc: "Every tile covering a bounding box at a zoom level, as URLs, row by row " +
				"from the north-west, and how many are already held. What to ask for when " +
				"you want the map around a place rather than one numbered tile",
		},
	},
}

// key is where a tile lives in the blob store.
//
// The style and the coordinates and nothing else, because that is everything
// that identifies the image. No account in it: a tile is the same tile whoever
// asked, and scoping the cache per account would mean paying for Snowdonia once
// per user.
func key(layer string, z, x, y int) string {
	return "tiles/" + layer + "/" + strconv.Itoa(z) + "/" + strconv.Itoa(x) + "/" +
		strconv.Itoa(y) + ".png"
}

// held reports whether this instance already has a tile.
func held(layer string, z, x, y int) bool {
	_, err := blob.Get(key(layer, z, x, y))
	return err == nil
}

// fetch gets a tile from Ordnance Survey and keeps it.
//
// The charge happens here and only here — in the one place that knows the tile
// was not already held. A caller cannot reach the provider any other way, which
// is the same argument service/mail's outbound.go makes about having one door.
func fetch(owner, layer string, z, x, y int) ([]byte, error) {
	k := key(layer, z, x, y)
	if b, err := blob.Get(k); err == nil {
		return b, nil
	}
	apiKey := settings.Get("OS_MAPS_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("this instance has no Ordnance Survey key, so it can only " +
			"serve tiles it already holds")
	}
	// Nothing is charged. What is checked is how many tiles this account has
	// already made this instance go and fetch this hour — the limit stands in
	// for the price, and it is checked before the provider is called because
	// after that we have been billed whatever we decide. See limit.go.
	if err := mayFetch(owner); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.os.uk/maps/raster/v1/zxy/%s/%d/%d/%d.png?key=%s",
		layer, z, x, y, apiKey)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("could not reach Ordnance Survey: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		// Outside Britain, which is most of the world. Said as itself rather
		// than as a provider error, because it is the first thing anybody hits.
		return nil, fmt.Errorf("no tile there — Ordnance Survey covers Britain only")
	default:
		return nil, fmt.Errorf("Ordnance Survey returned %d", resp.StatusCode)
	}

	b, err := readAll(resp)
	if err != nil {
		return nil, err
	}
	if err := blob.Put(k, b, "image/png"); err != nil {
		// Worth serving anyway: the caller has paid and the image is in hand.
		// It costs a second fetch later, which is a smaller loss than failing.
		app.Log("tiles", "could not keep %s: %v", k, err)
	}
	return b, nil
}
