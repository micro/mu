package server

import (
	"mu/internal/app"
	"time"
)

// Record both boundaries so the last start identifies a component still loading.
func startupStep(name string, load func()) {
	start := time.Now()
	app.Log("startup", "component=%s state=starting", name)
	load()
	app.Log("startup", "component=%s state=ready duration_ms=%.3f", name, float64(time.Since(start))/float64(time.Millisecond))
}
