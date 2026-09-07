package transit

// The page at /transit, and the card for the home screen.
//
// The page asks the browser where it is, because that is the only thing that
// knows — the same arrangement the weather and prayer cards use. Without a
// location it still shows line status, which is useful to anybody in London
// whether or not they will share where they are standing.

import (
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
)

// Handler serves /transit.
//
// With ?lat&lon it answers JSON — the page fetches that after asking the
// browser for a location. Without, it renders the page and the line status,
// which needs no location at all.
func Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if term := strings.TrimSpace(r.URL.Query().Get("q")); term != "" {
		if len(term) > 200 {
			app.RespondError(w, 400, "Search is too long")
			return
		}
		stops, err := searchStops(term)
		if err != nil {
			app.RespondError(w, 502, "Could not search transport stops")
			return
		}
		out := stopsJSON(stops)
		for _, v := range out {
			delete(v, "away")
		}
		app.RespondJSON(w, map[string]any{"stops": out})
		return
	}
	if q.Get("lat") != "" && q.Get("lon") != "" {
		lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
		lon, err2 := strconv.ParseFloat(q.Get("lon"), 64)
		if err1 != nil || err2 != nil {
			app.RespondError(w, http.StatusBadRequest, "Invalid coordinates")
			return
		}
		stops, err := nearbyStops(lat, lon, 400)
		if err != nil {
			app.RespondError(w, http.StatusBadGateway, err.Error())
			return
		}
		app.RespondJSON(w, map[string]any{"stops": stopsJSON(stops)})
		return
	}
	if id := strings.TrimSpace(q.Get("stop")); id != "" {
		arrs, err := arrivalsAt(id)
		if err != nil {
			app.RespondError(w, http.StatusBadGateway, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(arrs))
		for i, a := range arrs {
			if i >= 8 {
				break
			}
			dest := a.Destination
			if dest == "" {
				dest = a.Towards
			}
			out = append(out, map[string]any{"line": a.Line, "to": dest, "in": mins(a.Seconds)})
		}
		app.RespondJSON(w, map[string]any{"arrivals": out})
		return
	}

	app.Respond(w, r, app.Response{Title: "Transit", Description:  //nolint:errcheck
	"Stops near you, what is due, and which lines are down", HTML: page()})
}

func stopsJSON(stops []stop) []map[string]any {
	out := make([]map[string]any, 0, 8)
	for i, s := range stops {
		if i >= 8 {
			break
		}
		out = append(out, map[string]any{
			"id":    s.ID,
			"name":  s.Name,
			"modes": strings.Join(s.Modes, "/"),
			"away":  fmt.Sprintf("%.0fm", s.Distance),
		})
	}
	return out
}

// page renders the page: line status now, stops once the browser says where it
// is.
func page() string {
	var b strings.Builder
	b.WriteString(app.Column())
	b.WriteString(`<div class="card"><form id="xsearch" class="xsearch"><label for="xquery">Find a London stop or station</label><div class="xsearch-row"><input id="xquery" name="q" type="search" placeholder="Stop, station or area" required maxlength="200"><button type="submit" class="btn">Search</button><button type="button" id="xnear" class="btn">Use my location</button></div></form><div id="xstops" aria-live="polite" class="xmuted">Search for a stop or use your location.</div></div>`)

	b.WriteString(statusCard())
	b.WriteString(`</div>` + pageStyle + pageScript)
	return b.String()
}

// statusCard renders line status, or why it could not.
func statusCard() string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h3>Lines</h3>`)

	lines, err := statuses("")
	if err != nil {
		b.WriteString(`<p class="xmuted">` + html.EscapeString(err.Error()) + `</p></div>`)
		return b.String()
	}

	var bad, good int
	for _, l := range lines {
		if l.Disrupted {
			bad++
			b.WriteString(`<div class="xline xbad"><b>` + html.EscapeString(l.Name) + `</b> — ` +
				html.EscapeString(l.Status))
			if l.Reason != "" {
				b.WriteString(`<div class="xwhy">` + html.EscapeString(l.Reason) + `</div>`)
			}
			b.WriteString(`</div>`)
			continue
		}
		good++
	}
	if bad == 0 {
		b.WriteString(`<p class="xgood">Good service on all ` + strconv.Itoa(len(lines)) + ` lines.</p>`)
	} else if good > 0 {
		b.WriteString(`<p class="xmuted">Good service on the other ` + strconv.Itoa(good) + `.</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// Card renders the home-screen card: what is wrong, or that nothing is.
