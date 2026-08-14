// Package gtfs reads public transport timetables.
//
// GTFS is what a transit agency publishes when it publishes a timetable: a zip
// of CSV files describing stops, routes, and when a vehicle is meant to call
// where. Google and Portland's transit agency defined it in 2005 and almost
// everybody adopted it, which makes it the one place in this repository where
// the uniform interface already existed before we arrived — Berlin, BART and a
// bus company in Reading all publish the same eight files.
//
// That is why this exists. transit could answer "when is my bus" in London and
// nowhere else, because it spoke TfL's API. Speaking GTFS instead answers the
// same question in any city that publishes a feed, with the same tool, and the
// caller never learns which of the two replied.
//
// # Why none of this is unpacked
//
// Feeds are small zipped and enormous open. Berlin's is 75MB as published and
// 590MB unpacked, of which 382MB is stop_times.txt — one line per vehicle per
// stop per day, five and a half million of them. Unpacking that to disk to
// answer "the next three buses" would be absurd, so nothing is ever unpacked:
// the zip is streamed, and what comes out the other side is a fixed-width
// array of departures, eight bytes each, which is 46MB for Berlin and small
// enough to seek into rather than hold.
//
// Everything else — stops, trips, routes, the calendar — is small enough to
// keep in memory and is persisted beside the departures so a restart is a read
// rather than a rebuild.
package gtfs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Stop is a place a vehicle calls at.
type Stop struct {
	ID       string
	Name     string
	Lat, Lon float64
	// Code is what is written on the pole, where an agency publishes one. It is
	// what somebody standing at the stop can actually read off, which the
	// internal id usually is not.
	Code string
}

// Route is a named service — a bus number, a line.
type Route struct {
	ID    string
	Short string
	Long  string
	Type  int
}

// Trip is one run of a route on a given pattern of days.
type Trip struct {
	Route    uint32
	Service  uint32
	Headsign string
}

// Departure is one vehicle leaving one stop.
//
// Deliberately eight bytes and nothing more: this is the array with millions of
// entries in it, and every field added to it costs megabytes per city.
type Departure struct {
	// Seconds since midnight. GTFS allows times past 24:00:00 to express a
	// service that runs after midnight but belongs to the previous day's
	// timetable, so this legitimately exceeds 86400 and must not be wrapped.
	Time uint32
	Trip uint32
}

// Window is a span of the day over which a pattern repeats.
//
// "Every eight minutes between seven and ten" is one of these. A feed that uses
// them does not list the individual departures at all.
type Window struct {
	Start, End, Headway uint32
}

// nextFrom returns departure times at or after `from` for a stop that sits
// `offset` seconds into the pattern.
//
// At most `limit`, because a five-minute headway over a twelve-hour window is
// a hundred and forty departures and a caller asked for the next three.
func (w Window) nextFrom(from, offset uint32, limit int) []uint32 {
	if w.Headway == 0 || w.End <= w.Start {
		return nil
	}
	// The first departure of the pattern that has not already gone.
	var k uint32
	if start := w.Start + offset; from > start {
		k = (from - start + w.Headway - 1) / w.Headway
	}
	var out []uint32
	for ; len(out) < limit; k++ {
		t := w.Start + k*w.Headway
		if t >= w.End {
			break
		}
		out = append(out, t+offset)
	}
	return out
}

// Service is the set of days a trip runs on.
type Service struct {
	// Days is a bitmask, Monday is bit 0.
	Days       uint8
	Start, End int // yyyymmdd
	// Added and Removed are calendar_dates.txt: the bank holidays and the
	// engineering works. Without them a feed cheerfully reports a full weekday
	// service on Christmas Day.
	Added   map[int]bool
	Removed map[int]bool
}

// runsOn reports whether a service operates on a date.
func (s *Service) runsOn(date int, weekday time.Weekday) bool {
	if s.Removed[date] {
		return false
	}
	if s.Added[date] {
		return true
	}
	if date < s.Start || date > s.End {
		return false
	}
	// GTFS weeks start on Monday; Go's start on Sunday.
	bit := (int(weekday) + 6) % 7
	return s.Days&(1<<bit) != 0
}

// dateInt renders a time as the yyyymmdd integer GTFS uses.
func dateInt(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}

// parseDate reads a GTFS yyyymmdd field.
func parseDate(s string) (int, error) {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return 0, fmt.Errorf("not a date: %q", s)
	}
	return strconv.Atoi(s)
}

// parseTime reads a GTFS HH:MM:SS, which may go past 24 hours.
//
// A 25:10:00 departure is a real thing: it is the 1:10am bus that belongs to
// the previous day's service. Rejecting it, or wrapping it to 1:10, loses the
// night buses entirely.
func parseTime(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var h, m, sec int
	n, err := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec)
	if err != nil || n != 3 {
		return 0, false
	}
	if h < 0 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, false
	}
	return uint32(h*3600 + m*60 + sec), true
}
