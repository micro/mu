package tasks

// A finished task is a record of work, and a record you cannot search is a
// record you cannot check.
//
// There was no search here at all. List filters by status and returns
// everything newest first, so "what did I ask about the boiler" meant reading
// the list, and a task closed three weeks ago was reachable only by scrolling
// past everything closed since.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/data"
)

func TestATaskIsFindableByWhatWasAsked(t *testing.T) {
	const who = "tkidx1"
	t.Cleanup(func() { DeleteAll(who) })

	if _, err := Create(who, "Chase the boiler service",
		"British Gas said October, nothing since", "", time.Time{}); err != nil {
		t.Fatalf("create: %v", err)
	}

	hits := data.Search("boiler", 10, data.WithType(data.KindTask), data.WithOwner(who))
	if len(hits) == 0 {
		t.Fatal("a task is not findable by its title")
	}
	// And by the detail, which is where the useful words usually are.
	if h := data.Search("British Gas", 10, data.WithType(data.KindTask),
		data.WithOwner(who)); len(h) == 0 {
		t.Error("a task is not findable by its detail")
	}
}

// The answer an agent came back with is the part most worth finding again.
//
// A Result only exists after the run that produced it, so this is really a test
// that Update reindexes — a task indexed once at creation would be findable by
// the question and never by the answer.
func TestTheAnswerIsFindableAndNotJustTheQuestion(t *testing.T) {
	const who = "tkidx_result"
	t.Cleanup(func() { DeleteAll(who) })

	made, err := Create(who, "Check the flight refund", "BA2491, cancelled", "", time.Time{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := Update(who, made.ID, "", "", StatusDone, "",
		"They refunded £214 to the original card on 14 August"); err != nil {
		t.Fatalf("update: %v", err)
	}

	hits := data.Search("refunded", 10, data.WithType(data.KindTask), data.WithOwner(who))
	if len(hits) == 0 {
		t.Fatal("the result is not findable, so a task is searchable only by the " +
			"question somebody typed before the work was done")
	}
	// One entry, not two: the key is the record id, so an update rewrites.
	if len(hits) > 1 {
		t.Errorf("updating a task produced %d index entries", len(hits))
	}
}

// A task list is nobody else's, and is never public.
func TestATaskListIsNotPublic(t *testing.T) {
	const mine, theirs = "tkidx_mine", "tkidx_theirs"
	t.Cleanup(func() { DeleteAll(mine); DeleteAll(theirs) })

	if _, err := Create(mine, "Renew the passport", "expires in March", "", time.Time{}); err != nil {
		t.Fatalf("create: %v", err)
	}

	for _, h := range data.Search("passport", 10, data.WithType(data.KindTask),
		data.WithOwner(theirs)) {
		if strings.Contains(h.Title, "Renew the passport") {
			t.Fatal("one account's task was returned to another")
		}
	}
	for _, h := range data.Search("passport", 10, data.WithType(data.KindTask)) {
		if strings.Contains(h.Title, "Renew the passport") {
			t.Fatal("a task came back on an unowned search, so the task list is " +
				"in the archive")
		}
	}
}

// A deleted task stops being findable.
func TestADeletedTaskLeavesTheIndex(t *testing.T) {
	const who = "tkidx_del"
	t.Cleanup(func() { DeleteAll(who) })

	made, err := Create(who, "Cancel the gym", "before the renewal date", "", time.Time{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if h := data.Search("gym", 10, data.WithType(data.KindTask),
		data.WithOwner(who)); len(h) == 0 {
		t.Fatal("never indexed, so this proves nothing about deleting")
	}

	if err := Remove(who, made.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if h := data.Search("gym", 10, data.WithType(data.KindTask),
		data.WithOwner(who)); len(h) > 0 {
		t.Error("a deleted task is still findable — the index outlived it")
	}
}

// And deleting an account takes all of them.
func TestDeletingAnAccountLeavesNoTasksBehind(t *testing.T) {
	const who = "tkidx_clear"

	for _, title := range []string{"First thing", "Second thing"} {
		if _, err := Create(who, title, "indexed and then cleared", "", time.Time{}); err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
	}
	DeleteAll(who)

	if h := data.Search("cleared", 10, data.WithType(data.KindTask),
		data.WithOwner(who)); len(h) > 0 {
		t.Errorf("%d tasks are still findable after the account was deleted", len(h))
	}
}
