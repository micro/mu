package app

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/internal/origin"
)

// ClientIP returns the originating client IP for a request, honouring
// X-Forwarded-For (first hop) and X-Real-IP when present, falling back
// to RemoteAddr. The returned value is the IP only (no port).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			xff = xff[:i]
		}
		ip := strings.TrimSpace(xff)
		if ip != "" {
			return ip
		}
	}
	if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// BaseURL is the public origin of this instance. See internal/origin.
func BaseURL(r *http.Request) string { return origin.URL(r) }

func TimeAgo(d time.Time) string {
	// Handle zero time
	if d.IsZero() {
		return "just now"
	}

	timeAgo := ""
	startDate := time.Now().Unix()
	deltaMinutes := float64(startDate-d.Unix()) / 60.0
	if deltaMinutes <= 523440 { // less than 363 days
		timeAgo = fmt.Sprintf("%s ago", distanceOfTime(deltaMinutes))
	} else {
		timeAgo = d.Format("2 Jan")
	}

	return timeAgo
}

func distanceOfTime(minutes float64) string {
	switch {
	case minutes < 1:
		secs := int(minutes * 60)
		if secs < 1 {
			secs = 1
		}
		if secs == 1 {
			return "1 sec"
		}
		return fmt.Sprintf("%d secs", secs)
	case minutes < 2:
		return "1 minute"
	case minutes < 60:
		return fmt.Sprintf("%d minutes", int(minutes))
	case minutes < 1440:
		hrs := int(minutes / 60)
		if hrs == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hrs)
	case minutes < 2880:
		return "1 day"
	case minutes < 43800:
		return fmt.Sprintf("%d days", int(minutes/1440))
	case minutes < 87600:
		return "1 month"
	default:
		return fmt.Sprintf("%d months", int(minutes/43800))
	}
}

// Bytes is a size somebody has to read at a glance.
//
// Three significant figures at most and no decimal below a megabyte: the
// question a size answers on a page is "is this big", and 1.4MB answers it
// where 1468006 does not.
func Bytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

// Count is a quantity somebody has to read at a glance.
//
// The same job Bytes does, for things rather than for bytes: on a page the
// question a count answers is "roughly how much", and 612k answers it where
// 611,847 makes the reader do arithmetic to find out it is about six hundred
// thousand. Exact below a thousand, because there the exact number is the
// readable one.
func Count(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fm", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

// ClientName is what a client is called in front of somebody.
//
// One name, in one place. There were two of these — one on the conversation
// view and one on /recall — and they had already drifted: the same conversation
// was labelled "Web" on one page and "Here" on the other. A map of display
// names is exactly the kind of thing that gets copied rather than imported.
//
// Here rather than beside the record, because it is presentation: the same
// shape as TimeAgo above it, a stored value turned into words for a reader. The
// record owns the value — thread.WebClient is a constant it stores and looks up
// by — and this owns how it reads. It takes a plain string for that reason, and
// knows nothing about threads.
//
// A client not named here shows as it names itself, so a new one appears the
// day it is written rather than the day somebody remembers this switch.
func ClientName(client string) string {
	switch client {
	case "web":
		return "Web"
	case "mail":
		return "Email"
	// The chat clients are deleted; these stay so a conversation recorded
	// before that still says where it happened rather than showing a raw value.
	case "discord":
		return "Discord"
	case "telegram":
		return "Telegram"
	case "whatsapp":
		return "WhatsApp"
	case "sms":
		return "SMS"
	case "cli":
		return "CLI"
	}
	return client
}