//
// Contentless about the viewer — it says nothing about where anybody is,
// because the home screen is rendered server-side and has no location to use
// even if it wanted one.
func Card() string {
	lines, err := statuses("")
	if err != nil || len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	shown := 0
	for _, l := range lines {
		if !l.Disrupted {
			continue
		}
		if shown >= 3 {
			break
		}
		b.WriteString(`<div class="xline xbad"><b>` + html.EscapeString(l.Name) + `</b> — ` +
			html.EscapeString(l.Status) + `</div>`)
		shown++
	}
	if shown == 0 {
		b.WriteString(`<p class="xgood">Good service on all ` + strconv.Itoa(len(lines)) + ` lines.</p>`)
	}
	b.WriteString(`<p class="xmore"><a href="/transit">Stops near you →</a></p>`)
	return b.String()
}

const pageStyle = `<style>
.xsearch label{display:block;margin-bottom:8px}
.xsearch-row{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:12px}
.xsearch-row input{flex:1;min-width:150px}
.xsearch-row button{white-space:nowrap}
.xmuted{color:#888;font-size:14px;margin:0}
.xgood{color:#0f7a52;font-size:15px;margin:0;font-weight:600}
.xline{padding:8px 0;border-bottom:1px solid var(--border-color,#eee);font-size:15px}
.xline:last-of-type{border-bottom:0}
.xwhy{color:#666;font-size:13px;margin-top:4px}
.xstop{padding:8px 0;border-bottom:1px solid var(--border-color,#eee);font-size:15px;cursor:pointer}
.xstop:last-child{border-bottom:0}
.xaway{color:#888;font-size:13px}
.xarr{margin:6px 0 0 12px;font-size:14px;color:#444}
.xmore{margin:12px 0 0;font-size:14px}
</style>`

// Stops are fetched only after the browser offers a location, and arrivals only
// when a stop is tapped — two requests nobody made are two requests TfL rate
// limits for nothing.
const pageScript = `<script>
(function(){
 function wire(){
  var form=document.getElementById('xsearch'),out=document.getElementById('xstops'),near=document.getElementById('xnear');
  if(!form||form.dataset.wired)return;form.dataset.wired='1';
  var request=0;
  function load(query,seq){
   fetch('/transit?'+query).then(function(r){if(!r.ok)throw Error();return r.json();}).then(function(d){
    if(seq!==request)return;
    out.replaceChildren();
    if(!d.stops||!d.stops.length){out.textContent='No matching stops found. Try another London stop or station.';return;}
    d.stops.forEach(function(s){
     var row=document.createElement('div');row.className='xstop';
     var button=document.createElement('button');button.type='button';button.className='btn';button.textContent=s.name;
     var meta=document.createElement('span');meta.className='xaway';meta.textContent=' '+s.modes+(s.away?' · '+s.away:'');
     row.append(button,meta);out.appendChild(row);
     button.addEventListener('click',function(){
      button.disabled=true;var box=document.createElement('div');box.className='xarr';box.textContent='Loading arrivals…';row.appendChild(box);
      fetch('/transit?stop='+encodeURIComponent(s.id)).then(function(r){if(!r.ok)throw Error();return r.json();}).then(function(a){
       box.textContent=a.arrivals&&a.arrivals.length?a.arrivals.map(function(x){return x.line+' to '+x.to+' — '+x['in'];}).join('\n'):'Nothing due.';box.style.whiteSpace='pre-line';
      }).catch(function(){box.textContent='Could not load arrivals. Try again.';button.disabled=false;});
     });
    });
   }).catch(function(){if(seq===request)out.textContent='Could not reach transport data. Please try again.';});
  }
  form.addEventListener('submit',function(e){e.preventDefault();var q=document.getElementById('xquery').value.trim();if(!q)return;out.textContent='Searching…';load('q='+encodeURIComponent(q),++request);});
  near.addEventListener('click',function(){
   var seq=++request;
   if(!navigator.geolocation){out.textContent='Location is unavailable. Search for a stop instead.';return;}
   out.textContent='Finding nearby stops…';
   navigator.geolocation.getCurrentPosition(function(pos){if(seq===request)load('lat='+pos.coords.latitude+'&lon='+pos.coords.longitude,seq);},function(){if(seq===request)out.textContent='Location was not available. Search for a stop instead.';},{timeout:10000,maximumAge:60000});
  });
 }
 wire();document.addEventListener('mu:navigated',wire);
})();</script>`
