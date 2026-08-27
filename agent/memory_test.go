package agent

// Whether an agent knows anything an agent beside it does not.
//
// The registry declared a MemoryScope per agent from the day the multi-agent
// system was written — "weather", "markets", "faith" — and notes.ForScopedContext
// read it. Nothing ever wrote one. So eleven agents had eleven namespaces, every
// one of them empty, and each fell back to the same shared pool: separate agents
// with identical memories, which is one agent with eleven names.
//
// That was one of the reasons the ten went. The wire is still here, because an
// agent with a scope is still a thing this instance can register and an account
// can be given — so these tests are about the two ends of it, against a fixture
// rather than against a shipped agent, since a scope only one side of the code
// believes in is exactly what was there before.

import (
	"strings"
	"testing"

	"mu/agent/micro"
	"mu/internal/notes"
)

// Every agent that answers on its own address declares a scope, or it shares
// everybody's memory. Micro is the deliberate exception: it is the catch-all,
// and the shared pool is its pool.
func TestEverySpecialistHasSomewhereToPutWhatItLearns(t *testing.T) {
	withProbe(t)
	for id, a := range micro.Registry {
		if id == DefaultPlatformAgent {
			if a.MemoryScope != "" {
				t.Errorf("the catch-all has a scope (%q), so what it learns is "+
					"hidden from anything registered beside it", a.MemoryScope)
			}
			continue
		}
		if a.MemoryScope == "" {
			t.Errorf("%s has no memory scope, so everything it learns lands in the "+
				"pool every other agent reads", id)
		}
	}
}

// The scope of the agent that answered, which is what makes a fact land in one
// place rather than the other.
func TestWhereAFactGoesIsDecidedByWhoWasAsked(t *testing.T) {
	withProbe(t)
	if got := scopeOf(probeID); got != "news" {
		t.Errorf("the fixture agent's scope is %q", got)
	}
	// The catch-all, and an agent somebody made themselves. Both go in the
	// shared pool: one because it is the pool, the other because there is no
	// registry entry to declare a scope in and splitting their own facts
	// across agents they did not know were separate would be a worse answer.
	if got := scopeOf(""); got != "" {
		t.Errorf("the default agent was given a scope: %q", got)
	}
	if got := scopeOf("u_something_somebody_made"); got != "" {
		t.Errorf("a user's own agent was given a scope: %q", got)
	}
}

// A scoped fact is the specialist's; an unscoped one is everybody's. This is
// the property the whole design rests on and it is enforced by notes, so it is
// worth a test that would catch the prefix convention drifting.
func TestASpecialistSeesItsOwnAndTheShared(t *testing.T) {
	const who = "memory_scope_reader"
	notes.Add(who, "location", "London")               // everybody's
	notes.Add(who, "weather:forecast", "the 5am one")  // the weather agent's
	notes.Add(who, "markets:holdings", "mostly gilts") // somebody else's

	weather := notes.ForScopedContext(who, "weather")
	if !strings.Contains(weather, "London") {
		t.Error("the weather agent cannot see a fact about the person")
	}
	if !strings.Contains(weather, "the 5am one") {
		t.Error("the weather agent cannot see its own memory")
	}
	if strings.Contains(weather, "gilts") {
		t.Error("the weather agent can read what was said to the markets agent")
	}

	// And the prefix does not reach the prompt: the agent is told "forecast",
	// not "weather:forecast", because the namespace is bookkeeping.
	if strings.Contains(weather, "weather:") {
		t.Errorf("the namespace leaked into the context:\n%s", weather)
	}
}
