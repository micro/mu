package notes

import (
	"encoding/json"
	"testing"
)

func TestLegacyNoteIDsArePersistableAndStable(t *testing.T) {
	owned := map[string][]*Entry{"alice": {{Title: "Same", Text: "Private"}}, "bob": {{Title: "Same", Text: "Other"}}}
	if !ensureIDs(owned) {
		t.Fatal("legacy notes not assigned IDs")
	}
	first := owned["alice"][0].ID
	if first == "" || first == owned["bob"][0].ID {
		t.Fatal("IDs not distinct")
	}
	if ensureIDs(owned) || owned["alice"][0].ID != first {
		t.Fatal("IDs changed on another load")
	}
	b, err := json.Marshal(owned)
	if err != nil {
		t.Fatal(err)
	}
	var restored map[string][]*Entry
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if ensureIDs(restored) || restored["alice"][0].ID != first {
		t.Fatal("persisted ID lost")
	}
	if restored["alice"][0].Text != "Private" {
		t.Fatal("legacy content changed")
	}
}
