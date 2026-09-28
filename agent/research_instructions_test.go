package agent

import (
	"mu/service/events"
	"strings"
	"testing"
	"time"
)

func TestResearchInstructionsPersistWithoutChangingDelivery(t *testing.T) {
	owner := "research-instructions"
	when := time.Now().Add(time.Hour).UTC()
	e := &events.Event{ID: owner, Owner: owner, Kind: "research", Prompt: "Islam", Note: "Existing context", When: when, Repeat: "daily", Zone: "UTC", Paused: true, MaxCredits: 20, Sequence: 7, ResearchDigest: "old", ResearchReport: "old report"}
	if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[e.ID] = e; return nil }); err != nil {
		t.Fatal(err)
	}
	result, err := manageResearch(owner, map[string]any{"instructions": "Existing context. Focus on learning Arabic for Quran reading."})
	if err != nil || !strings.Contains(result, "Quran") {
		t.Fatalf("%s %v", result, err)
	}
	got := Research(owner)
	if got.Note == e.Note || got.Prompt != "Islam" || !got.When.Equal(when) || !got.Paused || got.MaxCredits != 20 || got.Sequence != 8 || got.ResearchDigest != "" || got.ResearchReport != "" {
		t.Fatalf("bad update: %+v", got)
	}
	if !strings.Contains(researchSearchQuery(got), "Quran") {
		t.Fatal("instructions missing from search")
	}
	if _, err := manageResearch("research-other", map[string]any{"instructions": "replace"}); err == nil {
		t.Fatal("foreign schedule updated")
	}
	if _, err := manageResearch(owner, map[string]any{"instructions": strings.Repeat("x", 4001)}); err == nil {
		t.Fatal("unbounded instructions")
	}
	if _, err := manageResearch(owner, map[string]any{}); err != nil || Research(owner).Sequence != 8 {
		t.Fatal("reading mutated schedule")
	}
}
