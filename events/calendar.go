package events

import (
	"net/url"
	"time"
)

// defaultDuration is how long a scheduled event is assumed to last when we
// present it to a calendar (reminders have no natural end).
const defaultDuration = 30 * time.Minute

// GoogleCalendarURL builds an "add to Google Calendar" link for an event. It
// works for anyone with a Google account with no API, OAuth, or stored token —
// clicking it opens Google Calendar pre-filled to save the event.
func GoogleCalendarURL(title string, when time.Time, note string) string {
	start := when.UTC().Format("20060102T150405Z")
	end := when.UTC().Add(defaultDuration).Format("20060102T150405Z")
	q := url.Values{}
	q.Set("action", "TEMPLATE")
	q.Set("text", title)
	q.Set("dates", start+"/"+end)
	if note != "" {
		q.Set("details", note)
	}
	return "https://calendar.google.com/calendar/render?" + q.Encode()
}
