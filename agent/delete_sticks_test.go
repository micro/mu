package agent

// A deleted agent stays deleted.
//
// It did not. The roster record went and the copy in agent/micro's store — the
// one the roster replaced — stayed, and ImportUserAgents ran on every startup.
// So an agent you removed came back at the next restart, and the one after
// that, from a store with no page and no way to see it. You could delete it as
// many times as you liked.
//
// Two things were wrong and both had to be fixed. Removing an agent only
// removed half of it, and a migration that runs on every boot is not a
// migration — it is a sync from a deprecated source, and a sync will always
// win against a delete.

import (
	"testing"

	"mu/agent/micro"
)

func TestRemovingAnAgentRemovesTheCopyThatWouldResurrectIt(t *testing.T) {
	acc := owner(t, "delete-sticks")

	// An agent as it existed before the roster: only in the old store.
	micro.SaveUserAgent(acc, &micro.Agent{
		ID: "legacy-foobar", Name: "Foobar", SystemPrompt: "You are Foobar",
	})

	// Startup imports it.
	if n := ImportUserAgents(micro.AllUserAgents()); n != 1 {
		t.Fatalf("imported %d agents, want 1", n)
	}
	var imported *Agent
	for _, a := range Agents(acc) {
		if a.Name == "Foobar" {
			imported = a
		}
	}
	if imported == nil {
		t.Fatal("the import did not produce a roster agent")
	}

	// You delete it.
	if err := RemoveAgent(acc, imported.ID); err != nil {
		t.Fatal(err)
	}

	// The server restarts.
	ImportUserAgents(micro.AllUserAgents())

	for _, a := range Agents(acc) {
		if a.Name == "Foobar" {
			t.Fatal("Foobar came back after a restart — deleting an agent has to delete it")
		}
	}
}

// The import drains the store it reads, so after one pass there is nothing left
// to resurrect anything from — including agents it skipped because they were
// already in the roster.
func TestTheImportEmptiesTheStoreItMigratesFrom(t *testing.T) {
	acc := owner(t, "drain-source")

	micro.SaveUserAgent(acc, &micro.Agent{ID: "legacy-a", Name: "Alpha", SystemPrompt: "a"})
	ImportUserAgents(micro.AllUserAgents())
	if left := micro.UserAgentsFor(acc); len(left) != 0 {
		t.Errorf("%d agent(s) left in the old store after importing", len(left))
	}

	// One already in the roster is still cleared out of the old store.
	micro.SaveUserAgent(acc, &micro.Agent{ID: "legacy-a2", Name: "Alpha", SystemPrompt: "a"})
	if n := ImportUserAgents(micro.AllUserAgents()); n != 0 {
		t.Errorf("imported %d duplicates, want 0", n)
	}
	if left := micro.UserAgentsFor(acc); len(left) != 0 {
		t.Errorf("a skipped duplicate was left behind to be re-read forever: %d", len(left))
	}
}
