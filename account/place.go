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
// Rounded to a 0.005-degree grid (roughly 500 metres), including direct API input.
// Device uncertainty may be greater than the rounding distance.

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

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
		acc.Lat = math.Round(lat*200) / 200
		acc.Lon = math.Round(lon*200) / 200
	}
	return auth.UpdateAccount(acc)
}

// LocalNow is the time where an account is.
//
// Their zone when they have set a place, and this machine's clock when they
// have not — which is right for a self-hosted instance, where the server and
// the person are usually in the same room, and is the only honest fallback for
// anybody else.
//
// Exported because more than the prompt wants it now: a page that greets
// somebody with "Morning" has to mean their morning, and a server in Virginia
// saying good morning to somebody in Tokyo at nine at night is worse than
// saying nothing.
func LocalNow(accountID string) time.Time {
	if accountID == "" {
		return time.Now()
	}
	acc, err := auth.GetAccount(accountID)
	if err != nil || acc == nil || acc.Zone == "" {
		return time.Now()
	}
	loc, err := time.LoadLocation(acc.Zone)
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

// PlaceOf is where an account is, as coordinates, and false when nobody has
// said.
//
// PlaceLine's fact without PlaceLine's sentence. That one is written for a
// model's prompt — a name, coordinates and a zone, joined with dashes — and a
// caller that wants to look something up by position had to parse it back out.
func PlaceOf(accountID string) (lat, lon float64, ok bool) {
	acc, err := auth.GetAccount(accountID)
	if err != nil || acc == nil {
		return 0, 0, false
	}
	if acc.Lat == 0 && acc.Lon == 0 {
		return 0, 0, false
	}
	return acc.Lat, acc.Lon, true
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
		parts = append(parts, fmt.Sprintf("%.3f,%.3f", acc.Lat, acc.Lon))
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

// PlaceHandler serves POST /account/place.
func PlaceHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/account/profile#place", http.StatusSeeOther)
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
	http.Redirect(w, r, "/account/profile#place", http.StatusSeeOther)
}
