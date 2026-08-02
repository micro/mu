package app

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"mu/internal/settings"
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

// BaseURL returns the public origin of this instance — "https://micro.mu" —
// for anything a caller outside the process will read back: an x402 resource
// identifier, a Stripe return URL, a link in an email.
//
// r.Host cannot be used on its own. Mu runs behind a reverse proxy that
// forwards to a loopback port, so r.Host is "localhost:8081" and any URL built
// from it names an address no client can reach. That is how the live x402
// challenge came to advertise https://localhost:8081/mcp as the resource being
// paid for.
//
// Order: MU_DOMAIN when configured, then the proxy's X-Forwarded-Host, then
// r.Host. Same shape as ClientIP, and the proxy that sets X-Forwarded-For sets
// X-Forwarded-Host too.
//
// X-Forwarded-Host is only trustworthy because the proxy sets it; a directly
// exposed instance would be taking it from the client. That is the same
// assumption ClientIP already makes.
func BaseURL(r *http.Request) string {
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" && d != "localhost" {
		return "https://" + strings.TrimSuffix(trimScheme(d), "/")
	}
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		if i := strings.Index(h, ","); i > 0 {
			h = strings.TrimSpace(h[:i])
		}
		return scheme(r) + "://" + trimScheme(h)
	}
	return scheme(r) + "://" + r.Host
}

func scheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

func trimScheme(s string) string {
	return strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
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
