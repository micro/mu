package server

import (
	"fmt"
	"mu/internal/app"
	"net/http"
	"strings"
	"time"
)

// Record both boundaries so the last start identifies a component still loading.
func startupStep(name string, load func()) {
	start := time.Now()
	app.Log("startup", "component=%s state=starting", name)
	load()
	elapsed := time.Since(start)
	app.RecordStartup(name, elapsed)
	app.Log("startup", "component=%s state=ready duration_ms=%.3f", name, float64(elapsed)/float64(time.Millisecond))
}

// startingResponse is independent of service state and the mutable route
// registry. Requests never wait for startup and cannot mutate unloaded stores.
// Closing ready publishes the completed registry to request goroutines.
func startingResponse(w http.ResponseWriter, r *http.Request, ready <-chan struct{}, assets http.Handler) bool {
	select {
	case <-ready:
		return false
	default:
	}
	setSecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		(r.URL.Path == "/mu.css" || strings.HasSuffix(r.URL.Path, ".woff2")) {
		assets.ServeHTTP(w, r)
		return true
	}
	w.Header().Set("Retry-After", "1")
	if app.WantsJSON(r) || app.SendsJSON(r) || strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" ||
		(r.Method != http.MethodGet && r.Method != http.MethodHead) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if r.Method != http.MethodHead {
			fmt.Fprintln(w, `{"error":"Micro is starting. Please try again shortly.","retryable":true}`)
		}
		return true
	}
	// GETs can safely refresh. Never automatically resubmit a mutation.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Refresh", "1")
	w.WriteHeader(http.StatusServiceUnavailable)
	if r.Method != http.MethodHead {
		fmt.Fprint(w, startupHTML)
	}
	return true
}

// Shared stylesheet, no model calls, personal data, inline scripts or new UI
// framework. The original URL stays in place and resumes when startup completes.
const startupHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>Micro</title><link rel="stylesheet" href="/mu.css"></head><body><div class="page"><header><a class="brand" href="/">Micro</a></header><main><div class="conversation"><div class="prompt-panel"><div class="prompt-welcome" role="status" aria-live="polite"><h1>Micro</h1><p>Loading your saved information…</p><p class="text-muted">This page will continue automatically.</p></div></div></div></main></div></body></html>`
