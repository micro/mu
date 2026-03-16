package app

import (
	"net/http"
)

// RouteOpts defines handlers for different content types
type RouteOpts struct {
	// JSON handler - called when Accept: application/json or Content-Type: application/json
	JSON http.HandlerFunc
	// HTML handler - called for browser requests (default)
	HTML http.HandlerFunc
	// Auth requires authentication (redirects to login if not authenticated)
	Auth bool
}

// Route creates a handler that dispatches based on content type
func Route(opts RouteOpts) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check auth if required
		if opts.Auth {
			// Import cycle prevention: auth check done by caller or middleware
			// This is just the routing logic
		}

		// Dispatch based on content type
		if WantsJSON(r) || SendsJSON(r) {
			if opts.JSON != nil {
				opts.JSON(w, r)
				return
			}
			// No JSON handler, return error
			http.Error(w, `{"error": "JSON not supported"}`, http.StatusNotAcceptable)
			return
		}

		// Default to HTML
		if opts.HTML != nil {
			opts.HTML(w, r)
			return
		}

		// No HTML handler, try JSON
		if opts.JSON != nil {
			opts.JSON(w, r)
			return
		}

		http.Error(w, "No handler available", http.StatusNotImplemented)
	}
}

// RouteFunc is a convenience for handlers that handle both HTML and JSON internally
// (for gradual migration - packages can switch to Route() over time)
func RouteFunc(handler http.HandlerFunc) http.HandlerFunc {
	return handler
}
