package tiles

// What a tile service gets wrong silently.

import (
	"context"
	"math"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The Web Mercator arithmetic, against points anybody can check.
//
// This is the one thing here with a right answer that is not obvious by
// reading: get it wrong by one and every map is offset by a tile, which looks
// almost right and is not.
func TestWhichTileAPlaceFallsIn(t *testing.T) {
	for _, c := range []struct {
		name     string
		lat, lon float64
		z, x, y  int
	}{
		// The origin of the whole grid.
		{"null island at zoom 1", 0, 0, 1, 1, 1},
		// Greenwich, where the prime meridian is: at zoom 1 it is the same
		// corner, and at zoom 8 it is a tile anybody can look up.
		{"greenwich", 51.4779, -0.0015, 8, 127, 85},
		// Scafell Pike, which the page centres on.
		{"scafell pike", 54.4542, -3.2119, 13, 4022, 2612},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, y := tileAt(c.lat, c.lon, c.z)
			if x != c.x || y != c.y {
				t.Errorf("got %d/%d, want %d/%d", x, y, c.x, c.y)
			}
		})
	}

	// Every zoom puts a point inside the grid rather than one past its edge,
	// which is what an unclamped floor does at the eastern and southern limits.
	for z := 0; z <= maxZoom; z++ {
		x, y := tileAt(-85.05, 180, z)
		if err := validZXY(z, x, y); err != nil {
			t.Errorf("the south-east corner at zoom %d landed outside the grid: %v", z, err)
		}
	}
}

// A tile that cannot exist is refused before anything is charged, because the
// charge happens on the fetch and a fetch of a nonexistent tile is money spent
// on a 404.
func TestATileThatCannotExistIsRefused(t *testing.T) {
	if err := validZXY(0, 1, 0); err == nil {
		t.Error("zoom 0 has one tile, and 1/0 was accepted")
	}
	if err := validZXY(21, 0, 0); err == nil {
		t.Error("a zoom past the maximum was accepted")
	}
	if err := validZXY(3, -1, 0); err == nil {
		t.Error("a negative column was accepted")
	}
	if err := validZXY(3, 7, 7); err != nil {
		t.Errorf("the last tile at zoom 3 was refused: %v", err)
	}
}

// An unbounded box is a way to spend somebody's balance by mistake: Britain at
// zoom 18 is millions of tiles, and the answer would be a list nobody wanted
// and a bill nobody expected.
func TestABoxIsBounded(t *testing.T) {
	var rsp AreaResponse
	err := Server{}.Area(context.Background(), &AreaRequest{
		North: 60, South: 50, East: 2, West: -8, Zoom: 16,
	}, &rsp)
	if err == nil {
		t.Fatal("a box covering Britain at zoom 16 was accepted")
	}
	if !strings.Contains(err.Error(), "smaller box") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
}

// A box given corner-first-in-any-order is the same box. A caller who says
// south before north should get the map rather than an error about geometry.
func TestABoxIsTheSameWhicheverCornerCameFirst(t *testing.T) {
	one, two := AreaResponse{}, AreaResponse{}
	box := AreaRequest{North: 54.46, South: 54.42, East: -3.18, West: -3.24, Zoom: 13}
	flipped := AreaRequest{North: 54.42, South: 54.46, East: -3.24, West: -3.18, Zoom: 13}

	if err := (Server{}).Area(context.Background(), &box, &one); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Area(context.Background(), &flipped, &two); err != nil {
		t.Fatal(err)
	}
	if len(one.URLs) == 0 {
		t.Fatal("the box covered no tiles")
	}
	if strings.Join(one.URLs, ",") != strings.Join(two.URLs, ",") {
		t.Error("swapping the corners produced a different set of tiles")
	}
	if one.Cols*one.Rows != len(one.URLs) {
		t.Errorf("%d×%d does not match %d urls", one.Cols, one.Rows, len(one.URLs))
	}
}

