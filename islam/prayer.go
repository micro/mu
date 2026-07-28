package islam

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Prayer times come from the Aladhan API: free, keyless, and widely used.
// Results are cached per location per day — times only change once a day, so a
// page view should not cost a round trip.
const prayerAPI = "https://api.aladhan.com/v1/timings"

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

// Ordered returns the five daily prayers in order, with sunrise included for
// context. Used for rendering and for finding the next prayer.
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

// Next returns the name and time of the next prayer after now (local to the
// requested location's clock, which is what the API returns), or the first
// prayer of the day when the last has passed.
func (p *PrayerTimes) Next(now time.Time) (string, string) {
	cur := now.Format("15:04")
	for _, e := range p.Ordered() {
		if e.Name == "Sunrise" {
			continue // not a prayer
		}
		if e.Time > cur {
			return e.Name, e.Time
		}
	}
	return "Fajr", p.Fajr // tomorrow
}

var (
	prayerMu    sync.RWMutex
	prayerCache = map[string]*PrayerTimes{}
)

func prayerKey(lat, lon float64, date string) string {
	// One decimal place (~11km) is plenty for prayer times and keeps the cache
	// from fragmenting on tiny GPS jitter.
	return fmt.Sprintf("%.1f:%.1f:%s", lat, lon, date)
}

// GetPrayerTimes returns today's prayer times for a location, caching per
// location per day.
func GetPrayerTimes(ctx context.Context, lat, lon float64) (*PrayerTimes, error) {
	date := time.Now().Format("02-01-2006") // Aladhan wants DD-MM-YYYY
	key := prayerKey(lat, lon, date)

	prayerMu.RLock()
	if pt, ok := prayerCache[key]; ok {
		prayerMu.RUnlock()
		return pt, nil
	}
	prayerMu.RUnlock()

	url := fmt.Sprintf("%s/%s?latitude=%f&longitude=%f&method=2", prayerAPI, date, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prayer times unavailable (%d)", resp.StatusCode)
	}

	var out struct {
		Data struct {
			Timings map[string]string `json:"timings"`
			Date    struct {
				Readable string `json:"readable"`
			} `json:"date"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	// The API appends timezone hints like "05:12 (BST)" — keep just the clock.
	clean := func(s string) string {
		if i := strings.Index(s, " "); i > 0 {
			return s[:i]
		}
		return s
	}
	t := out.Data.Timings
	pt := &PrayerTimes{
		Date:    out.Data.Date.Readable,
		Fajr:    clean(t["Fajr"]),
		Sunrise: clean(t["Sunrise"]),
		Dhuhr:   clean(t["Dhuhr"]),
		Asr:     clean(t["Asr"]),
		Maghrib: clean(t["Maghrib"]),
		Isha:    clean(t["Isha"]),
	}
	if pt.Fajr == "" {
		return nil, fmt.Errorf("prayer times unavailable")
	}

	prayerMu.Lock()
	// Bound the cache: it is keyed by location and date, so stale days would
	// otherwise accumulate on a busy instance.
	if len(prayerCache) > 500 {
		keys := make([]string, 0, len(prayerCache))
		for k := range prayerCache {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys[:len(keys)/2] {
			delete(prayerCache, k)
		}
	}
	prayerCache[key] = pt
	prayerMu.Unlock()

	return pt, nil
}
