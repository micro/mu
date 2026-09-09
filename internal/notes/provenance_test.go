package notes

import "testing"

func TestNotesRetainProvenanceAndIsolation(t *testing.T) {
	resetStore(t)
	AddFrom("alice", "location", "London", "thread-one")
	AddFrom("bob", "location", "Paris", "thread-two")
	AddFrom("alice", "location", "Hampton", "thread-three")
	if len(All("alice")) != 1 || All("alice")[0].SourceThread != "thread-three" || Get("bob", "location") != "Paris" {
		t.Fatal("crossed accounts or lost correction provenance")
	}
	if ForContext("") != "" {
		t.Fatal("guest read personal notes")
	}
	Delete("alice", "location")
	if ForContext("alice") != "" || Get("bob", "location") != "Paris" {
		t.Fatal("deletion was not account scoped")
	}
	AddFrom("", "location", "secret", "thread")
	if len(All("")) != 0 {
		t.Fatal("stored guest memory")
	}
}
