package apps

import (
	"strings"
	"testing"
)

// TestSeedIncludesHelloWorld verifies the built-in hello-world app seeds as a
// public raw-mode app that renders its greeting.
func TestSeedIncludesHelloWorld(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	seedApps()

	a := GetApp("hello-world")
	if a == nil {
		t.Fatal("hello-world was not seeded")
	}
	if !a.Public {
		t.Error("hello-world should be public")
	}
	if a.Mode != "raw" {
		t.Errorf("mode = %q, want raw", a.Mode)
	}
	if !strings.Contains(a.RenderHTML(), "Hello, World") {
		t.Error("RenderHTML() missing the greeting")
	}

	var found bool
	for _, p := range GetPublicApps() {
		if p.Slug == "hello-world" {
			found = true
			break
		}
	}
	if !found {
		t.Error("hello-world not listed in GetPublicApps()")
	}
}
