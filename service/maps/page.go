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
	b.WriteString(`<div class="maps-page">`)
	b.WriteString(`<p class="svc-lead">` + Spec.Description + `. Free, and fetched once — a ` +
		`tile is served from here forever after, so a region costs this instance one look ` +
		`however many people use it.</p>`)

	if !Configured() {
		b.WriteString(app.Problem("This instance has no Ordnance Survey key, so it can only " +
			"serve tiles it already holds. An admin can set OS_MAPS_KEY under Maps in " +
			"Settings — the free tier at osdatahub.os.uk is enough."))
	}

	b.WriteString(`<div class="maps-styles">`)
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

	b.WriteString(`<h2 class="svc-h">Pointing a map at it</h2>`)
	// app.BaseURL, not r.Host. Mu runs behind a reverse proxy that forwards to a
	// loopback port, so r.Host is "localhost:8081" and the URL this page hands
	// somebody to paste into MapLibre named an address no client can reach. See
	// internal/origin, which is where this question has one answer.
	b.WriteString(`<pre class="tool-call">` + html.EscapeString(app.BaseURL(r)) + `/maps/tiles/` +
		style + `/{z}/{x}/{y}.png</pre>`)
	b.WriteString(app.NoteHTML(`That is a raster tile URL — give it to MapLibre, Leaflet or ` +
		`OpenLayers as-is. Free: a tile is fetched once, ever, and served from here ` +
		`afterwards. Signing in is only needed for a tile this instance has never seen, ` +
		`and there is a limit of ` + strconv.Itoa(coldLimit()) + ` of those an hour per ` +
		`account so nobody can mirror Britain by accident. Ask <code>tiles_area</code> for ` +
		`the tiles covering a bounding box. ` +
		`Contains OS data © Crown copyright and database right.`))

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
	var b strings.Builder
	b.WriteString(`<div class="map-wrap">`)
	b.WriteString(`<div id="map" class="map" data-style="` + html.EscapeString(styleName(style)) +
		`" data-lat="` + strconv.FormatFloat(homeLat, 'f', -1, 64) +
		`" data-lon="` + strconv.FormatFloat(homeLon, 'f', -1, 64) +
		`" data-zoom="` + strconv.Itoa(homeZoom) +
		`" data-min="` + strconv.Itoa(minZoom) +
		`" data-max="` + strconv.Itoa(maxZoom) + `">`)
	b.WriteString(`<div id="map-layer" class="map-layer"></div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="map-controls">` +
		`<button type="button" id="map-in" aria-label="Zoom in">+</button>` +
		`<button type="button" id="map-out" aria-label="Zoom out">&minus;</button>` +
		`<button type="button" id="map-here" aria-label="Go to my location">Locate</button>` +
		`</div>`)
	b.WriteString(`<p class="map-note" id="map-where"></p>`)
	b.WriteString(`</div>`)
	b.WriteString(mapJS)
	return b.String()
}

// Card is the service at a glance: whether it can fetch, and how much it holds.
//
// Impersonal — a tile is the same tile for everybody, and what this card says
// is a fact about the instance rather than about the reader.
func Card() string {
	if !Configured() {
		return `<p class="note">No Ordnance Survey key set, so only tiles already held ` +
			`can be served. <a href="/maps">Maps →</a></p>`
	}
	return `<p class="note">Ordnance Survey raster tiles for Britain — road, outdoor and ` +
		`light. Fetched once, then free. <a href="/maps">Maps →</a></p>`
}

