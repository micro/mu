package agent

// A second agent, for the tests about there being more than one.
//
// This instance ships one. The machinery for more is not dead — Register is
// exported, a fork is expected to add its own, and agent+<name>@ resolves
// whatever is in the registry — so the tests for scoping, addressing and
// per-agent pages need an agent with a scope to point at. They used to point at
// the markets agent, which is how ten shipped agents came to be load-bearing
// for tests about none of them in particular.
//
// Registered per test rather than at init, so a test that does not ask for a
// second agent sees exactly what the product ships.

import (
	"testing"

	"mu/agent/micro"
)

// probeID is the fixture's name, in the tests and in the addresses they build.
const probeID = "probe"

// withProbe registers a scoped agent for the duration of one test.
func withProbe(t *testing.T) *micro.Agent {
	t.Helper()
	if _, taken := micro.Registry[probeID]; taken {
		t.Fatalf("%q is already registered, so this fixture would replace a real agent", probeID)
	}
	a := &micro.Agent{
		ID:           probeID,
		Name:         "Probe",
		Description:  "A fixture with a scope",
		SystemPrompt: "You are Probe, and you answer about news.",
		Tools:        []string{"news_list", "news_search"},
		MemoryScope:  "news",
	}
	micro.Register(a)
	t.Cleanup(func() { delete(micro.Registry, probeID) })
	return a
}
