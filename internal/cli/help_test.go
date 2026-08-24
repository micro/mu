package cli

import (
	"strings"
	"testing"
)

// `mu help` says how to sign in.
//
// It printed the tool list: two hundred lines of catalogue fetched from the
// server, with no mention of login, ask, setup, agent or --url anywhere in it.
// The summary that has all of those was printed only when the fetch *failed* —
// so the half somebody needs first was reachable only once something was
// already broken, and on a working install the answer scrolled off the top of
// the terminal.
func TestHelpNamesTheCommandsYouNeedFirst(t *testing.T) {
	for _, want := range []string{
		"mu login", // there is no way to discover this from the tool list
		"mu ask",   // the agent on the instance
		"mu agent", // the agent on this machine
		"mu setup", // configuring a model for it
		"mu tools", // and where the catalogue went
		"mu logout",
		"mu config",
		"mu x402",
		"mu version",
		"--url",
		"--token",
		"MU_URL",
	} {
		if !strings.Contains(shortHelp, want) {
			t.Errorf("`mu help` never mentions %q", want)
		}
	}
}

// The two agent commands point opposite ways and the help says so, because the
// English word is the same and choosing wrong wastes a model call on the wrong
// machine.
func TestHelpDistinguishesAskFromAgent(t *testing.T) {
	if !strings.Contains(shortHelp, "opposite directions") {
		t.Error("`mu help` lists both ask and agent without saying they point " +
			"opposite ways")
	}
}

// A raw string delimited by backticks cannot contain one, and this help text is
// full of prose that wants to quote commands. It compiled to a syntax error
// once; the check costs nothing.
func TestHelpTextHasNoBackticks(t *testing.T) {
	if strings.Contains(shortHelp, "`") {
		t.Error("shortHelp contains a backtick, which ends the raw string it is in")
	}
}

// Every command the dispatcher accepts is named in the help.
//
// The rule rather than the list: a command nobody can discover is a command
// nobody uses, and the way that happened was help being generated from the
// catalogue while the commands were written by hand somewhere else.
func TestEveryCommandIsDiscoverable(t *testing.T) {
	// The dispatcher's own cases, minus the flag spellings of the same thing.
	for _, cmd := range []string{
		"login", "logout", "config", "setup", "x402", "ask", "agent", "tools", "help",
	} {
		if !strings.Contains(shortHelp, "mu "+cmd) {
			t.Errorf("the dispatcher accepts %q and the help never says so", cmd)
		}
	}
}
