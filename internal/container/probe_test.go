package container

// A missing runtime is asked about again; a present one is not.

import (
	"testing"
	"time"
)

// The asymmetry is the fix. This was one sync.Once, so whatever the first probe
// said stood for the life of the process — an instance that started before
// Docker was up said there was no Docker until somebody restarted it, which is
// not a thing an operator who has just installed Docker would ever guess.
func TestAMissingRuntimeIsProbedAgain(t *testing.T) {
	restore := func(a bool, u string, l time.Time) func() {
		return func() { available, unreachable, lastProbe = a, u, l }
	}
	t.Cleanup(restore(available, unreachable, lastProbe))

	// A failure, just now: not re-probed, so the sentinel survives.
	available, unreachable, lastProbe = false, "sentinel", time.Now()
	probe()
	if unreachable != "sentinel" {
		t.Error("a failure was re-probed immediately, so every page load forks docker")
	}

	// The same failure, long enough ago: probed again, so the sentinel goes.
	lastProbe = time.Now().Add(-retryEvery - time.Second)
	probe()
	if unreachable == "sentinel" {
		t.Error("a stale failure was not re-probed — installing Docker will never " +
			"be noticed without restarting the server")
	}
}

// A working runtime is asked about once. It costs a subprocess and the answer
// does not change under us in the direction that matters.
func TestAWorkingRuntimeIsNotReProbed(t *testing.T) {
	restore := func(a bool, u string, l time.Time) func() {
		return func() { available, unreachable, lastProbe = a, u, l }
	}
	t.Cleanup(restore(available, unreachable, lastProbe))

	available, unreachable = true, ""
	lastProbe = time.Now().Add(-time.Hour)
	probe()
	if !available {
		t.Error("a working runtime was re-probed and lost")
	}
}

// The message says what was actually tested.
//
// LookPath finding nothing means this process cannot see Docker, which is not
// the same as Docker being absent — a service started by systemd gets a short
// PATH with no /snap/bin in it. Telling an operator who can type `docker ps`
// that it is not installed sends them to argue with the wrong thing.
func TestTheNotFoundMessageNamesThePath(t *testing.T) {
	restore := func(a bool, u string, l time.Time) func() {
		return func() { available, unreachable, lastProbe = a, u, l }
	}
	t.Cleanup(restore(available, unreachable, lastProbe))

	available, unreachable, lastProbe = false, "", time.Time{}
	probe()
	if available {
		t.Skip("this machine has a container runtime, so there is no message to check")
	}
	if !containsAny(unreachable, "PATH", "docker") {
		t.Errorf("the reason names neither PATH nor docker: %q", unreachable)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) && index(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
