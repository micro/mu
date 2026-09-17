package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mu/internal/abuse"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

// rateGate runs before page parsing and credential checks. A short in-memory
// admission queue also bounds the number of requests waiting on durable storage.
var rateQueue = make(chan struct{}, 64)

func rateGate(w http.ResponseWriter, r *http.Request, static []string) bool {
	// Ending a session must remain possible when a shared IP has exhausted its
	// allowance. This skips admission only; the normal request/auth gates and
	// logout handler still run. Never exempt login or other account operations.
	if (r.URL.Path == "/logout" || r.URL.Path == "/logout/") && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		return true
	}
	// Only known asset paths bypass protection, never an arbitrary .css suffix.
	if r.Method == "GET" || r.Method == "HEAD" {
		switch r.URL.Path {
		case "/mu.css", "/mu.js", "/manifest.webmanifest", "/favicon.ico", "/robots.txt":
			return true
		}
		if strings.HasPrefix(r.URL.Path, "/static/") {
			for _, ext := range static {
				if strings.HasSuffix(r.URL.Path, ext) {
					return true
				}
			}
		}
	}
	recordRefusal := func() { usage.Record("http-refused", usage.Endpoint(r.URL.Path), "unattributed") }
	select {
	case rateQueue <- struct{}{}:
		defer func() { <-rateQueue }()
	default:
		recordRefusal()
		w.Header().Set("Retry-After", "1")
		http.Error(w, "Server busy; retry shortly", 503)
		return false
	}
	ip := app.ClientIP(r)
	if parsed := net.ParseIP(ip); parsed != nil {
		if parsed.To4() == nil {
			ip = parsed.Mask(net.CIDRMask(64, 128)).String()
		} else {
			ip = parsed.String()
		}
	}
	take := func(key string, max int, window time.Duration) bool {
		wait, err := abuse.Take(key, max, window)
		if err != nil {
			recordRefusal()
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Request protection unavailable; retry shortly", 503)
			return false
		}
		if wait > 0 {
			recordRefusal()
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			http.Error(w, "Too many requests; try again later", 429)
			return false
		}
		return true
	}
	// All callers get a cheap outer IP bound before token validation. Merely
	// supplying a bogus cookie or Authorization header never bypasses a limit.
	if !take("http:ip:"+ip, abuse.Limit("HTTP_MAX_PER_MINUTE", 300), time.Minute) {
		return false
	}
	path := strings.TrimRight(r.URL.Path, "/")
	sensitive := path == "/login" || path == "/signup" || path == "/request-invite" || path == "/setup" || strings.HasPrefix(path, "/passkey/") || strings.HasPrefix(path, "/oauth/") || strings.HasPrefix(path, "/oauth2/")
	if sensitive && r.Method != "GET" && r.Method != "HEAD" {
		if !take("auth:ip:"+ip, abuse.Limit("AUTH_MAX_PER_15_MINUTES", 20), 15*time.Minute) {
			return false
		}
	}
	if _, acc := auth.TrySession(r); acc == nil {
		if !take("guest:minute:"+ip, abuse.Limit("HTTP_GUEST_MAX_PER_MINUTE", 60), time.Minute) {
			return false
		}
		if !take("guest:hour:"+ip, abuse.Limit("HTTP_GUEST_MAX_PER_HOUR", 300), time.Hour) {
			return false
		}
	}
	return true
}
