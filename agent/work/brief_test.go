package work

import (
	"mu/internal/thread"
	"mu/service/events"
	"strings"
	"testing"
)

func TestEveningComparisonUsesOnlyOwnersEarlierOutput(t *testing.T) {
	const owner = "evening_context_test"
	defer events.DeleteAll(owner)
	defer thread.Forget(owner)
	defer thread.Forget("other_brief_owner")
	if err := events.ScheduleBrief(owner, "20:00", "Etc/UTC", "daily", "evening", false); err != nil {
		t.Fatal(err)
	}
	e := events.Brief(owner, "evening")
	th := thread.Open(owner, thread.WebClient, "morning")
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "Already covered: market unchanged"})
	other := thread.Open("other_brief_owner", thread.WebClient, "private")
	thread.Add(thread.Message{Account: "other_brief_owner", Thread: other.ID, Role: thread.RoleAgent, Text: "Other person's private information"})
	s := briefContext(request{Account: owner, Kind: events.Kind, ID: e.ID})
	if !strings.Contains(s, "Already covered") || strings.Contains(s, "Other person's") {
		t.Fatal("incorrect comparison context", s)
	}
}
