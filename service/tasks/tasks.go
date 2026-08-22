// Package tasks is the list of what is to be done — yours, or your agent's.
//
// Everything else here answers a question the moment it is asked: news now,
// weather now, a search now. An agent that only answers is a lookup service.
// Work that outlives the conversation had nowhere to live: events schedules
// something for a time, which is a clock, not a list, and db holds records with
// no shared shape, so every caller invents its own.
//
// A task is the missing furniture. It has a state, so it can be picked up and
// put down; it has an owner and an assignee, so it can be handed to the agent;
// and it keeps its result, so what the agent did is still there tomorrow. That
// is the difference between asking an agent a question and giving it a job.
//
// Records live in userdb like every other per-user thing, so ownership and the
// private model are the ones the rest of Mu already uses. Identity comes from
// the call context, never a request field.
package tasks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/userdb"
)

const (
	ns         = "tasks"
	collection = "tasks"
)

// The states a task moves through. Three, because a fourth would be a
// judgement nobody has to make: it is waiting, it is being worked on, or it is
// finished.
const (
	StatusTodo  = "todo"
	StatusDoing = "doing"
	StatusDone  = "done"
)

// Assignee values. A task is yours unless you hand it over.
const (
	Me    = "me"
	Agent = "agent"
)

// Task is one piece of work.
// Step is one tool the agent ran while working on a task.
type Step struct {
	Tool    string  `json:"tool"`
	Detail  string  `json:"detail,omitempty"` // the argument worth showing, if any
	OK      bool    `json:"ok"`
	Seconds float64 `json:"seconds"`
}

