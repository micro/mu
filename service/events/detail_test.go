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
