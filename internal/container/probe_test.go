package container

// A missing runtime is asked about again; a present one is not.

import (
	"errors"
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

// A stopped daemon is named as one, whichever words the client uses for it.
//
// The CLI's phrasing is the CLI's, and it has already changed once under this
// code: the message whyNot was written against shares no matched substring with
// the one a current client prints. When it stopped matching, an operator with a
// stopped daemon got "the container runtime did not answer: |0|0" — the probe's
// own template rendering itself empty, which names nothing to start.
//
// Table-driven because the point is that there is more than one wording, and
// the next one goes in here.
func TestAStoppedDaemonIsNamedWhateverTheClientCallsIt(t *testing.T) {
	// Each as the probe sees it: our template on stdout, the complaint on
	// stderr, one buffer.
	for _, out := range []string{
		"|0|0\nCannot connect to the Docker daemon at unix:///var/run/docker.sock. " +
			"Is the docker daemon running?",
		"|0|0\nfailed to connect to the docker API at unix:///var/run/docker.sock; " +
			"check if the path is correct and if the daemon is running: dial unix " +
			"/var/run/docker.sock: connect: no such file or directory",
		"|0|0\nerror during connect: Get \"http://%2Fvar%2Frun%2Fdocker.sock/v1.51/info\": " +
			"dial unix /var/run/docker.sock: connect: connection refused",
	} {
		why := whyNot(out, errors.New("exit status 1"))
		if !containsAny(why, "daemon is not running") {
			t.Errorf("a stopped daemon was not reported as one:\n  said: %s\n  from: %s",
				why, trimLine(out))
		}
		if containsAny(why, "|0|0") {
			t.Errorf("the reason quotes the probe's own empty template at the operator: %s", why)
		}
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
