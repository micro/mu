package events

import "testing"

func TestEventDetailsRespectOwner(t *testing.T) {
	mu.Lock()
	events["detail-owner-test"] = &Event{ID: "detail-owner-test", Owner: "alice", Title: "Private"}
	mu.Unlock()
	defer func() { mu.Lock(); delete(events, "detail-owner-test"); mu.Unlock() }()
	if ownedEvent("bob", "detail-owner-test") != nil {
		t.Fatal("another owner's event disclosed")
	}
	e := ownedEvent("alice", "detail-owner-test")
	if e == nil {
		t.Fatal("owner cannot read event")
	}
	e.Title = "Changed"
	if ownedEvent("alice", "detail-owner-test").Title != "Private" {
		t.Fatal("read returned mutable stored record")
	}
}

func TestExternalEventLinksRejectScripts(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "//evil.example", "data:text/html,test"} {
		if externalURL(External{URL: raw}) != "/events" {
			t.Fatal(raw)
		}
	}
	if externalURL(External{URL: "https://calendar.google.com/calendar/event?eid=123"}) == "/events" {
		t.Fatal("valid event link rejected")
	}
}
