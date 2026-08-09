package agent

// Standing instructions have to be findable, and they have to run as the agent
// you scheduled them with.
//
// Every piece was already built — a scheduler, an agent, an inbox, and an
// events_create that takes a prompt and a repeat. The only way to reach it was
// to know that a calendar tool's create call has an optional prompt argument.
// A capability reachable only by knowing an argument exists is a capability
// nobody has.

import (
	"strings"
	"testing"
	"time"

	"mu/service/events"
)

func TestSchedulingATemplateRunsItAsTheAgentYouPicked(t *testing.T) {
	acc := owner(t, "standing-owner")
	a, _, err := CreateAgent(acc, "Analyst", Hosted, "You analyse things", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := CreateStanding(acc, "brief", a.ID); err != nil {
		t.Fatalf("scheduling the brief: %v", err)
	}

	var got *events.Event
	for _, e := range events.List(acc) {
		if strings.TrimSpace(e.Prompt) != "" {
			got = e
		}
	}
	if got == nil {
		t.Fatal("no standing instruction was created")
	}
	if got.Agent != a.ID {
		t.Errorf("it runs as %q, not the agent it was scheduled with — the agent you "+
			"built and scoped for the job is being ignored by the schedule", got.Agent)
	}
	if got.Repeat != events.RepeatDaily {
		t.Errorf("the brief does not repeat daily: %q", got.Repeat)
	}
	if !strings.Contains(got.Title, "Analyst") {
		t.Errorf("the title does not say whose it is: %q", got.Title)
	}
	if got.When.Before(time.Now()) {
		t.Error("it was scheduled in the past, so it fires immediately and then never again at the right hour")
	}
}

// An agent id that is not yours must not bind a schedule to somebody else's
// agent. It falls back to the default rather than failing, because the
// instruction is still worth running.
func TestSchedulingIgnoresAnAgentThatIsNotYours(t *testing.T) {
	mine := owner(t, "standing-mine")
	theirs := owner(t, "standing-theirs")
	other, _, err := CreateAgent(theirs, "Theirs", Hosted, "Not yours", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := CreateStanding(mine, "watch", other.ID); err != nil {
		t.Fatal(err)
	}
	for _, e := range events.List(mine) {
		if strings.TrimSpace(e.Prompt) != "" && e.Agent != "" {
			t.Errorf("a schedule bound to another account's agent %q", e.Agent)
		}
	}
}

func TestAnUnknownTemplateSchedulesNothing(t *testing.T) {
	acc := owner(t, "standing-unknown")
	if err := CreateStanding(acc, "not-a-template", ""); err == nil {
		t.Error("an unknown template was accepted")
	}
	for _, e := range events.List(acc) {
		if strings.TrimSpace(e.Prompt) != "" {
			t.Error("something was scheduled anyway")
		}
	}
}

// The daily template runs at its hour, not at whatever time you clicked.
func TestADailyTemplateLandsOnItsHour(t *testing.T) {
	brief := standingTemplate("brief")
	if brief == nil {
		t.Fatal("the morning brief template is gone")
	}
	// Clicked at 9am: the next 7am is tomorrow.
	at := nextRun(brief, time.Date(2026, 8, 9, 9, 0, 0, 0, time.Local))
	if at.Hour() != brief.Hour || at.Day() != 10 {
		t.Errorf("scheduled for %s, want 7am tomorrow", at.Format("2 Jan 15:04"))
	}
	// Clicked at 6am: today still works.
	at = nextRun(brief, time.Date(2026, 8, 9, 6, 0, 0, 0, time.Local))
	if at.Hour() != brief.Hour || at.Day() != 9 {
		t.Errorf("scheduled for %s, want 7am today", at.Format("2 Jan 15:04"))
	}
}
