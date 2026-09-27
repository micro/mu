package events

import (
	"errors"
	"testing"
	"time"
)

func TestOwnedScheduleEditIsolationAndRollback(t *testing.T) {
	const owner = "managed-owner"
	const other = "managed-other"
	defer DeleteAll(owner)
	defer DeleteAll(other)
	e, err := Create(owner, "Mine", time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := Create(other, "Theirs", time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("abort")
	if err := EditOwned(owner, func(records map[string]*Event) error {
		if records[foreign.ID] != nil {
			t.Fatal("foreign record exposed")
		}
		records[e.ID].Title = "Changed"
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if List(owner)[0].Title != "Mine" {
		t.Fatal("failed edit changed state")
	}
	if err := EditOwned(owner, func(records map[string]*Event) error {
		records[foreign.ID] = &Event{ID: foreign.ID, Owner: owner, Title: "Overwrite"}
		return nil
	}); err == nil {
		t.Fatal("accepted foreign id")
	}
	if List(other)[0].Title != "Theirs" {
		t.Fatal("foreign record changed")
	}
}
