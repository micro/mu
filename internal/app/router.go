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
