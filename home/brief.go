package home

// How things are, in a sentence, before you look anywhere.
//
// Home had a box to type in and then a list of conversations, and everything
// else about your own day was somewhere you had to go and find: the inbox for
// what arrived, /tasks for what is being worked on, /agents for who is working.
// Three pages to answer one question, which is the question somebody actually
// arrives with — is there anything I need to know.
//
// So this sits between the box and the inbox and answers it. Not a dashboard of
// counts: four tiles reading Unread 0, Tasks 0 is the thing Home already
// refused, and its reason holds — a number duplicates a sidebar badge and says
// nothing you can act on. What is here is the same facts written as clauses,
// with the ones that are true today and nothing else.
//
// Three of the four are about you — what arrived, what the agents have in hand,
// what is owed — and the fourth is about everything else, because the page they
// sit on was failing at that. Home's moving parts are sixteen cards below two
// sections that only change when you use them, so an instance fetching news and
// posts and prices all day looked idle from the front.
//
// # Not the digest
//
// agent/digest writes a daily briefing and publishes it as a blog post:
// headlines, markets, video, the same for everybody, about the world. The
// fourth clause here is about the world too, so the line between them is not
// the subject: the digest is five paragraphs you go and read, and this is one
// sentence on the page you were already on. A thing to read, and a reason to
// stop reading.
//
// # Not the inbox either
//
// The inbox is where work is tracked, triaged and read at length. This is the
// glance before that — how many, who from, what is moving — and its whole
// success condition is that somebody can skip it and be right about their day.
//
// # Why nothing here calls a model
//
// It would read better. It would also cost a credit and two seconds on every
// load of the front page, to say something these facts already say, and be
// occasionally wrong about them. quota.json prices real cost, and a summary of
// four numbers this instance already holds is not one.
//
// The fourth clause is a model's work and still costs nothing here, because the
// objection above is about *when*: agent/brief writes that sentence on a timer
// for the whole instance and this reads the string. See happening().

import (
	"html"
	"strconv"
	"strings"
	"time"

	"mu/agent/brief"
	"mu/inbox"
	"mu/internal/app"
	"mu/service/tasks"
)

// briefHTML is your day, or nothing at all.
//
// Named for what it returns, because the package holding the world's sentence
// is called brief and one of the two had to give. Sibling of cardHTML.
//
// # Four clauses, and the last one is the world
//
// The world's sentence was moved out to sit over the Services cards for one
// commit and came straight back. The argument for moving it was that it is
// about the world and the cards are the world; the answers are that services
// are services, and that a bare paragraph over a grid had no card of its own so
// it read as text that had escaped. Where something belongs is not settled by
// what it is about.
//
// What that attempt was right about is the proportion. The world's sentence is
// 256 characters and each of the others is about forty, so this block is mostly
// news whatever the order says. That is a reason to shorten the sentence or to
// give this block more to say about your day — not a reason to file it
// elsewhere.
//
// Silent when there is nothing true to say, which is most of a quiet week. A
// line reading "Nothing new" costs a reader a glance and gives them nothing
// back, and it is on the screen they see most often.
//
// That includes being alone. For one commit this drew a section saying "Just
// you here" over a link to the chat, on the argument that who is present is
// true on the quietest day. True and useless as a sentence — who is here is its
// own block under the box now, and it names people rather than telling you
// there are none. See hereHTML.
func briefHTML(accountID string) string {
	if accountID == "" {
		return ""
	}

	var parts []string
	if s := waiting(accountID); s != "" {
		parts = append(parts, s)
	}
	if s := working(accountID); s != "" {
		parts = append(parts, s)
	}
	if s := owed(accountID); s != "" {
		parts = append(parts, s)
	}
	if s := happening(); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}

	// In a card, like the two blocks under it.
	//
	// It was a bare paragraph under a heading, which was right when Home was
	// one column: the brief ran the width of the page and the border would have
	// been a box around a single sentence. In the rail it sits directly above
	// the inbox and the agents, both of them bordered, and the odd one out was
	// the one at the top — three blocks under matching headings, the first
	// looking like a caption that had escaped.
	//
	// brief-peek, and the class is in the same rule as inbox-peek and
	// agent-peek in mu.css rather than beside them. Three names for one shape,
	// kept in one place, because the whole complaint was that they had drifted.
	//
	// No presence line in here any more.
	//
	// Who else was online drew above the clauses for a while, on the argument
	// that it is the only thing on this page that changes because somebody else
	// did something. That was right about the fact and wrong about the place: it
	// is exactly why it does not belong buried in the rail under a heading
	// saying Brief. It is its own block under the box now, and it names people
	// rather than counting them — see hereHTML.
	return sectionRule("Brief") + `<div class="brief-peek">` +
		`<p class="home-brief">` + strings.Join(parts, " ") + `</p></div>`
}

