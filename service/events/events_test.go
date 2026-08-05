package events

import (
	"strings"
	"testing"
	"time"
)

func reset() {
	mu.Lock()
	events = map[string]*Event{}
	mu.Unlock()
	OnFire = nil
}

func TestCreateListCancel(t *testing.T) {
	reset()
	if _, err := Create("", "x", time.Now().Add(time.Hour), ""); err == nil {
		t.Fatal("expected error creating without an owner")
	}
	e, err := Create("alice", "Call the dentist", time.Now().Add(time.Hour), "bring card")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := Upcoming("alice"); len(got) != 1 || got[0].Title != "Call the dentist" {
		t.Fatalf("upcoming = %+v", got)
	}
	if got := Upcoming("bob"); len(got) != 0 {
		t.Fatalf("bob should see nothing, got %+v", got)
	}
	// Another user can't cancel alice's event.
	if err := Cancel("bob", e.ID); err == nil {
		t.Fatal("bob should not cancel alice's event")
	}
	if err := Cancel("alice", e.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if got := Upcoming("alice"); len(got) != 0 {
		t.Fatalf("expected empty after cancel, got %+v", got)
	}
}

func TestFireDueDeliversOnceAndDropsFromUpcoming(t *testing.T) {
	reset()
	var fired []string
	OnFire = func(acc, title, note string) { fired = append(fired, acc+":"+title) }

	// Past event fires; future event doesn't.
	Create("alice", "past", time.Now().Add(-time.Minute), "")
	Create("alice", "future", time.Now().Add(time.Hour), "")

	fireDue()
	fireDue() // second pass must not re-deliver

	if len(fired) != 1 || fired[0] != "alice:past" {
		t.Fatalf("expected one delivery of past, got %v", fired)
	}
	if got := Upcoming("alice"); len(got) != 1 || got[0].Title != "future" {
		t.Fatalf("only future should remain upcoming, got %+v", got)
	}
}

func TestICS(t *testing.T) {
	e := &Event{
		ID:    "abc123",
		Title: "Call the dentist; bring, card",
		When:  time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC),
		Note:  "second floor",
	}
	ics := ICS(e, "user@gmail.com")
	for _, want := range []string{
		"BEGIN:VCALENDAR",
		"METHOD:PUBLISH",
		"UID:abc123@mu",
		"DTSTART:20260722T140000Z",
		"DTEND:20260722T143000Z",
		`SUMMARY:Call the dentist\; bring\, card`, // escaped ; and ,
		"DESCRIPTION:second floor",
		"ORGANIZER:mailto:user@gmail.com",
		"END:VCALENDAR",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("ics missing %q\n---\n%s", want, ics)
		}
	}
	// CRLF line endings are required by RFC 5545.
	if !strings.Contains(ics, "\r\n") {
		t.Error("ics must use CRLF line endings")
	}
}

func TestGoogleCalendarURL(t *testing.T) {
	when := time.Date(2026, 7, 22, 14, 0, 0, 0, time.UTC)
	u := GoogleCalendarURL("Dentist", when, "bring card")
	for _, want := range []string{
		"calendar.google.com/calendar/render",
		"action=TEMPLATE",
		"dates=20260722T140000Z%2F20260722T143000Z",
		"text=Dentist",
		"details=bring+card",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("calendar url missing %q\n got: %s", want, u)
		}
	}
}

// Scheduling something you cannot unschedule is not safe to use for anything
// real: a standing instruction set to run every morning could not be stopped by
// the person paying for each run.
func TestAnEventCanBeCancelled(t *testing.T) {
	at := time.Now().Add(24 * time.Hour)
	e, err := Create("canceller", "Dentist", at, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove("canceller", e.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	for _, left := range List("canceller") {
		if left.ID == e.ID {
			t.Error("the event survived being cancelled")
		}
	}
}

// One person's calendar. Nobody else cancels anything in it, and an attempt
// cannot be used to find out whether an id exists.
func TestOnlyTheOwnerCanCancel(t *testing.T) {
	at := time.Now().Add(24 * time.Hour)
	e, _ := Create("owner1", "Mine", at, "")

	mine := Remove("stranger", e.ID)
	missing := Remove("stranger", "no-such-id")
	if mine == nil {
		t.Fatal("a stranger cancelled somebody else's event")
	}
	if mine.Error() != missing.Error() {
		t.Errorf("a stranger can tell an existing event from a missing one: %q vs %q",
			mine, missing)
	}
	if len(List("owner1")) != 1 {
		t.Error("the owner's event was removed anyway")
	}
}

func TestCancelRefusesNonsense(t *testing.T) {
	if Remove("", "x") == nil {
		t.Error("a signed-out caller cancelled an event")
	}
	if Remove("someone", "  ") == nil {
		t.Error("an empty id was accepted")
	}
}
