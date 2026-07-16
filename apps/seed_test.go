package apps

import (
	"strings"
	"testing"
)

// TestSeedIncludesHelloWorld verifies the built-in hello-world app seeds as a
// public raw-mode app that renders its greeting.
func TestSeedIncludesHelloWorld(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ensureBuiltins()

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

// TestEnsureBuiltinsFillsGapsWithoutClobber verifies built-ins are added when
// missing but a user's app on the same slug is left untouched.
func TestEnsureBuiltinsFillsGapsWithoutClobber(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	saved := apps
	apps = map[string]*App{"notes": {Slug: "notes", Name: "Mine", AuthorID: "alice"}}
	mutex.Unlock()
	defer func() { mutex.Lock(); apps = saved; mutex.Unlock() }()

	ensureBuiltins()

	if a := GetApp("notes"); a == nil || a.AuthorID != "alice" {
		t.Fatalf("ensureBuiltins clobbered the user's notes app: %+v", a)
	}
	if GetApp("hello-world") == nil {
		t.Error("a missing built-in was not added")
	}
}
