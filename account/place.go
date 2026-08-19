package account

// Where you are.
//
// The one fact that makes the difference between an agent and a search box, and
// it lived in a browser. The weather card asked for geolocation, resolved
// coordinates, and put them in localStorage — so the home screen could show
// today's forecast while the weather agent, asked "do I need a coat today",
// answered "which city are you in?". Same instance, same second, same account.
//
// Every specialist inherited it. Places cannot answer "near me", transit has no
// stop to start from, prayer cannot compute a time or a qibla without a
// latitude, flights have no origin, and news cannot say what is happening where
// you live. And nothing scheduled could ever work: a briefing that runs at 7am
// has no browser in the room to ask.
//
// So it belongs to the account, beside the language — which is the same kind of
// fact, and was already there.
//
// # Precision
//
// Rounded to two decimal places, always, on the way in. That is about a
// kilometre. A forecast, a prayer time, a train and what is nearby are all the
// same at that resolution, and it is not somebody's address — which matters
// because this is stored on a server, read by a model, and can end up quoted in
// an answer. There is no case here that wants more precision and several that
// are harmed by it.

import (
	"fmt"
	"html"
	"math"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// SetPlace records where an account is.
//
// Coordinates are rounded before storage and a place with neither a name nor a
// coordinate clears it, which is how somebody takes it back.
func SetPlace(accountID, place string, lat, lon float64, zone string) error {
	acc, err := auth.GetAccount(accountID)
	if err != nil {
		return err
	}
	place = strings.TrimSpace(place)
	if len(place) > 120 {
		place = place[:120]
	}
	acc.Place = place
	acc.Zone = strings.TrimSpace(zone)
	acc.Lat, acc.Lon = 0, 0
	if lat != 0 || lon != 0 {
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return fmt.Errorf("that is not a point on the earth")
		}
		acc.Lat = math.Round(lat*100) / 100
		acc.Lon = math.Round(lon*100) / 100
	}
	return auth.UpdateAccount(acc)
}

// PlaceLine is where an account is, as one line for a prompt, and empty when
// nobody has said.
//
// Coordinates as well as the name, because a name is what a person reads and a
// coordinate is what a tool takes — an agent given only "Lisbon" has to geocode
// it before it can ask for a forecast, which is a tool call and a chance to get
// it wrong.
func PlaceLine(accountID string) string {
	acc, err := auth.GetAccount(accountID)
	if err != nil || acc == nil {
		return ""
	}
	var parts []string
	if acc.Place != "" {
		parts = append(parts, acc.Place)
	}
	if acc.Lat != 0 || acc.Lon != 0 {
		parts = append(parts, fmt.Sprintf("%.2f,%.2f", acc.Lat, acc.Lon))
	}
	if len(parts) == 0 {
		return ""
	}
	line := strings.Join(parts, " — ")
	if acc.Zone != "" {
		line += " (" + acc.Zone + ")"
	}
	return line
}

// PlaceCard is the control on /account.
//
// Takes the request for the CSRF token: this form changes something, and
// StrictCSRF refuses a POST that leans on the session cookie alone.
func PlaceCard(r *http.Request, accountID string) string {
	acc, err := auth.GetAccount(accountID)
	if err != nil || acc == nil {
		return ""
	}
	coords := ""
	if acc.Lat != 0 || acc.Lon != 0 {
		coords = fmt.Sprintf("%.2f, %.2f", acc.Lat, acc.Lon)
	}

	var b strings.Builder
	b.WriteString(`<div class="card" id="place"><h4>Where you are</h4>`)
	b.WriteString(`<form action="/account/place" method="POST" class="place-form">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	b.WriteString(`<input class="ib-field" type="text" name="place" maxlength="120" placeholder="London" value="` +
		html.EscapeString(acc.Place) + `">`)
	b.WriteString(`<input type="hidden" name="lat" id="place-lat" value="` +
		strconv.FormatFloat(acc.Lat, 'f', -1, 64) + `">`)
	b.WriteString(`<input type="hidden" name="lon" id="place-lon" value="` +
		strconv.FormatFloat(acc.Lon, 'f', -1, 64) + `">`)
	b.WriteString(`<input type="hidden" name="zone" id="place-zone" value="` +
		html.EscapeString(acc.Zone) + `">`)
	b.WriteString(`<div class="ib-ask-row"><button type="submit">Save</button>` +
		`<button type="button" class="pill" onclick="muUseMyLocation(this)">Use my location</button>`)
	if coords != "" {
		b.WriteString(`<span class="place-at">` + html.EscapeString(coords) + `</span>`)
	}
	b.WriteString(`</div></form>`)
	b.WriteString(`<p class="text-sm text-muted" style="margin:10px 0 0">Your agents use this ` +
		`for the forecast, what is nearby, prayer times and the trains — including runs that ` +
		`happen while you are away from the screen. Coordinates are rounded to about a ` +
		`kilometre, never finer.</p>`)
	b.WriteString(placeJS)
	b.WriteString(`</div>`)
	return b.String()
}

// placeJS fills the coordinates from the browser, which is the only thing that
// knows them. The timezone comes free with it and is worth as much: an agent
// that knows the city and not the hour still cannot say whether it is today.
const placeJS = `<script>
function muUseMyLocation(btn){
  if(!navigator.geolocation){btn.textContent='Not available';return}
  btn.textContent='Locating…';
  navigator.geolocation.getCurrentPosition(function(p){
    document.getElementById('place-lat').value=p.coords.latitude.toFixed(2);
    document.getElementById('place-lon').value=p.coords.longitude.toFixed(2);
    try{document.getElementById('place-zone').value=Intl.DateTimeFormat().resolvedOptions().timeZone||''}catch(e){}
    btn.textContent='Got it — press Save';
  },function(){btn.textContent='Could not locate'},{timeout:8000});
}
</script>`

// PlaceHandler serves POST /account/place.
func PlaceHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/account#place", http.StatusSeeOther)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}
	lat, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("lat")), 64)
	lon, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("lon")), 64)
	if err := SetPlace(acc.ID, r.FormValue("place"), lat, lon, r.FormValue("zone")); err != nil {
		app.Log("account", "setting a place for %s: %v", acc.ID, err)
	}
	http.Redirect(w, r, "/account#place", http.StatusSeeOther)
}