// waiting is what has arrived and not been read.
//
// app.TextLink throughout, not app.Link: the second is a display:block anchor
// that appends an arrow, which is right at the end of a card and wrong four
// words into a sentence — "3 conversations →, the newest from Henrik" breaks
// the line and leaves a stray arrow before a comma. That distinction is why
// TextLink exists; its own comment says so.
//
// Who it is from, not only how many. "3 waiting" is a number; "3 waiting, the
// newest from Henrik" is a reason to open it or a reason not to, which is the
// decision this line exists to let somebody make without opening anything.
func waiting(accountID string) string {
	n, newest := inbox.Waiting(accountID)
	if n == 0 {
		return ""
	}
	out := app.TextLink(count(n, "conversation", "conversations"), "/inbox") + " waiting"
	if newest != "" && !strings.EqualFold(newest, "You") {
		out += ", the newest from " + html.EscapeString(newest)
	}
	return out + "."
}

// working is what the agents have in hand.
//
// Named, because "1 task running" tells you work is happening and not which of
// your agents is doing it — and the answer to "should I wait for this" usually
// depends on which. Assignee is me or agent today; when a task can name one of
// the roster this reads correctly without changing.
func working(accountID string) string {
	doing := tasks.List(accountID, tasks.StatusDoing)
	if len(doing) == 0 {
		return ""
	}
	// The oldest, because a run that started four hours ago is the one worth
	// knowing about. tasks.List is newest first.
	oldest := doing[len(doing)-1]
	out := "The agent is on " + app.TextLink(count(len(doing), "thing", "things"), "/tasks")
	if !oldest.Updated.IsZero() {
		out += ", the longest since " + html.EscapeString(app.TimeAgo(oldest.Updated))
	}
	return out + "."
}

// owed is what is waiting on you: due today, then everything else open.
//
// Overdue first and separately, because a task that has passed its date is the
// one fact on this line somebody may need to act on today, and burying it in a
// total is how it stops being one.
func owed(accountID string) string {
	todo := tasks.List(accountID, tasks.StatusTodo)
	if len(todo) == 0 {
		return ""
	}

	now := time.Now()
	late, today := 0, 0
	for _, t := range todo {
		switch {
		case t.Due.IsZero():
		case t.Due.Before(now):
			late++
		case sameDay(t.Due, now):
			today++
		}
	}

	switch {
	case late > 0 && today > 0:
		return app.TextLink(count(late, "task", "tasks"), "/tasks") + " overdue and " +
			strconv.Itoa(today) + " due today."
	case late > 0:
		return app.TextLink(count(late, "task", "tasks"), "/tasks") + " overdue."
	case today > 0:
		return app.TextLink(count(today, "task", "tasks"), "/tasks") + " due today."
	}
	return app.TextLink(count(len(todo), "task", "tasks"), "/tasks") + " open."
}

// happening is what the world did today, in agent/brief's words.
//
// Last of the four on purpose. The three above are things that need you and
// this is a thing that happened, which is the right order on a day when both
// are true; on a quiet day it is the only clause, which is the day it matters.
//
// It spent one commit under the Services heading instead, on the argument that
// the sentence is about the world and the cards are the world. Wrong twice
// over: services are services, and a bare paragraph over a grid of cards had no
// card of its own, so it read as text that had escaped from something. Where a
// thing belongs is not settled by what it is about.
//
// Escaped, because unlike the other three this is a model's prose rather than
// counts and names from this instance's own stores.
//
// Not per-account: the rows behind it are public, so the sentence is the same
// for everybody and is written once for the instance rather than once per
// person. What is personal on this line is the three clauses above it.
func happening() string {
	line := brief.Line()
	if line == "" {
		return ""
	}
	return html.EscapeString(line)
}

// sameDay is whether two times fall on the same date, in the same zone.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// count writes a number and its noun, agreeing.
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
