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
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/account"
	"mu/agent/brief"
	"mu/inbox"
	"mu/internal/app"
	"mu/service/events"
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
// there are none.
func briefHTML(accountID string, external ...events.External) string {
	parts := briefParts(accountID, external...)
	if len(parts) == 0 {
		return ""
	}

	// Home is a live summary, not an archived or separately navigable card.
	return `<p class="home-brief">` + strings.Join(parts, " ") + `</p>`
}

// briefParts is the clauses, without deciding how they are set.
//
// Split out because the front door shows the same sentence in a different
// shape: a centred line on a page with nothing else on it, rather than a card
// in a rail between the inbox and the agents. Two renderings of one brief, and
// only one place that decides what is in it — which is the property worth
// having, because a second copy of these five calls would answer the same
// question differently within a month.
//
// The last clause is the world's and is the only one an account is not needed
// for, so a signed-out reader gets it alone. That is the whole of the public
// brief: everyone's day is the same, yours is not.
func briefParts(accountID string, external ...events.External) []string {
	var parts []string
	if accountID != "" {
		if s := waiting(accountID); s != "" {
			parts = append(parts, s)
		}
		if s := working(accountID); s != "" {
			parts = append(parts, s)
		}
		// What is actually on today, before the world's line.
		//
		// The brief said what was waiting, what the agent was doing and what
		// was owed — three questions about work — and nothing at all about the
		// day. So somebody with a dentist at four and a school pickup at three
		// read a line about their inbox and went to look at a calendar, which
		// is the one thing a brief is supposed to save.
		if s := onToday(accountID, external...); s != "" {
			parts = append(parts, s)
		}
	}
	if s := happening(); s != "" {
		parts = append(parts, s)
	}
	return parts
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
// onToday is what is in the diary between now and the end of the day.
//
// Only what is still ahead. A brief read at six in the evening that says "3
// things today" when all three have happened is worse than saying nothing —
// it is a number that cannot be acted on, and the reader has to open the
// calendar to find that out, which is the trip this exists to save.
//
// The next one is named, with its time, because that is the fact somebody
// actually wants: not how many, but what and when. The count carries the rest.
func onToday(accountID string, external ...events.External) string {
	now := account.LocalNow(accountID)
	var ahead []*events.Event
	for _, e := range events.List(accountID) {
		if e == nil || e.Paused || e.Kind == "brief" || e.Prompt != "" || e.When.IsZero() {
			continue
		}
		if sameDay(e.When.In(now.Location()), now) && e.When.After(now) {
			ahead = append(ahead, e)
		}
	}
	allDay := map[*events.Event]bool{}
	for _, e := range external {
		if (e.AllDay && e.Start.Format("2006-01-02") <= now.Format("2006-01-02") && e.End.Format("2006-01-02") > now.Format("2006-01-02")) || (sameDay(e.Start.In(now.Location()), now) && e.Start.After(now)) {
			item := &events.Event{Title: e.Title, When: e.Start}
			ahead = append(ahead, item)
			allDay[item] = e.AllDay
		}
	}
	if len(ahead) == 0 {
		return ""
	}

	// Soonest first. events.List does not promise an order, and "the next
	// thing" is wrong if it is merely the first one stored.
	sort.Slice(ahead, func(i, j int) bool { return ahead[i].When.Before(ahead[j].When) })

	next := ahead[0]
	out := app.TextLink(html.EscapeString(next.Title), "/events")
	if allDay[next] {
		out += " today"
	} else {
		out += " at " + html.EscapeString(next.When.In(now.Location()).Format("15:04"))
	}
	if rest := len(ahead) - 1; rest > 0 {
		// The Home fetch is bounded. Do not present a partial count as the total.
		if len(external) >= events.PreviewLimit {
			out += ", with more today"
		} else {
			out += ", and " + strconv.Itoa(rest) + " more today"
		}
	}
	return out + "."
}

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
