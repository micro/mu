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
// with the ones that are true today and nothing else. A number in a clause with
// a name attached is a different thing: "3 waiting, the newest from Henrik" is
// a decision somebody can make without opening anything, and a tile is not.
//
// Three clauses are about you — what arrived, what the agents have in hand,
// what is owed — and the fourth is about everything else, because the page they
// sit on was failing at that. Home's moving parts are sixteen cards below two
// sections that change only when you use them, so an instance fetching news,
// video and the firehose all day looks idle from the front. See arrived().
//
// # Not the digest
//
// agent/digest writes a daily briefing and publishes it as a blog post:
// headlines, markets, video, synthesised by a model, the same for everybody.
// The two overlap now that a clause here counts the world, so the line between
// them is worth drawing properly rather than by subject. The digest is a piece
// of writing, produced once, that you go and read. This is four facts, rebuilt
// whenever they change, on the page you were already on — and it costs nothing,
// which is why it can be on every load and the digest cannot.
//
// # Not the inbox either
//
// The inbox is where work is tracked, triaged and read at length. This is the
// glance before that — how many, who from, what is moving — and its whole
// success condition is that somebody can skip it and be right about their day.
//
// # Why nothing calls a model
//
// It would read better. It would also cost a credit and two seconds on every
// load of the front page, to say something these facts already say, and be
// occasionally wrong about them. quota.json prices real cost, and a summary of
// four numbers this instance already holds is not one.
//
// A model pass over the assembled facts is a good thing to add later — once a
// day, cached, saying what is *worth* attention rather than what exists. What
// it should not be is a call per page view.

import (
	"html"
	"strconv"
	"strings"
	"time"

	"mu/inbox"
	"mu/internal/app"
	"mu/internal/arrivals"
	"mu/service/tasks"
)

// brief is how things are, or nothing at all.
//
// Silent when there is nothing true to say, which is most of a quiet week. A
// line reading "Nothing new" is a sentence that costs a reader a glance and
// gives them nothing back, and it is on the screen they see most often.
func brief(accountID string) string {
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
	if s := arrived(); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return `<p class="home-brief">` + strings.Join(parts, " ") + `</p>`
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

// arrived is what came in today: how much, and what the newest of it is called.
//
// The one clause here that is not about you, and the reason it exists is a
// complaint about this page. Everything on Home that actually moves — news,
// video, the social firehose, markets, weather — is sixteen cards at the
// bottom, under two sections that change only when you use them. The services
// fetch all day and the front page reads as though nothing is happening. This
// says how much did, in the line somebody already reads.
//
// Last of the four on purpose. The three above are things that need you and
// this is a thing that happened, which is the right order on a day when both
// are true; on a quiet day it is the only clause, which is the day it matters.
//
// Not per-account: the rows it counts are public, so the answer is the same for
// everybody and internal/arrivals works it out once for the instance. It shows
// only to a signed-in reader because the whole brief does.
func arrived() string {
	day := arrivals.Today()
	if !day.Any() {
		return ""
	}

	var kinds []string
	for _, k := range arrivals.Kinds {
		if n := day.Counts[k.Type]; n > 0 {
			kinds = append(kinds, count(n, k.One, k.Many))
		}
	}
	if len(kinds) == 0 {
		return ""
	}

	out := app.TextLink(series(kinds), "/archive") + " today"
	if day.Newest != "" {
		out += ", the newest “" + html.EscapeString(clip(day.Newest, 60)) + "”"
	}
	return out + "."
}

// series writes a list the way it is said: "a", "a and b", "a, b and c".
func series(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// clip shortens a title to fit in a sentence, at a word where it can.
//
// Headlines run to a hundred characters and this one is quoted inside a clause
// about three other things. Cut at the last space before the limit so the
// ellipsis follows a word rather than lands mid-syllable, and count runes,
// because a title is as likely to be in a language that does not have one byte
// per letter.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:—-") + "…"
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
