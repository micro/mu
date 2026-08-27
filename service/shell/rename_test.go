package shell

// The rename must not take a running instance's configuration with it.

import "testing"

// An instance configured before the rename keeps its settings.
//
// SANDBOX_SSH_PORT is in an operator's environment right now. A build that
// reads only SHELL_SSH_PORT finds nothing, and the SSH door goes quiet on a
// deploy whose changelog says "renamed a package".
func TestTheOldSettingNamesStillWork(t *testing.T) {
	t.Setenv("SANDBOX_SSH_PORT", "2222")
	if got := setting("SHELL_SSH_PORT"); got != "2222" {
		t.Errorf("setting(SHELL_SSH_PORT) = %q with only SANDBOX_SSH_PORT set; "+
			"an instance configured before the rename loses its SSH door", got)
	}
}

// And the new name wins where both are set, or an operator who has migrated
// cannot tell which one the instance is obeying.
func TestTheNewSettingNameWins(t *testing.T) {
	t.Setenv("SANDBOX_MEMORY", "256m")
	t.Setenv("SHELL_MEMORY", "1g")
	if got := setting("SHELL_MEMORY"); got != "1g" {
		t.Errorf("setting(SHELL_MEMORY) = %q, want the new name to win", got)
	}
}

// A machine started by the old build is still found at boot.
//
// The sweep finds containers from a previous run by name prefix, and the first
// boot of this build has a predecessor that used mu-sandbox-. Those containers
// hold their memory caps until something stops them.
func TestTheBootSweepKnowsBothPrefixes(t *testing.T) {
	if namePrefix == oldNamePrefix {
		t.Fatal("the two prefixes are the same, so this proves nothing")
	}
	for _, want := range []string{"mu-shell-", "mu-sandbox-"} {
		if namePrefix != want && oldNamePrefix != want {
			t.Errorf("neither prefix is %q, so machines named that are never swept", want)
		}
	}
}
