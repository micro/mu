package prayer

import (
	"strings"
	"testing"
)

// The home card answers the time-sensitive question in its corner — "Asr
// 14:25" — while the verse stays the body. Before this you had to open the
// page and scroll to find out when the next prayer was.
func TestHomeCardCarriesTheNextPrayer(t *testing.T) {
	html := ReminderHTML()
	if !strings.Contains(html, `id="prayer-next"`) {
		t.Error("home card has no slot for the next prayer")
	}
	if !strings.Contains(html, "card-corner") {
		t.Error("the mark is not using the corner style")
	}
}

// The home screen must not prompt for location. The mark fills itself in only
// from coordinates already granted to the weather or prayer cards, and stays
// empty otherwise — .card-corner:empty is display:none.
func TestHomeCardNeverAsksForLocation(t *testing.T) {
	html := ReminderHTML()
	if strings.Contains(html, "geolocation") {
		t.Error("the home card asks for geolocation; it must only reuse cached coordinates")
	}
	if !strings.Contains(html, "mu_weather_lat") {
		t.Error("the mark is not reading the shared cached coordinates")
	}
}
