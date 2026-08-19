package flights

// The page: what is overhead, and where one aeroplane is.
//
// Two boxes, because there are two questions. "What is that" is asked from
// wherever you are standing, and the browser knows that, so the page offers to
// use it. "Where is BA117" is asked about somebody else's journey and needs
// nothing but the number.
//
// No sign-in. Aircraft broadcast their positions in clear over the public
// airwaves and the source charges nothing to relay them, so there is no cost to
// recover and no reason to ask who is looking.

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Handler serves /flights.
func Handler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	near := strings.TrimSpace(r.URL.Query().Get("near"))
	lat, _ := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, _ := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)

	if app.WantsJSON(r) {
		handleJSON(w, r, q, near, lat, lon)
		return
	}

	var b strings.Builder
	radius := radiusOr(r.URL.Query().Get("radius"))
	b.WriteString(forms(q, near, radius))

	switch {
	case q != "":
		b.WriteString(trackCard(q))
	case near != "" || lat != 0 || lon != 0:
		b.WriteString(overheadCard(near, lat, lon, radius))
	default:
		b.WriteString(`<div class="card"><p class="text-sm text-muted">` +
			`Positions come from ADS-B: aircraft broadcast where they are, volunteer ` +
			`receivers hear them, and this shows what they said. There is no timetable ` +
			`here — an aeroplane that has not taken off is not transmitting, so it cannot ` +
			`be found, and that is not the same as a flight being cancelled.</p></div>`)
	}
	b.WriteString(pageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Flights", "Live aircraft positions from ADS-B", b.String(), r)))
}

func handleJSON(w http.ResponseWriter, r *http.Request, q, near string, lat, lon float64) {
	radius := radiusOr(r.URL.Query().Get("radius"))
	if q != "" {
		found, err := Lookup(q)
		if err != nil {
			app.RespondError(w, http.StatusServiceUnavailable, problem(err))
			return
		}
		app.RespondJSON(w, map[string]any{"query": q, "aircraft": found})
		return
	}
	rlat, rlon, label, ok := resolve(near, lat, lon)
	if !ok {
		app.RespondError(w, http.StatusBadRequest, "give near, or lat and lon, or q for one flight")
		return
	}
	found, err := Near(rlat, rlon, radius)
	if err != nil {
		app.RespondError(w, http.StatusServiceUnavailable, problem(err))
		return
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Distance < found[j].Distance })
	app.RespondJSON(w, map[string]any{
		"location": label, "lat": rlat, "lon": rlon, "radius_nm": radius, "aircraft": found,
	})
}

// forms are the two questions the page answers.
func forms(q, near string, radius int) string {
	return `<div class="card fl-forms">
<form method="GET" action="/flights" class="fl-form">
<label class="fl-label" for="fl-near">What's overhead</label>
<div class="fl-row">
<input id="fl-near" type="text" name="near" value="` + html.EscapeString(near) + `" placeholder="A place or airport — Camden, London or LHR" autocomplete="off">
<select name="radius" aria-label="Range in nautical miles" class="fl-range">` + ranges(radius) + `</select>
<button type="submit">Look</button>
</div>
<a href="#" class="fl-here" onclick="muFlightsHere();return false">Use my location</a>
</form>
<form method="GET" action="/flights" class="fl-form">
<label class="fl-label" for="fl-q">Where's a flight</label>
<div class="fl-row">
<input id="fl-q" type="text" name="q" value="` + html.EscapeString(q) + `" placeholder="BA117, BAW117 or G-ZBKL" autocomplete="off">
<button type="submit">Find</button>
</div>
</form>
</div>
<script>
function muFlightsHere(){
if(!navigator.geolocation){return}
navigator.geolocation.getCurrentPosition(function(p){
location.href='/flights?lat='+p.coords.latitude.toFixed(4)+'&lon='+p.coords.longitude.toFixed(4);
},function(){},{timeout:8000});
}
</script>`
}

// radiusOr reads the radius from the query string, defaulting to thirty
// nautical miles and refusing anything the provider will not serve.
func radiusOr(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 {
		return 30
	}
	if n > maxRadiusNM {
		return maxRadiusNM
	}
	return n
}

