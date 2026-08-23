package agent

// The roster, as a list of people you might talk to.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

// The roster says whether an agent is alive.
//
// It was a directory: name, purpose, three links, and nothing saying whether
// any of them had ever answered anything. A list you are scanning to decide who
// to talk to needs a sign of life — "Last: Tuesday · 2 hours ago" turns an
// entry into somebody you are about to write to, and "Not used yet" is the
// honest version of the same fact.
func TestARosterRowSaysWhenItLastSpoke(t *testing.T) {
	const who = "roster-seen"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	made, _, err := CreateAgent(who, "Research", "", "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	// Made and never used: the row says so rather than leaving a gap where
	// every other row has a line.
	if got := seenLine(who, made.ID); got != "Not used yet" {
		t.Errorf("an unused agent reads %q", got)
	}

	th := thread.Open(who, thread.WebClient, "roster-seen-root")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.SetAgent(who, th.ID, made.ID)
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "What is happening in the markets?"})
	thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RoleAgent,
		Text: "Down across the board."})

	got := seenLine(who, made.ID)
	if !strings.HasPrefix(got, "Last: ") {
		t.Errorf("a used agent reads %q", got)
	}
	if !strings.Contains(got, "ago") {
		t.Errorf("the row does not say when: %q", got)
	}

	// And it is on the row itself, under the description.
	row := agentRow(made, "csrf", "https://example.test")
	if !strings.Contains(row, `class="agent-seen"`) {
		t.Errorf("the row carries no sign of life:\n%s", row)
	}
}
