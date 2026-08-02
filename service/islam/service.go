package islam

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/service"
)

// Server is the go-micro service handler for the daily Islamic reminder. Its
// methods are exposed as RPC endpoints and, through the agent and gateways, as
// AI tools.
type Server struct{}

// TodayRequest takes no input.
type TodayRequest struct{}

// TodayResponse is today's reminder as model-ready text.
type TodayResponse struct {
	Reminder string `json:"reminder" description:"Today's Islamic reminder: Quran verse with reference, a hadith, and a short reflection"`
}

// Today returns today's Islamic reminder — a Quran verse with its surah
// reference, a hadith, and a short reflection.
// @example {}
func (Server) Today(_ context.Context, _ *TodayRequest, rsp *TodayResponse) error {
	d := GetDailyReminderData()
	if d == nil {
		rsp.Reminder = "No reminder is available right now."
		return nil
	}
	var b strings.Builder
	if d.Name != "" {
		b.WriteString(d.Name + "\n")
	}
	if d.Verse != "" {
		b.WriteString(d.Verse + "\n")
	}
	if d.Hadith != "" {
		b.WriteString("\nHadith: " + d.Hadith + "\n")
	}
	if d.Message != "" {
		b.WriteString("\nReflection: " + d.Message)
	}
	rsp.Reminder = strings.TrimSpace(b.String())
	return nil
}

// PrayerRequest asks for prayer times at a location.
type PrayerRequest struct {
	Lat    float64 `json:"lat" description:"Latitude of the location"`
	Lon    float64 `json:"lon" description:"Longitude of the location"`
	TZ     string  `json:"tz" description:"IANA timezone of the location, e.g. Europe/London (defaults to UTC)"`
	Method string  `json:"method" description:"Calculation convention: isna, mwl, egypt, karachi, gulf, diyanet, muis or jakim (defaults to isna)"`
}

// PrayerResponse is the day's prayer schedule as model-ready text.
type PrayerResponse struct {
	Times string `json:"times" description:"Today's prayer times, and which prayer is next"`
}

// Prayer returns today's prayer times for a location (Fajr, Dhuhr, Asr,
// Maghrib and Isha, plus sunrise), and which prayer comes next.
// @example {"lat": 51.5074, "lon": -0.1278}
func (Server) Prayer(_ context.Context, req *PrayerRequest, rsp *PrayerResponse) error {
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
	Lat float64 `json:"lat" description:"Latitude of the location"`
	Lon float64 `json:"lon" description:"Longitude of the location"`
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

var toolDocs = service.Docs{
	"Today":  "Get today's Islamic reminder — a Quran verse with its surah and translation",
	"Prayer": "Get today's prayer times for a location, and which prayer is next",
	"Qibla":  "Get the compass bearing to face for prayer from a location",
}