// The URLs are what a map library takes. A tool that returned base64 in JSON
// would be one nobody could point a map at, which is the whole reason this
// hands back a path.
func TestTheAnswerIsSomethingAMapCanUse(t *testing.T) {
	var rsp TileResponse
	if err := (Server{}).Tile(context.Background(), &TileRequest{
		Z: 13, X: 4022, Y: 2612, Style: "outdoor"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.URL != "/tiles/outdoor/13/4022/2612.png" {
		t.Errorf("the URL is %q, which is not the z/x/y shape every map library takes", rsp.URL)
	}
	if rsp.Style != "outdoor" {
		t.Errorf("the style came back as %q", rsp.Style)
	}
}

// An unknown style is refused and says what there is, rather than quietly
// serving the road map — a caller who asked for contours and got a street map
// would not notice until they looked at a hill.
func TestAnUnknownStyleSaysWhatThereIs(t *testing.T) {
	_, err := styleOf("terrain")
	if err == nil {
		t.Fatal("a style that does not exist was accepted")
	}
	for _, want := range StyleNames() {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %v", want, err)
		}
	}
	// Empty is the default rather than an error: a caller who does not care
	// should not have to choose.
	if got, err := styleOf(""); err != nil || got != styles["road"] {
		t.Errorf("no style given did not default to the road map: %q %v", got, err)
	}
}

// The cache key is the tile and nothing else. An account in it would mean
// paying for Snowdonia once per person, which is the opposite of the pricing.
func TestTheCacheIsSharedByEverybody(t *testing.T) {
	k := key("Outdoor_3857", 13, 4022, 2612)
	if k != "tiles/Outdoor_3857/13/4022/2612.png" {
		t.Errorf("the key is %q", k)
	}
	if strings.Contains(k, "account") || strings.Contains(k, "user") {
		t.Error("the cache key is scoped to somebody, so the same tile would be " +
			"fetched and charged again for the next person")
	}
}

// The preview grid is what the constants say, so changing the span cannot
// silently produce a grid the stylesheet is not laid out for.
func TestThePreviewIsTheGridTheStylesheetExpects(t *testing.T) {
	const across = previewSpan*2 + 1
	if across != 7 {
		t.Errorf("the preview is %d tiles across and .tiles-grid in mu.css is 7", across)
	}
	if got := strings.Count(preview("road"), "<img"); got != across*across {
		t.Errorf("%d images in a %d×%d grid", got, across, across)
	}
	// And it is centred where the comment says, which is the point of choosing
	// a hill rather than a city: Outdoor shows paths and contours there.
	if math.Abs(previewLat-54.4542) > 0.001 {
		t.Error("the preview moved off Scafell Pike without the comment moving with it")
	}
}

// Free means nothing else throttles what this instance spends at Ordnance
// Survey, so the limit is the only thing standing between an enthusiastic
// script and a mirror of Britain. Worth testing as the load-bearing thing it is.
func TestColdFetchesAreBounded(t *testing.T) {
	t.Setenv("TILE_FETCH_PER_HOUR", "3")
	forgetFetches()
	t.Cleanup(forgetFetches)

	for i := 0; i < 3; i++ {
		if err := mayFetch("mapper"); err != nil {
			t.Fatalf("fetch %d of 3 was refused: %v", i+1, err)
		}
	}
	err := mayFetch("mapper")
	if err == nil {
		t.Fatal("the fourth cold fetch was allowed past a limit of three")
	}
	// The refusal says what still works, because "limit reached" on a map that
	// then keeps drawing from cache would read as broken.
	if !strings.Contains(err.Error(), "already held") {
		t.Errorf("the refusal does not say cached tiles still work: %v", err)
	}

	// One account's budget is not another's.
	if err := mayFetch("somebody-else"); err != nil {
		t.Errorf("one account exhausting its own budget blocked another: %v", err)
	}
}

// An anonymous caller cannot make this instance fetch anything. With no charge
// and no account there is nothing to bound, so a shared bucket would be one
// script able to exhaust it for everybody.
func TestNobodyAnonymousCausesAFetch(t *testing.T) {
	forgetFetches()
	t.Cleanup(forgetFetches)
	if err := mayFetch(""); err == nil {
		t.Error("an anonymous caller was allowed to cause a cold fetch")
	}
}

// Tiles are free, and the way to be sure is that no operation exists to charge
// them with. A zero-cost operation left lying around is the kind of thing this
// codebase keeps rediscovering years later.
func TestNothingChargesForATile(t *testing.T) {
	for _, e := range Spec.Endpoints {
		if e.Cost != "" {
			t.Errorf("an endpoint declares Cost %q, but tiles are free", e.Cost)
		}
	}
	src, err := os.ReadFile("tiles.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "ConsumeQuota") {
		t.Error("something here consumes quota, so tiles are not free after all")
	}
}

// The tile URL on the page names the instance's public address.
//
// It was built from r.Host, and Mu runs behind a reverse proxy that forwards to
// a loopback port — so the page told everybody to point MapLibre at
// https://localhost:8081/tiles/road/{z}/{x}/{y}.png. That URL is the whole
// output of this page: a raster tile template you copy into somebody else's map
// library, where an address no client can reach is not a cosmetic mistake.
func TestTheTileURLNamesTheInstance(t *testing.T) {
	r := httptest.NewRequest("GET", "/tiles", nil)
	r.Host = "localhost:8081" // what the proxy actually passes through
	r.Header.Set("X-Forwarded-Host", "micro.mu")
	r.Header.Set("X-Forwarded-Proto", "https")

	w := httptest.NewRecorder()
	Handler(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "https://micro.mu/tiles/road/{z}/{x}/{y}.png") {
		t.Errorf("the tile URL does not name the public address:\n%s", body)
	}
	if strings.Contains(body, "localhost:8081") {
		t.Error("the page is handing out a loopback address to paste into a map")
	}
}
