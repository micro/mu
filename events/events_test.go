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
