package home

// The cards, as something an agent can read.
//
// The home screen is a summary of the services somebody chose to watch — news
// headlines, market movers, what is on today. That is exactly the context a
// question about any of it would otherwise be answered by fetching, one tool
// call at a time, after the model has worked out which tool to call.
//
// So the cards are offered to the agent as text. Somebody who has already
// decided "these are the things I care about" has done the work of saying what
// their assistant should know, and the same choice drives both. Ask "anything I
// should know today" with context on, and the answer is composed rather than
// researched.
//
// Opt-in per message, not always on. Context is not free — it is tokens on
// every turn, and most questions have nothing to do with the cards. The toggle
// makes it a thing somebody reaches for when the summary is the point.
//
// Text, not HTML, and capped. This is read by a model, and the cap is what
// stops a long news day from crowding out the actual question.

import (
	"regexp"
	"strings"

	"mu/internal/auth"
)

// contextCap is the most card text passed in one turn. Generous enough for a
// handful of cards, small enough that it cannot dominate a prompt.
const contextCap = 4000

// perCardCap keeps one busy card from spending the whole budget.
const perCardCap = 900

var (
	tagRe   = regexp.MustCompile(`(?s)<(script|style)\b.*?</(script|style)>|<[^>]*>`)
	spaceRe = regexp.MustCompile(`[ \t]*\n[ \t\n]*`)
)

// CardContext renders an account's chosen cards as plain text for the agent.
// Empty when nothing is chosen, which is the normal case.
func CardContext(acc *auth.Account) string {
	if acc == nil {
		return ""
	}
	var b strings.Builder
	for _, id := range acc.HomeCardOrder() {
		body := textOf(cardBody(id))
		if body == "" {
			continue
		}
		if len(body) > perCardCap {
			body = strings.TrimSpace(body[:perCardCap]) + "…"
		}
		entry := "## " + cardTitle(id) + "\n" + body + "\n\n"
		if b.Len()+len(entry) > contextCap {
			break
		}
		b.WriteString(entry)
	}
	if b.Len() == 0 {
		return ""
	}
	return "The reader watches these on their home screen. Use them to answer " +
		"directly rather than fetching the same thing again:\n\n" + strings.TrimSpace(b.String())
}

// cardBody is the rendered content of one card, or "" if it has none.
func cardBody(id string) string {
	for _, c := range Cards {
		if c.ID == id {
			return c.CachedHTML
		}
	}
	return ""
}

func cardTitle(id string) string {
	for _, c := range Cards {
		if c.ID == id {
			return c.Title
		}
	}
	return id
}

// textOf strips markup and collapses the whitespace it leaves behind. Crude on
// purpose: the cards render HTML for people, and a model reads the words.
func textOf(html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	out := tagRe.ReplaceAllString(html, "\n")
	out = strings.ReplaceAll(out, "&nbsp;", " ")
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&#39;", "'")
	out = strings.ReplaceAll(out, "&quot;", `"`)
	out = spaceRe.ReplaceAllString(out, "\n")
	return strings.TrimSpace(out)
}