type Task struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Detail   string    `json:"detail,omitempty"`
	Status   string    `json:"status"`
	Assignee string    `json:"assignee"`
	Result   string    `json:"result,omitempty"`
	Due      time.Time `json:"due,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Owner    string    `json:"owner"`
	// Steps is what the agent did, in order — the tools it ran and whether
	// each worked. Without it a finished task is a paragraph with no way to
	// tell whether it was researched or invented.
	Steps []Step `json:"steps,omitempty"`

	// Thread is the conversation this came out of, where there was one.
	//
	// Work does not usually start on a task page. Somebody writes in and asks
	// for something that takes an hour, and what should happen is that a task
	// is made and the answer comes back where they asked — not onto a page
	// they were not looking at and have no reason to check.
	//
	// A thread id and nothing else, because this service does not know what a
	// conversation is. internal/thread holds that, whoever put the answer
	// there reads it, and a task with no origin is an ordinary task somebody
	// wrote down.
	Thread string `json:"thread,omitempty"`
}

// Open reports whether the task is still to be done.
func (t *Task) Open() bool { return t.Status != StatusDone }

// Create adds a task.
func Create(owner, title, detail, assignee string, due time.Time) (*Task, error) {
	return CreateOn(owner, "", title, detail, assignee, due)
}

// CreateOn adds a task that came out of a conversation, so the answer can go
// back to it. See Task.Thread.
func CreateOn(owner, threadID, title, detail, assignee string, due time.Time) (*Task, error) {
	if owner == "" {
		return nil, fmt.Errorf("sign in to use tasks")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("a task needs a title")
	}

	fields := map[string]any{
		"title":    title,
		"status":   StatusTodo,
		"assignee": normaliseAssignee(assignee),
		"created":  stamp(now()),
		"updated":  stamp(now()),
	}
	if detail = strings.TrimSpace(detail); detail != "" {
		fields["detail"] = detail
	}
	if threadID = strings.TrimSpace(threadID); threadID != "" {
		fields["thread"] = threadID
	}
	if !due.IsZero() {
		fields["due"] = stamp(due)
	}

	rec, err := userdb.Create(ns, owner, collection, fields, false)
	if err != nil {
		return nil, err
	}
	return toTask(rec.ID, rec.Owner, rec.Data), nil
}

// List returns the caller's tasks. status filters to one state; empty returns
// everything. Open tasks come first and oldest first within that, because a
// list of work is read from the top.
func List(owner, status string) []*Task {
	if owner == "" {
		return nil
	}
	recs, err := userdb.List(ns, owner, collection, "mine", nil, "", "", 0)
	if err != nil {
		return nil
	}

	out := make([]*Task, 0, len(recs))
	for _, r := range recs {
		t := toTask(r.ID, r.Owner, r.Data)
		if status != "" && t.Status != status {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Open() != out[j].Open() {
			return out[i].Open()
		}
		if out[i].Open() {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].Updated.After(out[j].Updated) // finished: most recent first
	})
	return out
}

// Get returns one task the caller owns.
func Get(owner, id string) (*Task, error) {
	if owner == "" {
		return nil, fmt.Errorf("sign in to use tasks")
	}
	rec, err := userdb.Get(ns, owner, collection, strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("no such task")
	}
	return toTask(rec.ID, rec.Owner, rec.Data), nil
}

// Next returns the task an agent should pick up: the oldest open one assigned
// to it. This is the whole point of the assignee field — an agent asking "what
// should I be doing?" gets an answer without a person having to say it again.
func Next(owner string) *Task {
	for _, t := range List(owner, "") {
		if t.Open() && t.Assignee == Agent {
			return t
		}
	}
	return nil
}

// Update changes a task. Empty strings leave a field as it was, so an agent
// finishing a task sends a status and a result without having to restate the
// title it was given.
// Update edits a task. runSteps is variadic so a run can record what the agent
// did without every other caller having to pass an empty slice for something
// only a run produces.
func Update(owner, id, title, detail, status, assignee, result string, runSteps ...[]Step) (*Task, error) {
	existing, err := Get(owner, id)
	if err != nil {
		return nil, err
	}

	if status != "" && !validStatus(status) {
		return nil, fmt.Errorf("status must be %s, %s or %s", StatusTodo, StatusDoing, StatusDone)
	}

	fields := map[string]any{
		"title":    firstNonEmpty(strings.TrimSpace(title), existing.Title),
		"status":   firstNonEmpty(strings.TrimSpace(status), existing.Status),
		"assignee": existing.Assignee,
		"created":  stamp(existing.Created),
		"updated":  stamp(now()),
	}
	if assignee != "" {
		fields["assignee"] = normaliseAssignee(assignee)
	}
	if d := firstNonEmpty(strings.TrimSpace(detail), existing.Detail); d != "" {
		fields["detail"] = d
	}
	if r := firstNonEmpty(strings.TrimSpace(result), existing.Result); r != "" {
		fields["result"] = r
	}
	if !existing.Due.IsZero() {
		fields["due"] = stamp(existing.Due)
	}
	// Carried forward: an edit is not a reason to forget what the agent did,
	// or where the work came from. Both are written once and never sent again,
	// so a field left out here is a field an ordinary edit deletes.
	if enc := encodeSteps(existing.Steps); enc != "" {
		fields["steps"] = enc
	}
	if existing.Thread != "" {
		fields["thread"] = existing.Thread
	}
	if len(runSteps) > 0 {
		fields["steps"] = encodeSteps(runSteps[0])
	}

	rec, err := userdb.Update(ns, owner, collection, existing.ID, fields, false)
	if err != nil {
		return nil, err
	}
	return toTask(rec.ID, rec.Owner, rec.Data), nil
}

// Remove deletes a task the caller owns.
func Remove(owner, id string) error {
	if owner == "" {
		return fmt.Errorf("sign in to use tasks")
	}
	return userdb.Delete(ns, owner, collection, strings.TrimSpace(id))
}

// Render writes tasks the way a model should read them: what to do, what state
// it is in, and what came of it.
func Render(ts []*Task) string {
	if len(ts) == 0 {
		return "No tasks."
	}
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "- [%s] %s", t.Status, t.Title)
		if t.Assignee == Agent {
			b.WriteString(" (assigned to the agent)")
		}
		if !t.Due.IsZero() {
			fmt.Fprintf(&b, " — due %s", t.Due.UTC().Format("2006-01-02 15:04 UTC"))
		}
		fmt.Fprintf(&b, "\n  id: %s\n", t.ID)
		if t.Detail != "" {
			fmt.Fprintf(&b, "  %s\n", t.Detail)
		}
		if t.Result != "" {
			fmt.Fprintf(&b, "  result: %s\n", t.Result)
		}
	}
	return b.String()
}

func validStatus(s string) bool {
	return s == StatusTodo || s == StatusDoing || s == StatusDone
}

// normaliseAssignee accepts the words a person or a model would use and lands
// on one of two answers. Anything unrecognised is you: handing work to an agent
// should be something you said, not something that was assumed.
func normaliseAssignee(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case Agent, "bot", "ai", "mu":
		return Agent
	default:
		return Me
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// stamp formats a time for storage with sub-second precision. Seconds are not
// enough: two tasks added in the same second compared equal, and the list then
// showed them in whatever order the store happened to return.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func toTask(id, owner string, d map[string]any) *Task {
	str := func(k string) string {
		v, _ := d[k].(string)
		return v
	}
	when := func(k string) time.Time {
		t, _ := time.Parse(time.RFC3339Nano, str(k))
		return t
	}
	status := str("status")
	if !validStatus(status) {
		status = StatusTodo
	}
	return &Task{
		ID: id, Title: str("title"), Detail: str("detail"),
		Status: status, Assignee: normaliseAssignee(str("assignee")),
		Result: str("result"), Due: when("due"),
		Created: when("created"), Updated: when("updated"), Owner: owner,
		Steps: decodeSteps(str("steps")), Thread: str("thread"),
	}
}

// Steps are stored as JSON text. Handing a []Step to the store as a
// map[string]any value means reading back []interface{} of
// map[string]interface{}, and decoding that by hand is how a field quietly
// turns into a list of zeroes.
func decodeSteps(raw string) []Step {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []Step
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func encodeSteps(steps []Step) string {
	if len(steps) == 0 {
		return ""
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return ""
	}
	return string(b)
}

// now is overridable so ordering can be tested without waiting.
var now = time.Now

// StepDetail picks the one argument worth showing beside a tool name.
//
// "web_search" says less than "web_search · latest AI news", and the argument
// that carries the meaning is almost always the query. Everything else — ids,
// coordinates, limits — is noise in a line meant to be read at a glance, and
// some of it is the caller's own data, which does not belong in a summary.
func StepDetail(args map[string]any) string {
	for _, key := range []string{"query", "q", "prompt", "collection", "to", "title", "category", "slug"} {
		if v, ok := args[key]; ok {
			if s := strings.TrimSpace(fmt.Sprintf("%v", v)); s != "" {
				r := []rune(s)
				if len(r) > 60 {
					return string(r[:60]) + "…"
				}
				return s
			}
		}
	}
	return ""
}

// DeleteAll removes everything tasks holds for an owner.
//
// Called when the account is deleted (internal/server/hooks.go). Without it
// the records outlived the account that made them: there was no way to ask
// this store for everything one owner had, so the deletion hooks had nothing
// to call and their task list was simply left behind.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("tasks", "deleting %s's records: %v", owner, err)
	} else if n > 0 {
		app.Log("tasks", "deleted %d records for %s", n, owner)
	}
}
