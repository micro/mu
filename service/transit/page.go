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
	b.WriteString(`<div class="card"><h2>Transit</h2>`)
	b.WriteString(`<p class="xlede">Live London transport — stops near you, what is due, ` +
		`and which lines are in trouble. Free, and callable by an agent: see <a href="/tools">Tools</a>.</p></div>`)

	b.WriteString(`<div class="card xnear"><h3>Near you</h3>`)
	b.WriteString(`<p id="xstops" class="xmuted">Asking your browser where you are…</p></div>`)

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
.xlede{color:#666;font-size:15px;margin:0}
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
    var out = document.getElementById('xstops');
    if (!out || !navigator.geolocation) {
      if (out) out.textContent = 'Your browser will not share a location.';
      return;
    }
    navigator.geolocation.getCurrentPosition(function(pos){
      fetch('/transit?lat=' + pos.coords.latitude + '&lon=' + pos.coords.longitude)
        .then(function(r){ return r.json(); })
        .then(function(d){
          if (!d.stops || !d.stops.length) { out.textContent = 'No stops within 400m. London only.'; return; }
          out.innerHTML = '';
          d.stops.forEach(function(s){
            var el = document.createElement('div');
            el.className = 'xstop';
            el.innerHTML = '<b>' + s.name + '</b> <span class="xaway">' + s.modes + ' · ' + s.away + '</span>';
            el.addEventListener('click', function(){
              if (el.dataset.open) { return; }
              el.dataset.open = '1';
              fetch('/transit?stop=' + encodeURIComponent(s.id))
                .then(function(r){ return r.json(); })
                .then(function(a){
                  var box = document.createElement('div');
                  box.className = 'xarr';
                  box.textContent = (a.arrivals && a.arrivals.length)
                    ? a.arrivals.map(function(x){ return x.line + ' to ' + x.to + ' — ' + x['in']; }).join('\n')
                    : 'Nothing due.';
                  box.style.whiteSpace = 'pre-line';
                  el.appendChild(box);
                });
            });
            out.appendChild(el);
          });
        })
        .catch(function(){ out.textContent = 'Could not reach transport data.'; });
    }, function(){ out.textContent = 'No location, so no stops. Line status is below.'; });
  }
  wire();
  document.addEventListener('mu:navigated', wire);
})();
</script>`
