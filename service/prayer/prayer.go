package prayer

import (
	"fmt"
	"strings"
	"sync"
	"time"

	goprayer "github.com/hablullah/go-prayer"
)

// Prayer times are computed locally from the sun's position — no API call, so
// the page cannot hang on an upstream and a self-hosted instance works with no
// network at all.
//
// Fajr and Isha depend on a twilight angle, and the conventions disagree by up
// to an hour, so the convention is the caller's choice. The default is ISNA;
// midday and sunset prayers are solar and identical under every convention.

// Conventions offered to callers, keyed by the name used in the API and UI.
var conventions = map[string]func() *goprayer.TwilightConvention{
	"isna":    goprayer.ISNA,    // 15° / 15°
	"mwl":     goprayer.MWL,     // 18° / 17°
	"egypt":   goprayer.Egypt,   // 19.5° / 17.5°
	"karachi": goprayer.Karachi, // 18° / 18°
	"gulf":    goprayer.Gulf,    // 19.5° / 90 min after maghrib
	"diyanet": goprayer.Diyanet, // Turkey
	"muis":    goprayer.MUIS,    // Singapore
	"jakim":   goprayer.JAKIM,   // Malaysia
}

// DefaultConvention is used when the caller does not specify one.
const DefaultConvention = "isna"

// ConventionNames lists the selectable conventions with display labels, in the
// order they should appear.
func ConventionNames() []struct{ ID, Label string } {
	return []struct{ ID, Label string }{
		{"isna", "ISNA (15°)"},
		{"mwl", "Muslim World League (18°)"},
		{"egypt", "Egyptian Authority (19.5°)"},
		{"karachi", "Karachi (18°)"},
		{"gulf", "Gulf (19.5°)"},
		{"diyanet", "Diyanet (Turkey)"},
		{"muis", "MUIS (Singapore)"},
		{"jakim", "JAKIM (Malaysia)"},
	}
}

func convention(name string) func() *goprayer.TwilightConvention {
	if c, ok := conventions[strings.ToLower(strings.TrimSpace(name))]; ok {
		return c
	}
	return conventions[DefaultConvention]
}

// PrayerTimes is a day's prayer schedule for one location.
type PrayerTimes struct {
	Date    string `json:"date"`
	Fajr    string `json:"fajr"`
	Sunrise string `json:"sunrise"`
	Dhuhr   string `json:"dhuhr"`
	Asr     string `json:"asr"`
	Maghrib string `json:"maghrib"`
	Isha    string `json:"isha"`
}

// Ordered returns the five daily prayers in order, with sunrise for context.
func (p *PrayerTimes) Ordered() []struct{ Name, Time string } {
	return []struct{ Name, Time string }{
		{"Fajr", p.Fajr},
		{"Sunrise", p.Sunrise},
		{"Dhuhr", p.Dhuhr},
		{"Asr", p.Asr},
		{"Maghrib", p.Maghrib},
		{"Isha", p.Isha},
	}
}

// Next returns the name and time of the next prayer after now, or the first
// prayer of the day once the last has passed.
func (p *PrayerTimes) Next(now time.Time) (string, string) {
	cur := now.Format("15:04")
	for _, e := range p.Ordered() {
		if e.Name == "Sunrise" || e.Time == "" {
			continue // sunrise is not a prayer
		}
		if e.Time > cur {
			return e.Name, e.Time
		}
	}
	return "Fajr", p.Fajr // tomorrow
}

type prayerCacheEntry struct {
	times *PrayerTimes
	day   string
}

var (
	prayerMu    sync.RWMutex
	prayerCache = map[string]prayerCacheEntry{}
)

// GetPrayerTimes returns today's prayer times for a location. tz is an IANA
// timezone name (e.g. "Europe/London") supplied by the browser; an unknown or
// empty value falls back to UTC, which still gives correct instants, just
// labelled in UTC.
func GetPrayerTimes(lat, lon float64, tz, method string) (*PrayerTimes, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil || tz == "" {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	day := now.Format("2006-01-02")

	// One decimal place (~11km) is ample for prayer times and stops the cache
	// fragmenting on GPS jitter.
	key := fmt.Sprintf("%.1f:%.1f:%s:%s", lat, lon, loc.String(), strings.ToLower(method))

	prayerMu.RLock()
	if e, ok := prayerCache[key]; ok && e.day == day {
		prayerMu.RUnlock()
		return e.times, nil
	}
	prayerMu.RUnlock()

	schedules, err := goprayer.Calculate(goprayer.Config{
		Latitude:            lat,
		Longitude:           lon,
		Timezone:            loc,
		TwilightConvention:  convention(method)(),
		AsrConvention:       goprayer.Shafii,
		HighLatitudeAdapter: goprayer.AngleBased(),
	}, now.Year())
	if err != nil {
		return nil, fmt.Errorf("could not calculate prayer times: %w", err)
	}

	var today *goprayer.Schedule
	for i := range schedules {
		if schedules[i].Date == day {
			today = &schedules[i]
			break
		}
	}
	if today == nil {
		return nil, fmt.Errorf("no prayer schedule for %s", day)
	}

	hhmm := func(t time.Time) string {
		if t.IsZero() {
			return "" // can happen at extreme latitudes
		}
		return t.In(loc).Format("15:04")
	}
	pt := &PrayerTimes{
		Date:    now.Format("2 Jan 2006"),
		Fajr:    hhmm(today.Fajr),
		Sunrise: hhmm(today.Sunrise),
		Dhuhr:   hhmm(today.Zuhr),
		Asr:     hhmm(today.Asr),
		Maghrib: hhmm(today.Maghrib),
		Isha:    hhmm(today.Isha),
	}

	prayerMu.Lock()
	if len(prayerCache) > 500 {
		prayerCache = map[string]prayerCacheEntry{} // cheap to recompute; just reset
	}
	prayerCache[key] = prayerCacheEntry{times: pt, day: day}
	prayerMu.Unlock()

	return pt, nil
}
