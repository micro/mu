package events

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// defaultDuration is how long a scheduled event is assumed to last when we
// present it to a calendar (reminders have no natural end).
const defaultDuration = 30 * time.Minute

// icsEscape escapes text for an iCalendar value (RFC 5545): backslash, comma,
// semicolon and newlines are the special characters.
func icsEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, "\r\n", `\n`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// ICS returns an iCalendar (.ics) document for an event, method PUBLISH so a
// mail client (Gmail/Google Calendar, Apple, Outlook) offers to add it. The
// event is attributed to organizerEmail — the platform user — so it lands as
// their own event. organizerEmail may be empty.
func ICS(e *Event, organizerEmail string) string {
	stamp := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Mu//Events//EN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:%s@mu\r\n", e.ID)
	fmt.Fprintf(&b, "DTSTAMP:%s\r\n", stamp(time.Now()))
	fmt.Fprintf(&b, "DTSTART:%s\r\n", stamp(e.When))
	fmt.Fprintf(&b, "DTEND:%s\r\n", stamp(e.When.Add(defaultDuration)))
	fmt.Fprintf(&b, "SUMMARY:%s\r\n", icsEscape(e.Title))
	if e.Note != "" {
		fmt.Fprintf(&b, "DESCRIPTION:%s\r\n", icsEscape(e.Note))
	}
	if organizerEmail != "" {
		fmt.Fprintf(&b, "ORGANIZER:mailto:%s\r\n", organizerEmail)
	}
	// A calendar reminder 10 minutes before.
	b.WriteString("BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Reminder\r\nTRIGGER:-PT10M\r\nEND:VALARM\r\n")
	b.WriteString("END:VEVENT\r\n")
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

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
