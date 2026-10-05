package thread

import (
	"mu/internal/data"
	"testing"
)

func TestSavedPersistsAndSurvivesMerge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	Load()
	owner := "saved-merge"
	a := Open(owner, "mail", "source")
	b := Open(owner, WebClient, "target")
	if err := SetSaved(owner, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := Merge(owner, a.ID, b.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !Get(owner, b.ID).Saved {
		t.Fatal("bookmark lost during consolidation")
	}
	if err := Flush(); err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Threads []Thread `json:"threads"`
	}
	if err := data.LoadJSON("threads.json", &disk); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, th := range disk.Threads {
		if th.ID == b.ID {
			found = th.Saved
		}
	}
	if !found {
		t.Fatal("bookmark not persisted")
	}
}
