package app

import (
	"context"
	"net/http"
)

type rendererKey struct{}

// Renderer supplies a request-scoped shell for a separately routed application.
type Renderer func(title, description, body string, r *http.Request) string

func WithRenderer(r *http.Request, renderer Renderer) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), rendererKey{}, renderer))
}
