package inbox

// Notes and tasks arrive here too.
//
// The inbox was conversations. Everything else you wrote down lived somewhere
// else — a note on /notes, a task on /tasks — and a person with something on
// their mind had to decide, before writing it, which of three pages it belonged
// on. That is a filing decision demanded up front, about a sentence that has not
// been written yet, and it is the reason people keep everything in their mail:
// one place, and you sort it later if you sort it at all.
//
// So the inbox lists all three. What arrived, what you wrote down, and what is
// still to be done, in one column, newest first, each saying which it is.
//
// # A composition, not a fourth store
//
// Nothing here is copied. The note is still in internal/notes, the task is
// still in service/tasks, the conversation is still in internal/thread, and
// this reads the three and merges them. A fourth store holding a shadow of the
// other three is the shape of the bug this repository has paid for twice —
// the note would be edited on /notes and stale here, and there would be no
// answer to which one is true.
//
// That is what inbox/ is: a composition over mail, chat, SMS and the record.
// Two more sources is the same move, and the package doc's rule allows it —
// internal/, and services, which provide tools rather than consuming them.
//
// # One row shape
//
// A note and a task render into the same three-line row a conversation does:
// what it is on the left, the facts about it on the right, then the title, then
// the first of what it says. Not because the shapes are interchangeable but
// because the eye running down the column should not have to change gear —
// which is exactly what a list of mixed things does when each kind invents its
// own layout.

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/notes"
	"mu/internal/thread"
	"mu/service/tasks"
)

// The kinds an inbox holds, and the ones /inbox/new can start.
//
// Strings rather than an enum because they travel: they are the value of the
// kind pill on the compose page and the value of the filter on the list, and a
// vocabulary the server published is exactly what a URL may carry. See
// AGENTS.md, "What may travel in a URL".
const (
	kindMessage = "message"
	kindNote    = "note"
	kindTask    = "task"
)

// item is one thing in the list, already drawn, with the time it sorts on.
//
// Rendered rather than kept as a union of three types. The three sources have
// nothing structurally in common — a thread has parties and a client, a note
// has a title and a body, a task has a status and an assignee — and inventing a
// struct wide enough for all of them would be a fourth model of the same data,
// which is the thing the note above says not to build. What they do have in
// common is that each becomes one row and rows sort by when they last changed.
type item struct {
	at   time.Time
	html string
}

// everything is the inbox: conversations, notes and open tasks, newest first.
//
// only narrows to one kind, empty for all of them.
//
// A finished task stays on the list. It is a record of work, and a list that
// quietly drops things the moment they are done cannot be used to check what was
// done — the same reason a read conversation does not disappear. What it stops
// doing is claiming to be waiting: taskRow drops the unread mark when it closes.
func everything(r *http.Request, accountID, box, only string) []item {
	var out []item

	if only != kindNote && only != kindTask {
		all := arrivals(accountID)
		for _, t := range all {
			if only != "" && only != kindMessage && t.Client != only {
				continue
			}
			// The mailbox narrows conversations only. A note has no agent and no
			// alias, so a mailbox is not a question that can be asked of it —
			// and a box filter that silently dropped every note would look like
			// the notes had gone.
			if box != "" && !strings.EqualFold(boxOfThread(accountID, t), box) {
				continue
			}
			out = append(out, item{at: t.Updated, html: row(r, accountID, t)})
		}
	}

	// Notes and tasks are not in a mailbox, so a mailbox view has none of them.
	if box == "" && (only == "" || only == kindNote) {
		for _, n := range notes.All(accountID) {
			out = append(out, item{at: n.UpdatedAt, html: noteRow(n)})
		}
	}
	if box == "" && (only == "" || only == kindTask) {
		for _, t := range tasks.List(accountID, "") {
			out = append(out, item{at: t.Updated, html: taskRow(t)})
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

// noteRow opens the original note inside Inbox.
func noteRow(n *notes.Entry) string {
	text := strings.TrimSpace(n.Text)
	return `<div class="ib-item">` +
		`<a class="ib-row" href="/inbox?kind=note&amp;id=` + url.QueryEscape(n.ID) + `">` +
		rowMeta("", "Note", nil, n.UpdatedAt) +
		`<span class="ib-subject">` + html.EscapeString(trimTo(n.Title, 90)) + `</span>` +
		`<span class="ib-snip">` + html.EscapeString(trimTo(text, 110)) + `</span></a></div>`
}

// taskRow is a task, in the same row.
//
// The bordered type label identifies the task; status and assignee are context.
// What is done says so and stays on the list — a list that quietly drops
// finished work cannot be used to check what was done.
func taskRow(t *tasks.Task) string {
	tags := []string{t.Status}
	if t.Assignee == tasks.Agent {
		name := agentLabel(t.Owner, t.Agent)
		if name == "" {
			name = defaultAgentName()
		}
		tags = append(tags, name)
	}

	// The result when there is one, because a finished task's answer is the
	// thing worth previewing; the detail is what you asked for and you know it.
	snippet := strings.TrimSpace(t.Outcome())
	if snippet == "" {
		snippet = strings.TrimSpace(t.Detail)
	}

	cls := "ib-row"
	if t.Open() {
		cls += " unseen"
	}
	return `<div class="ib-item">` +
		`<a class="` + cls + `" href="/inbox?kind=task&amp;id=` + url.QueryEscape(t.ID) + `">` +
		rowMeta("", "Task", tags, t.Updated) +
		`<span class="ib-subject">` + html.EscapeString(trimTo(t.Title, 90)) + `</span>` +
		`<span class="ib-snip">` + html.EscapeString(trimTo(snippet, 110)) + `</span></a></div>`
}

// kinds is the filter above the list: everything, or one kind of thing.
//
// Beside the mailboxes rather than inside them, because they answer different
// questions — a mailbox is which agent's address something came to, a kind is
// what sort of thing it is — and a row that mixed the two would let somebody
// pick a combination that is always empty.
//
// Keep service names visible even before the first item arrives.
func kinds(current string) string {
	var b strings.Builder
	// "Type", not "Kind". Kind is this repository's own word — data.Vocabulary
	// calls them kinds and the URL still says kind=, because that vocabulary
	// travels and renaming it would break a link somebody saved. What a reader
	// is being asked here is what sort of thing to show, and the word for that
	// on a screen is Type.
	//
	// Inline, in front of its chips, the same as the mailbox row it sits
	// beside — see boxes for why the heading came off the top and went here.
	b.WriteString(`<div class="ib-boxes ib-kinds"><span class="ib-axis">Type</span>`)
	for _, k := range []struct{ label, val string }{
		{"Everything", ""},
		{"Mail", mailClient},
		{"Chat", thread.ChatClient},
		{"SMS", thread.SMSClient},
		{"WhatsApp", thread.WhatsAppClient},
		{"Notes", kindNote},
		{"Tasks", kindTask},
	} {
		href := "/inbox"
		if k.val != "" {
			href += "?kind=" + url.QueryEscape(k.val)
		}
		b.WriteString(app.PillLink(k.label, href, k.val == current))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// kindOf is which kind a request is asking for, and only ever one we published.
//
// Anything else is everything, rather than an error: a kind is a view, and a
// typo in a view name should show you the page rather than a refusal.
func kindOf(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case mailClient, thread.ChatClient, thread.SMSClient, thread.WhatsAppClient:
		return strings.ToLower(strings.TrimSpace(v))
	case kindMessage:
		return kindMessage
	case kindNote:
		return kindNote
	case kindTask:
		return kindTask
	}
	return ""
}