// overheadCard is the sky near a place: the scope, then the same aircraft as a
// table. The picture answers where, the table answers what.
func overheadCard(near string, lat, lon float64, radius int) string {
	rlat, rlon, label, ok := resolve(near, lat, lon)
	if !ok {
		return notice("Couldn't find " + html.EscapeString(near) + ". Try an airport code, or a town and country.")
	}
	found, err := Near(rlat, rlon, radius)
	if err != nil {
		return notice(problem(err))
	}
	if len(found) == 0 {
		return notice(fmt.Sprintf("Nothing in the air within %d nm of %s right now.", radius, html.EscapeString(label)))
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Distance < found[j].Distance })

	var b strings.Builder
	b.WriteString(`<div class="card"><h3>` + html.EscapeString(label) + `</h3>`)
	b.WriteString(scope(label, radius, found))
	fmt.Fprintf(&b, `<p class="text-sm text-muted">%d aircraft within %d nm, nearest first.</p>`, len(found), radius)
	b.WriteString(`<table class="fl-table"><thead><tr><th>Flight</th><th>Aircraft</th>` +
		`<th class="fl-num">Altitude</th><th class="fl-num">Speed</th><th class="fl-num">Distance</th></tr></thead><tbody>`)
	for i, a := range found {
		if i == 40 {
			break
		}
		b.WriteString(`<tr><td>` + linkTo(a) + `</td><td>` + html.EscapeString(AircraftType(a.Type)) + `</td>`)
		if a.OnGround {
			b.WriteString(`<td class="fl-num">ground</td>`)
		} else {
			b.WriteString(`<td class="fl-num">` + comma(a.Altitude) + ` ft</td>`)
		}
		fmt.Fprintf(&b, `<td class="fl-num">%.0f kt</td><td class="fl-num">%.0f nm %s</td></tr>`,
			a.Speed, a.Distance, html.EscapeString(compass(a.Bearing)))
	}
	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// trackCard is one aeroplane.