// mapJS moves the map.
//
// Inline rather than in mu.js because it is the whole behaviour of one page and
// belongs with the markup it drives — the same call made for the ask box in the
// inbox and the agent panel on /agents.
//
// The model is three numbers: a centre in tile coordinates, held as floats so a
// half-tile pan is representable, and a zoom. Everything else is derived. A
// render works out which tiles the viewport covers, creates the <img> elements
// that are not there yet, and drops the ones that have gone — so panning across
// Britain does not accumulate ten thousand images.
const mapJS = `<script>
(function(){
  var el=document.getElementById('map'), layer=document.getElementById('map-layer');
  if(!el||!layer) return;
  var SIZE=256, style=el.dataset.style, where=document.getElementById('map-where');
  var z=+el.dataset.zoom, minZ=+el.dataset.min, maxZ=+el.dataset.max;
  var live={}, arrived=0, missing=0, asked=0;
  function done(){ say(); }

  // Web Mercator, the same formula the service uses server-side. Kept as a
  // float so the centre can sit anywhere in a tile rather than snapping.
  function xOf(lon,z){ return (lon+180)/360*Math.pow(2,z); }
  function yOf(lat,z){ var r=lat*Math.PI/180;
    return (1-Math.log(Math.tan(r)+1/Math.cos(r))/Math.PI)/2*Math.pow(2,z); }
  function lonOf(x,z){ return x/Math.pow(2,z)*360-180; }
  function latOf(y,z){ var n=Math.PI-2*Math.PI*y/Math.pow(2,z);
    return 180/Math.PI*Math.atan(0.5*(Math.exp(n)-Math.exp(-n))); }

  var cx=xOf(+el.dataset.lon,z), cy=yOf(+el.dataset.lat,z);

  function render(){
    var w=el.clientWidth, h=el.clientHeight, n=Math.pow(2,z);
    // The pixel at the top-left of the viewport, in world pixels.
    var left=cx*SIZE-w/2, top=cy*SIZE-h/2;
    var x0=Math.floor(left/SIZE), y0=Math.floor(top/SIZE);
    var x1=Math.floor((left+w)/SIZE), y1=Math.floor((top+h)/SIZE);
    var seen={};
    for(var y=y0;y<=y1;y++){
      for(var x=x0;x<=x1;x++){
        if(y<0||y>=n) continue;
        var wx=((x%n)+n)%n;            // wrap east-west, so a pan does not run out
        var k=z+'/'+wx+'/'+y;
        seen[k]=true;
        var img=live[k];
        if(!img){
          img=new Image();
          img.className='map-tile';
          img.alt='';
          img.src='/maps/tiles/'+style+'/'+z+'/'+wx+'/'+y+'.png';
          // A tile outside Britain is a 404 and that is normal here, so it
          // fades out rather than showing a broken image.
          img.onerror=function(){ this.classList.add('map-gap'); missing++; done(); };
          img.onload=function(){ arrived++; done(); };
          layer.appendChild(img);
          live[k]=img;
        }
        img.style.left=(x*SIZE-left)+'px';
        img.style.top=(y*SIZE-top)+'px';
      }
    }
    for(var have in live){
      if(!seen[have]){ layer.removeChild(live[have]); delete live[have]; }
    }
    asked=Object.keys(live).length;
    say();
  }

  // What the map is looking at, and — when nothing came back — why that might
  // be. A tile that fails is hidden, which is right for the sea around Britain
  // and wrong when every tile fails: a map where nothing loaded then looks
  // exactly like a map still loading, which is how "tiles do not load" becomes
  // a report with nothing in it. So it says so.
  function say(){
    if(!where) return;
    var at=latOf(cy,z).toFixed(4)+', '+lonOf(cx,z).toFixed(4)+'  ·  zoom '+z;
    if(asked>0 && arrived===0 && missing>=asked){
      where.textContent=at+'  ·  no tiles came back. Ordnance Survey covers Britain only, '+
        'so this may be outside it — or this instance has no OS_MAPS_KEY and holds none of these yet.';
      return;
    }
    where.textContent=at;
  }

  function zoomTo(next){
    next=Math.max(minZ,Math.min(maxZ,next));
    if(next===z) return;
    var lat=latOf(cy,z), lon=lonOf(cx,z);
    z=next; cx=xOf(lon,z); cy=yOf(lat,z);
    // Every tile is the wrong size now, so start again rather than reposition.
    layer.innerHTML=''; live={}; arrived=0; missing=0;
    render();
  }

  // Dragging. Pointer events, so a finger and a mouse are the same code.
  var dragging=false, lastX=0, lastY=0;
  el.addEventListener('pointerdown',function(e){
    dragging=true; lastX=e.clientX; lastY=e.clientY;
    el.setPointerCapture(e.pointerId); el.classList.add('map-dragging');
  });
  el.addEventListener('pointermove',function(e){
    if(!dragging) return;
    cx-=(e.clientX-lastX)/SIZE; cy-=(e.clientY-lastY)/SIZE;
    lastX=e.clientX; lastY=e.clientY;
    render();
  });
  function stop(e){ dragging=false; el.classList.remove('map-dragging');
    if(e&&e.pointerId!==undefined&&el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId); }
  el.addEventListener('pointerup',stop);
  el.addEventListener('pointercancel',stop);

  el.addEventListener('wheel',function(e){ e.preventDefault(); zoomTo(z+(e.deltaY<0?1:-1)); },{passive:false});
  el.addEventListener('dblclick',function(){ zoomTo(z+1); });

  var zin=document.getElementById('map-in'), zout=document.getElementById('map-out');
  if(zin) zin.onclick=function(){ zoomTo(z+1); };
  if(zout) zout.onclick=function(){ zoomTo(z-1); };

  // Where you are, asked for rather than taken. The browser prompts, and a
  // refusal leaves the map where it was rather than saying anything: somebody
  // who declines has answered the question.
  var here=document.getElementById('map-here');
  if(here) here.onclick=function(){
    if(!navigator.geolocation) return;
    here.disabled=true;
    navigator.geolocation.getCurrentPosition(function(p){
      here.disabled=false;
      z=Math.max(z,14);
      cx=xOf(p.coords.longitude,z); cy=yOf(p.coords.latitude,z);
      layer.innerHTML=''; live={}; arrived=0; missing=0; render();
    },function(){ here.disabled=false; },{timeout:10000});
  };

  window.addEventListener('resize',render);
  render();
})();
</script>`
