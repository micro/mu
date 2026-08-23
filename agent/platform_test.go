package agent

// Two namespaces of agents, and the rule that keeps them apart.
//
// you+research@ is your roster. agent+news@ is this instance's. The same word
// means different things on either side, which is only safe while the platform
// side cannot see an account — the moment it can, adding a built-in agent could
// take over an address somebody was already using, and nobody would find out
// from a compiler.

import (
	"os"
	"strings"
	"testing"

	"mu/agent/micro"
)

func readSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTheUntaggedAddressIsTheCatchAll(t *testing.T) {
	a := Platform("")
	if a == nil {
		t.Fatal("agent@<domain> resolves to nothing, so the plainest address here is dead")
	}
	if a.ID != DefaultPlatformAgent {
		t.Errorf("agent@ resolved to %q, want %q — the catch-all is what somebody "+
			"writing for the first time means", a.ID, DefaultPlatformAgent)
	}
	// And naming it explicitly is the same agent.
	if same := Platform(DefaultPlatformAgent); same == nil || same.ID != a.ID {
		t.Errorf("agent+%s@ and agent@ resolve differently", DefaultPlatformAgent)
	}
}

func TestASpecialistIsReachableByItsOwnName(t *testing.T) {
	withProbe(t)
	a := Platform(probeID)
	if a == nil {
		t.Fatalf("agent+%s@ resolves to nothing", probeID)
	}
	if a.ID != probeID {
		t.Fatalf("agent+%s@ resolved to %q", probeID, a.ID)
	}
	// Case and spacing come off the wire however the sender's client felt.
	for _, spelling := range []string{"Probe", "PROBE", " probe "} {
		if got := Platform(spelling); got == nil || got.ID != probeID {
			t.Errorf("agent+%q@ did not resolve — a local part is not "+
				"case-sensitive and senders do not agree on whitespace", spelling)
		}
	}
}

func TestAnUnknownNameResolvesToNothingRatherThanTheDefault(t *testing.T) {
	// Falling back to the catch-all would mean a typo silently gets a different
	// agent than the one asked for, and the sender never learns the name was
	// wrong. Nil is what lets the caller say so.
	if a := Platform("definitely-not-an-agent"); a != nil {
		t.Errorf("an unknown name resolved to %q instead of nothing", a.ID)
	}
}

// Every registered agent is reachable, with no second list to keep in step.
//
// The claim in platform.go is that registering a micro agent makes it
// addressable by mail. If the two ever come apart it will be because somebody
// added an agent and did not know there was a list.
func TestEveryRegisteredAgentIsAddressable(t *testing.T) {
	names := PlatformNames()
	if len(names) != len(micro.Registry) {
		t.Fatalf("%d names for %d registered agents", len(names), len(micro.Registry))
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if Platform(n) == nil {
			t.Errorf("%q is offered as an address and resolves to nothing", n)
		}
	}
	for id := range micro.Registry {
		if !seen[id] {
			t.Errorf("%q is a registered agent and is not offered at agent+%s@", id, id)
		}
	}
	if len(names) == 0 || names[0] != DefaultPlatformAgent {
		t.Errorf("names start %v — the default is the one to try, so it reads first", names)
	}
}

// A specialist brings its own tools, or it is not a specialist.
//
// The reason to write to agent+markets@ rather than agent@ is a narrower thing.
// If the allow-list did not travel with it, both addresses would reach the same
// general-purpose agent wearing a different name.
func TestASpecialistCarriesItsOwnPromptAndTools(t *testing.T) {
	withProbe(t)
	opts := PlatformOpts(Platform(probeID))
	if opts.System == "" {
		t.Error("a registered agent runs with no system prompt, so it is not that agent")
	}
	if len(opts.Tools) == 0 {
		t.Fatal("it runs with every tool, which is the catch-all wearing a name")
	}
	if !strings.Contains(strings.Join(opts.Tools, " "), "news") {
		t.Errorf("its tools are %v, none of which is the scope it was given", opts.Tools)
	}

	// The catch-all is the deliberate exception: no allow-list means all.
	if tools := PlatformOpts(Platform("")).Tools; len(tools) != 0 {
		t.Errorf("the catch-all was given an allow-list (%v), so it is no longer the "+
			"thing that handles anything", tools)
	}
}

// The platform namespace never consults an account.
//
// This is the whole safety property of having two namespaces. Platform takes no
// account argument, which is the enforcement — this test exists so that
// changing the signature to add one has to be argued for rather than done.
func TestThePlatformNamespaceIsTheSameForEveryone(t *testing.T) {
	src := readSource(t, "platform.go")
	if strings.Contains(src, "func Platform(name, account") ||
		strings.Contains(src, "accountID") || strings.Contains(src, "OwnerAccountID") {
		t.Error("platform resolution has taken an account into account. Two namespaces " +
			"are only safe while this one means the same thing to everybody — " +
			"otherwise a new built-in agent can take over an address somebody uses")
	}
	if PlatformOpts(nil).System != "" || PlatformOpts(nil).Tools != nil {
		t.Error("PlatformOpts(nil) invented options for an agent that does not exist")
	}
}
