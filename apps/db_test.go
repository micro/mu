package apps

import "testing"

func recs() []Record {
	return []Record{
		{ID: "1", Owner: "alice", Public: false, Data: map[string]interface{}{"title": "a-priv"}},
		{ID: "2", Owner: "alice", Public: true, Data: map[string]interface{}{"title": "a-pub"}},
		{ID: "3", Owner: "bob", Public: false, Data: map[string]interface{}{"title": "b-priv"}},
		{ID: "4", Owner: "bob", Public: true, Data: map[string]interface{}{"title": "b-pub"}},
	}
}

func ids(rs []Record) map[string]bool {
	m := map[string]bool{}
	for _, r := range rs {
		m[r.ID] = true
	}
	return m
}

func TestFilterScopeMine(t *testing.T) {
	got := ids(filterRecords(recs(), "mine", "alice", nil))
	// alice sees only her own records, private and public
	if !got["1"] || !got["2"] || got["3"] || got["4"] {
		t.Fatalf("mine scope wrong: %v", got)
	}
}

func TestFilterScopePublicHidesOthersPrivate(t *testing.T) {
	got := ids(filterRecords(recs(), "public", "alice", nil))
	// public scope: every public record, no private ones (incl. own private)
	if !got["2"] || !got["4"] || got["1"] || got["3"] {
		t.Fatalf("public scope wrong: %v", got)
	}
}

func TestFilterScopeAll(t *testing.T) {
	got := ids(filterRecords(recs(), "all", "alice", nil))
	// all: mine (private+public) + others' public — but never others' private
	if !got["1"] || !got["2"] || !got["4"] || got["3"] {
		t.Fatalf("all scope wrong: %v", got)
	}
}

func TestFilterGuestSeesNoPrivate(t *testing.T) {
	// A guest (empty caller) in public scope must never see a private record.
	got := filterRecords(recs(), "public", "", nil)
	for _, r := range got {
		if !r.Public {
			t.Fatalf("guest saw private record %s", r.ID)
		}
	}
	if len(got) != 2 {
		t.Fatalf("guest public count = %d, want 2", len(got))
	}
}

func TestFilterWhere(t *testing.T) {
	got := ids(filterRecords(recs(), "all", "alice", map[string]interface{}{"title": "a-pub"}))
	if len(got) != 1 || !got["2"] {
		t.Fatalf("where filter wrong: %v", got)
	}
}

func TestSortRecordsByField(t *testing.T) {
	rs := []Record{
		{ID: "1", Data: map[string]interface{}{"n": float64(3)}},
		{ID: "2", Data: map[string]interface{}{"n": float64(1)}},
		{ID: "3", Data: map[string]interface{}{"n": float64(2)}},
	}
	sortRecords(rs, "n", "asc")
	if rs[0].ID != "2" || rs[1].ID != "3" || rs[2].ID != "1" {
		t.Fatalf("asc sort wrong: %v %v %v", rs[0].ID, rs[1].ID, rs[2].ID)
	}
	sortRecords(rs, "n", "desc")
	if rs[0].ID != "1" || rs[2].ID != "2" {
		t.Fatalf("desc sort wrong: %v %v %v", rs[0].ID, rs[1].ID, rs[2].ID)
	}
}