func trackCard(q string) string {
	found, err := Lookup(q)
	if err != nil {
		return notice(problem(err))
	}
	if len(found) == 0 {
		return notice("No aircraft is transmitting as <strong>" + html.EscapeString(strings.ToUpper(q)) +
			"</strong> right now. Only aeroplanes in the air and in range of a receiver appear here.")
	}
	var b strings.Builder
	for _, a := range found {
		b.WriteString(`<div class="card"><h3>` + html.EscapeString(a.Label()) + `</h3>`)
		b.WriteString(`<p class="fl-line">` + html.EscapeString(a.Describe(false)) + `</p>`)
		if near, dist := NearestAirport(a.Lat, a.Lon); near != nil {
			if dist < 5 {
				b.WriteString(`<p class="text-sm text-muted">At ` + html.EscapeString(near.Label()) + `.</p>`)
			} else {
				fmt.Fprintf(&b, `<p class="text-sm text-muted">%.0f nm %s of %s — <a href="/flights?lat=%.4f&amp;lon=%.4f">what else is nearby</a></p>`,
					dist, html.EscapeString(compass(bearingTo(near.Lat, near.Lon, a.Lat, a.Lon))),
					html.EscapeString(near.Label()), a.Lat, a.Lon)
			}
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

// linkTo names an aircraft and links to its own page.
func linkTo(a Aircraft) string {
	id := a.Callsign
	if id == "" {
		id = a.Reg
	}
	if id == "" {
		return html.EscapeString(a.Hex)
	}
	return `<a href="/flights?q=` + html.EscapeString(id) + `">` + html.EscapeString(a.Label()) + `</a>`
}

func notice(msg string) string {
	return `<div class="card"><p class="text-sm text-muted">` + msg + `</p></div>`
}

const pageCSS = `<style>
.fl-forms{display:flex;gap:24px;flex-wrap:wrap}
.fl-form{flex:1;min-width:260px}
.fl-label{display:block;font-size:12px;color:#888;margin-bottom:6px}
.fl-row{display:flex;gap:8px}
.fl-row input{flex:1;min-width:0}
.fl-range{flex:0 0 auto}
.fl-here{font-size:12px;color:#888;display:inline-block;margin-top:6px}
.fl-table{width:100%;border-collapse:collapse;font-size:13px}
.fl-table th{text-align:left;font-weight:normal;color:#888;padding:6px 8px;border-bottom:1px solid #eee}
.fl-table td{padding:6px 8px;border-bottom:1px solid #f4f4f4}
.fl-num{text-align:right;white-space:nowrap}
.fl-line{font-size:14px}
` + scopeCSS + `</style>`

// CardHTML renders the flights card for the home screen.
//
// It asks the browser where it is and shows the three nearest aircraft. Without
// permission it says so and offers the page, rather than guessing a city — the
// answer to "what is overhead" is worthless if it is overhead somewhere else.
func CardHTML(who service.Viewer) string {
	// Rendered here when we know where you are, which is the better answer and
	// the one that works with no browser in the room — see account/place.go.
	// Three aircraft rather than the page's forty: the page has a scope and a
	// table and is a different thing, which is why it is still a page.
	if lat, lon, ok := auth.Located(who.Account); ok {
		if found, err := Near(lat, lon, 25); err == nil {
			return nearestHTML(found)
		}
	}
	return `<div id="flights-card"><div id="flights-card-content" style="font-size:13px;color:#888">
<a href="/flights" style="color:#888">See what's overhead</a></div>
<script>
(function(){
var el=document.getElementById('flights-card-content');
var KEY='mu_flights_lat',KEY2='mu_flights_lon';
var lat=localStorage.getItem(KEY),lon=localStorage.getItem(KEY2);
function show(lat,lon){
fetch('/flights?lat='+lat+'&lon='+lon+'&radius=25',{headers:{'Accept':'application/json'}})
.then(function(r){return r.json()}).then(function(d){
var ac=(d&&d.aircraft)||[];
if(!ac.length){el.innerHTML='<a href="/flights" style="color:#888">Nothing overhead</a>';return}
var h='';
for(var i=0;i<Math.min(3,ac.length);i++){var a=ac[i];
h+='<div style="display:flex;justify-content:space-between;gap:8px"><span>'+
(a.callsign||a.reg||a.hex)+'</span><span style="color:#aaa">'+
(a.on_ground?'ground':(a.altitude||0).toLocaleString()+' ft')+'</span></div>'}
h+='<div style="margin-top:6px"><a href="/flights?lat='+lat+'&lon='+lon+'" style="color:#888">More</a></div>';
el.innerHTML=h}).catch(function(){});
}
if(lat&&lon){show(lat,lon);return}
if(!navigator.geolocation){return}
el.innerHTML='<a href="#" onclick="muFlightsCard();return false" style="color:#555">Enable location for what\'s overhead</a>';
window.muFlightsCard=function(){navigator.geolocation.getCurrentPosition(function(p){
var la=p.coords.latitude.toFixed(4),lo=p.coords.longitude.toFixed(4);
localStorage.setItem(KEY,la);localStorage.setItem(KEY2,lo);show(la,lo)},function(){},{timeout:8000})};
})();
</script></div>`
}

// ranges are the distances the scope offers.
//
// The scope draws to whatever radius it is given, and until this existed the
// only way to change it was to edit the URL. Five steps rather than a free
// number: thirty miles is a town, a hundred is a region, and the provider stops
// at 250 — a box that accepts 47 invites a precision the picture does not have.
func ranges(selected int) string {
	var b strings.Builder
	for _, nm := range []int{10, 30, 60, 100, 250} {
		sel := ""
		if nm == selected {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%d"%s>%d nm</option>`, nm, sel, nm)
	}
	return b.String()
}

// nearestHTML is the three nearest aircraft, for a card.
func nearestHTML(found []Aircraft) string {
	if len(found) == 0 {
		return `<a class="fl-none" href="/flights">Nothing overhead</a>`
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Distance < found[j].Distance })
	var b strings.Builder
	for i, a := range found {
		if i >= 3 {
			break
		}
		alt := "ground"
		if !a.OnGround {
			alt = comma(a.Altitude) + " ft"
		}
		b.WriteString(`<div class="fl-line-row">` + linkTo(a) +
			`<span class="fl-alt">` + html.EscapeString(alt) + `</span></div>`)
	}
	b.WriteString(`<div class="fl-more"><a href="/flights">More →</a></div>`)
	return b.String()
}
