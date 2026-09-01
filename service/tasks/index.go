package tasks

// Tasks are searchable, and finished ones especially.
//
// List filters by status and returns everything, newest first. There was no
// search at all — so "what did I ask about the boiler" meant reading the list,
// and a task closed three weeks ago was findable only by scrolling past
// everything closed since.
//
// That is the wrong way round. An open task is on a page you already look at.
// A finished one is a record of work, and a record you cannot search is a
// record you cannot check.
//
// # The result is in the index, not just the detail
//
// A task an agent ran carries a Result — the answer it came back with — and
// that is usually the part worth finding again. "What did it say about the
// flight refund" is a question about the result, not about the title somebody
// typed when they asked. Both go in, with the title as the title because the
// index scores that above the body.
//
// Steps are left out. They are the tools the agent ran and whether each worked,
// which is how you audit one task you are already looking at, not how you find
// it — and putting a run log of twenty tool names in the searchable text would
// have every task matching every tool name.
//
// # Owned, always
//
// IndexOwned and never Index. An entry with no owner is public, which is what
// the archive reads, and somebody's task list is not the archive's business.
// See data.Personal.

import (
	"strings"

	"mu/internal/auth"
	"mu/internal/data"
)

// indexType is what a task is called in the index. The word is data's — see
// data.Personal.
const indexType = data.KindTask

// indexKey is one task, for one account.
//
// Keyed on the record id, because a title can be edited and a key made of the
// title would leave the old one behind pointing at a task that has been
// renamed.
func indexKey(owner, id string) string { return indexType + ":" + owner + ":" + id }

// index puts one task where it can be found.
func index(t *Task) {
	if t == nil || t.Owner == "" || t.ID == "" {
		return
	}
	body := strings.TrimSpace(strings.Join([]string{t.Detail, t.Result}, " "))
	data.IndexOwned(indexKey(t.Owner, t.ID), indexType,
		t.Title, body, t.Owner, map[string]interface{}{
			"task":   t.ID,
			"status": t.Status,
		})
}

// unindex forgets one task.
func unindex(owner, id string) {
	if owner == "" || id == "" {
		return
	}
	data.Unindex(indexKey(owner, id))
}

// Reindex puts every task on the index.
//
// From the account list rather than the store, because userdb cannot enumerate
// owners — see internal/contacts, which has the same problem for the same
// reason. Run at boot in the background: the index is a separate file from the
// store, so an instance with one and not the other answers every search with
// nothing, including every instance upgrading to the first build that indexes
// tasks at all.
func Reindex() {
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.ID == "" {
			continue
		}
		// Every status. A finished task is the one most worth finding again,
		// and filtering to open ones here would index exactly the half that is
		// already on a page in front of you.
		for _, t := range List(acc.ID, "") {
			index(t)
		}
	}
}
