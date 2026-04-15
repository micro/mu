package app

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
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
