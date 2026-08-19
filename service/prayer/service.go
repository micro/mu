package prayer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for prayer times, qibla and the daily
// reflection. Its methods are exposed as RPC endpoints and, through the agent
// and gateways, as AI tools.
//
// The service is named for what it does rather than for the tradition it serves.
// Everything here is Islamic — the times are the five salah, the qibla is the
// bearing to the Kaaba, the reflection is a verse of the Quran — and the page and the
// tool descriptions say so. But a sidebar is a door, and "Prayer" is a word
// anyone can walk through; the specificity belongs inside, where it is
// information, not at the entrance, where it is a filter.
type Server struct{}

// ReflectionRequest takes no input.
type ReflectionRequest struct{}

// ReflectionResponse is today's reflection as model-ready text.
type ReflectionResponse struct {
	Reminder string `json:"reminder" description:"Today's Islamic reflection: a verse of the Quran with its reference, a saying of the Prophet, a name of Allah, and a short message"`
}

// Reflection returns today's Islamic reflection — a verse of the Quran with its
// surah reference, a saying of the Prophet, a name of Allah, and a short
// message.
//
// Labelled Verse, Saying, Name and Reflection, the same four words the page
// uses. The sources are named in the descriptions and on the page; the labels
// themselves are the plain English, so nothing has to be known before it can be
// read.
// @example {}
func (Server) Reflection(_ context.Context, _ *ReflectionRequest, rsp *ReflectionResponse) error {
	d := DailyReminderData()
	if d == nil {
		rsp.Reminder = "No reflection is available right now."
		return nil
	}
	var b strings.Builder
	if d.Verse != "" {
		b.WriteString("Verse: " + d.Verse + "\n")
	}
	if d.Hadith != "" {
		b.WriteString("\nSaying: " + d.Hadith + "\n")
	}
	if d.Name != "" {
		b.WriteString("\nName: " + d.Name + "\n")
	}
	if d.Message != "" {
		b.WriteString("\nReflection: " + d.Message)
	}
	rsp.Reminder = strings.TrimSpace(b.String())
	return nil
}

// TimesRequest asks for prayer times at a location.
type TimesRequest struct {
	Lat    float64 `json:"lat" required:"true" description:"Latitude of the location"`
	Lon    float64 `json:"lon" required:"true" description:"Longitude of the location"`
	TZ     string  `json:"tz" description:"IANA timezone of the location, e.g. Europe/London (defaults to UTC)"`
	Method string  `json:"method" description:"Calculation convention: isna, mwl, egypt, karachi, gulf, diyanet, muis or jakim (defaults to isna)"`
}

// TimesResponse is the day's prayer schedule as model-ready text.
type TimesResponse struct {
	Times string `json:"times" description:"Today's prayer times, and which prayer is next"`
}

// Times returns today's prayer times for a location (Fajr, Dhuhr, Asr, Maghrib
// and Isha, plus sunrise), and which prayer comes next.
//
// Named Times rather than Prayer because the tool name is derived as
// service_method, and prayer.Prayer would produce prayer_prayer — the same trap
// search.Search fell into. See TestNoMethodRepeatsItsService.
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Times(_ context.Context, req *TimesRequest, rsp *TimesResponse) error {
	pt, err := GetPrayerTimes(req.Lat, req.Lon, req.TZ, req.Method)
	if err != nil {
		return err
	}
	var b strings.Builder
	if pt.Date != "" {
		b.WriteString(pt.Date + "\n")
	}
	for _, e := range pt.Ordered() {
		if e.Time == "" {
			continue
		}
		b.WriteString(e.Name + ": " + e.Time + "\n")
	}
	next, at := pt.Next(time.Now())
	b.WriteString("\nNext prayer: " + next + " at " + at)
	rsp.Times = strings.TrimSpace(b.String())
	return nil
}

// QiblaRequest asks for the qibla direction at a location.
type QiblaRequest struct {
	Lat float64 `json:"lat" required:"true" description:"Latitude of the location"`
	Lon float64 `json:"lon" required:"true" description:"Longitude of the location"`
}

// QiblaResponse is the direction to face for prayer.
type QiblaResponse struct {
	Direction string `json:"direction" description:"Compass bearing to the Kaaba, and the distance to Mecca"`
}

// Qibla returns the direction to face for prayer — the compass bearing from
// true north to the Kaaba in Mecca — and how far away Mecca is.
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Qibla(_ context.Context, req *QiblaRequest, rsp *QiblaResponse) error {
	b := QiblaBearing(req.Lat, req.Lon)
	rsp.Direction = fmt.Sprintf("Qibla: %.1f° (%s) from true north. Mecca is %.0f km away.",
		b, CompassPoint(b), DistanceToMeccaKm(req.Lat, req.Lon))
	return nil
}

var Spec = service.Spec{
	Name:        "prayer",
	Handler:     new(Server),
	Description: "Islamic prayer times, qibla and a daily reflection",
	Page:        "/prayer",
	Card:        service.Personal(ReminderHTML),
	Endpoints: map[string]service.Endpoint{
		"Times":      {Aliases: []string{"islam_prayer"}, Doc: "Get today's Islamic prayer times (salah) for a location, and which prayer is next"},
		"Qibla":      {Aliases: []string{"islam_qibla"}, Doc: "Get the qibla — the compass bearing to face for Islamic prayer from a location"},
		"Reflection": {Aliases: []string{"prayer_today", "islam_today", "islam", "reminder"}, Doc: "Get today's Islamic reflection — a verse of the Quran with its surah, a saying of the Prophet, and a name of Allah"},

		// Looking something up in the sources, rather than being told today's.
		// Implemented in scripture.go.
		"Verse":  {Aliases: []string{"quran"}, Doc: "Look up a chapter of the Quran, or one verse within it, by number. Use prayer_search to ask a question instead of naming a reference"},
		"Saying": {Aliases: []string{"hadith"}, Doc: "Look up a hadith from Sahih al-Bukhari, optionally from a given book"},
		"Search": {Aliases: []string{"quran_search"}, Doc: "Search the Quran, the hadith and the names of Allah by meaning — ask a question in plain language rather than naming a reference", Cost: quota.OpQuranSearch},
	},
}
