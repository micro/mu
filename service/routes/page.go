package routes

// The page shows a route after an explicit, CSRF-protected POST.
// GET only displays or prefills the form and never calls the paid provider.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

// Handler serves /routes.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		app.MethodNotAllowed(w, r)
		return
	}
	if app.WantsJSON(r) && r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		app.MethodNotAllowed(w, r)
		return
	}
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			app.RespondError(w, 400, "Invalid directions request")
			return
		}
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		from = strings.TrimSpace(r.PostFormValue("from"))
		to = strings.TrimSpace(r.PostFormValue("to"))
		mode = strings.TrimSpace(r.PostFormValue("mode"))
	}
	if app.WantsJSON(r) {
		owner, ok := app.BillableCaller(w, r, quota.OpRoutesDirections)
		if !ok {
			return
		}
		if owner != "" {
			if err := auth.CheckPostRate(owner); err != nil {
				app.RespondError(w, 429, "Please wait before another lookup.")
				return
			}
		}

		j, msg := plan(&ETARequest{From: from, To: to, Mode: mode})
		if msg != "" {
			app.RespondError(w, 400, msg)
			return
		}
		route, err := paidRoute(owner, j)
		if err != nil {
			app.RespondError(w, 502, "Could not load directions")
			return
		}

		app.RespondJSON(w, map[string]any{"summary": j.fromLabel + " → " + j.toLabel + ": " + humanDuration(route.Duration) + ", " + humanDistance(route.Metres), "estimate": route.Estimate, "shape": route.Shape, "steps": route.Steps})
		return
	}
	var b strings.Builder
	b.WriteString(form(from, to, mode, auth.CSRFToken(r)))

	if r.Method == http.MethodPost && from != "" && to != "" {
		owner, ok := app.BillableCaller(w, r, quota.OpRoutesDirections)
		if !ok {
			return
		}
		if owner != "" {
			if err := auth.CheckPostRate(owner); err != nil {
				app.RespondError(w, 429, "Please wait before another lookup.")
				return
			}
		}
		b.WriteString(journeyCard(from, to, mode, owner))
	} else {
		b.WriteString(`<div class="card"><p class="text-sm text-muted">` +
			`Two places, and how you are travelling. You get the time, how much of it ` +
			`is traffic, the turns, and the route on a map — which ` +
			`helps you check that it goes where you expected. Include the city for ambiguous place names.</p></div>`)
	}
	app.Respond(w, r, app.Response{Title: "Routes", Description: "How to get from one place to another", HTML: b.String()})
}

// form is the two ends and the mode.
func form(from, to, mode string, csrf ...string) string {
	var b strings.Builder
	b.WriteString(`<form method="POST" action="/routes" class="card form form-inline">`)
	if len(csrf) > 0 {
		b.WriteString(app.CSRFField(csrf[0]))
	}
	b.WriteString(`<input name="from" value="` + html.EscapeString(from) +
		`" placeholder="From — e.g. King's Cross, London" autocomplete="off" aria-label="Starting point">`)
	b.WriteString(`<input name="to" value="` + html.EscapeString(to) +
		`" placeholder="To — e.g. British Museum" autocomplete="off" aria-label="Destination">`)
	b.WriteString(`<select name="mode" aria-label="How to travel">`)
	for _, m := range []struct{ value, label string }{
		{"drive", "Drive"}, {"walk", "Walk"}, {"cycle", "Cycle"}, {"transit", "Transit"},
	} {
		sel := ""
		if m.value == mode || (mode == "" && m.value == "drive") {
			sel = " selected"
		}
		b.WriteString(`<option value="` + m.value + `"` + sel + `>` + m.label + `</option>`)
	}
	b.WriteString(`</select><button type="submit">Go</button></form>`)
	return b.String()
}

// journeyCard is the answer: the summary, the drawing, and the turns.
func journeyCard(from, to, mode string, owners ...string) string {
	j, msg := plan(&ETARequest{From: from, To: to, Mode: mode})
	if msg != "" {
		return `<div class="card"><p class="text-sm text-muted">` + html.EscapeString(msg) + `</p></div>`
	}
	owner := ""
	if len(owners) > 0 {
		owner = owners[0]
	}
	r, err := paidRoute(owner, j)
	if err != nil {
		return `<div class="card"><p class="text-sm text-muted">` +
			html.EscapeString(err.Error()) + `.</p></div>`
	}

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	fmt.Fprintf(&b, `<h3 class="rt-head">%s → %s</h3>`, html.EscapeString(j.fromLabel), html.EscapeString(j.toLabel))
	fmt.Fprintf(&b, `<p class="rt-summary"><strong>%s</strong> · %s · by %s`,
		html.EscapeString(humanDuration(r.Duration)), html.EscapeString(humanDistance(r.Metres)),
		html.EscapeString(spoken(j.mode)))
	if d := r.Delay(); d > 0 {
		fmt.Fprintf(&b, ` · <span class="rt-traffic">%s of it traffic</span>`, html.EscapeString(humanDuration(d)))
	}
	b.WriteString(`</p>`)

	if r.Estimate {
		b.WriteString(`<p class="text-sm text-muted">Estimated from straight-line distance — ` +
			`live routing is unavailable on this instance, so treat it as approximate.</p>`)
	}
	if svg := draw(r.Shape); svg != "" {
		b.WriteString(svg)
	}
	if len(r.Steps) > 0 {
		b.WriteString(`<ol class="rt-steps">`)
		for _, s := range r.Steps {
			b.WriteString(`<li>` + html.EscapeString(s.Text))
			if s.Metres > 0 {
				b.WriteString(` <span class="rt-dist">` + html.EscapeString(humanDistance(s.Metres)) + `</span>`)
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ol>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// drawing bounds, in the SVG's own units.
const (
	drawW   = 640
	drawH   = 320
	drawPad = 18
)

// draw is the route as an SVG path.
//
// Latitude and longitude are projected flat, with longitude squeezed by the
// cosine of the latitude — at 51 degrees north a degree of longitude is barely
// six tenths of a degree of latitude, and without the correction every route in
// Britain comes out stretched sideways. Over the length of a journey somebody
// might drive, flat is indistinguishable from correct.
func draw(shape []point) string { return app.RouteMap(shape) }

// Estimates do not reach a paid provider and refund the reservation.
func paidRoute(owner string, j journey) (result route, err error) {
	settle, err := quota.Reserve(owner, quota.OpRoutesDirections)
	if err != nil {
		return result, err
	}
	completed := false
	defer func() {
		if e := settle(completed); e != nil {
			err = e
		}
	}()
	result, err = computeRoute(j.fromLat, j.fromLon, j.toLat, j.toLon, j.mode, j.when, full, j.fromAddress, j.toAddress)
	completed = err == nil && !result.Estimate
	return result, err
}
