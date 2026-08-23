package weather

// When the light goes.
//
// Google returns sunriseTime and sunsetTime on every day of the forecast and
// this parsed past both of them. For anything outdoors it is the fact that
// decides the afternoon: "18°C and cloudy" does not tell you whether to set
// off, and "the light goes in forty minutes" does.

import (
	"strings"
	"testing"
	"time"
)

func day(sunrise, sunset string) DailyItem {
	d := DailyItem{Date: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)}
	if sunrise != "" {
		d.Sunrise, _ = time.Parse(time.RFC3339, sunrise)
	}
	if sunset != "" {
		d.Sunset, _ = time.Parse(time.RFC3339, sunset)
	}
	return d
}

func TestItSaysHowMuchLightIsLeft(t *testing.T) {
	d := day("2026-08-23T05:54:00Z", "2026-08-23T19:48:00Z")

	// Mid-afternoon: hours and minutes.
	got := lightToday(d, time.Date(2026, 8, 23, 17, 20, 0, 0, time.UTC))
	if !strings.Contains(got, "2h 28m of daylight left") {
		t.Errorf("mid-afternoon gave %q", got)
	}
	if !strings.Contains(got, "sunrise 05:54") || !strings.Contains(got, "sunset 19:48") {
		t.Errorf("the times are missing: %q", got)
	}

	// Close to sunset: minutes, which is the number somebody outdoors wants.
	got = lightToday(d, time.Date(2026, 8, 23, 19, 8, 0, 0, time.UTC))
	if !strings.Contains(got, "40 minutes of daylight left") {
		t.Errorf("near sunset gave %q", got)
	}
}

// After dark and before dawn there is nothing to count down to. "0 minutes of
// daylight left" at ten at night is a true sentence nobody needs.
func TestAfterDarkItOnlyGivesTheTimes(t *testing.T) {
	d := day("2026-08-23T05:54:00Z", "2026-08-23T19:48:00Z")

	for _, at := range []time.Time{
		time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC),
	} {
		got := lightToday(d, at)
		if strings.Contains(got, "daylight left") {
			t.Errorf("at %s it counted down: %q", at.Format("15:04"), got)
		}
		if !strings.Contains(got, "sunset 19:48") {
			t.Errorf("at %s it said nothing useful: %q", at.Format("15:04"), got)
		}
	}
}

// Above the Arctic circle the sun does neither for part of the year. That is a
// real place, not a parse failure, and the line simply is not there.
func TestWhereTheSunDoesNeitherItSaysNothing(t *testing.T) {
	if got := lightToday(day("", ""), time.Now().UTC()); got != "" {
		t.Errorf("a day with no sunrise or sunset produced %q", got)
	}
}

// And the forecast carries it, which is the half that was dropped: the field
// was in the response and never reached DailyItem.
func TestTheForecastCarriesTheSunTimes(t *testing.T) {
	d := day("2026-08-23T05:54:00Z", "2026-08-23T19:48:00Z")
	wf := &WeatherForecast{
		Location:   "London",
		Source:     "test",
		DailyItems: []DailyItem{d},
	}
	got := formatForecastText(wf, time.Date(2026, 8, 23, 17, 20, 0, 0, time.UTC))
	if !strings.Contains(got, "Light: sunrise 05:54") {
		t.Errorf("the forecast does not mention the light:\n%s", got)
	}
}
