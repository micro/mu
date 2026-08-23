package container

// Whether there is a runtime here, and why not when there is not.

import (
	"strings"
	"testing"
)

// A probe that only checks the exit code is not a probe.
//
// This is the bug that reached production. `docker info` fails when no daemon
// answers — on some versions by exiting non-zero, and on others by printing the
// error and exiting 0. The check was `.Run() == nil`, the deployed box had one
// of the second kind, so every method sailed through the gate and an agent got
// a raw socket path back instead of an answer.
//
// The daemon's own version is the fact a client cannot produce alone: it
// renders empty unless something answered.
func TestAProbeThatExitedZeroIsNotEnough(t *testing.T) {
	// What the CLI prints when it cannot reach the socket but exits 0 anyway.
	// No line here carries three fields with a version in the first, so nothing
	// may be read out of it.
	for _, out := range []string{
		"permission denied while trying to connect to the docker API at unix:///var/run/docker.sock\n|0|0\n",
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?\n",
		"|0|0\n",
		"",
	} {
		if ver, _, _, ok := readFacts(out); ok {
			t.Errorf("a failed probe was read as a working daemon (version %q) from:\n%s", ver, out)
		}
	}

	// And a daemon that did answer is read, warnings on the same output and all.
	out := "WARNING: bridge-nf-call-iptables is disabled\n28.1.1|16856150016|4\n"
	ver, mem, cpus, ok := readFacts(out)
	if !ok || ver != "28.1.1" || mem != 16856150016 || cpus != 4 {
		t.Errorf("readFacts(%q) = %q %d %d %v", out, ver, mem, cpus, ok)
	}
}

// Not having a runtime and not being allowed to reach one need different things
// done about them, and "install Docker" is the wrong instruction for the second.
func TestTheReasonSaysWhatToDoAboutIt(t *testing.T) {
	permission := whyNot("permission denied while trying to connect to the docker API", nil)
	if !strings.Contains(permission, "docker group") {
		t.Errorf("a socket this server may not open does not mention the group: %q", permission)
	}
	if !strings.Contains(permission, "restart") {
		t.Error("it does not say a restart is needed — group membership is read once, at process start")
	}

	if got := whyNot("Cannot connect to the Docker daemon. Is the docker daemon running?", nil); !strings.Contains(got, "not running") {
		t.Errorf("a stopped daemon reads as %q", got)
	}
	if got := whyNot("", nil); got == "" {
		t.Error("a probe that said nothing gives no reason at all")
	}
}
