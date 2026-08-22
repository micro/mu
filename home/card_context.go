package home

// The cards, as something an agent can read.
//
// The home screen shows what this instance knows right now — headlines, market
// movers, what is on today. That is exactly what a question about any of it
// would otherwise be answered by fetching, one tool call at a time, after the
// model has worked out which tool to call. So the cards go with the question.
//
// Always, not on a toggle. It was opt-in behind a "use live context" checkbox
// on the argument that context costs tokens — which is true and is not the
// reader's problem to solve. They are looking at the cards; an answer that
// ignores what is on the screen in front of them is the wrong answer.
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

// CardContext renders the home cards as plain text for the agent. The same
// cards, in the same order, that the reader is looking at — there is no
// per-account selection to consult, which is why the two can no longer
// disagree about what "what I am looking at" means.
func CardContext(acc *auth.Account) string {
	if acc == nil {
		return ""
	}
	order := make([]string, 0, len(Cards))
	for _, c := range Cards {
		order = append(order, c.ID)
	}

	var b strings.Builder
	for _, id := range order {
		body := textOf(cardHTML(id))
		if body == "" {
			continue
		}
		if len(body) > perCardCap {
			body = strings.TrimSpace(body[:perCardCap]) + "…"
		}
		entry := "## " + cardName(id) + "\n" + body + "\n\n"
		if b.Len()+len(entry) > contextCap {
			break
		}
		b.WriteString(entry)
	}
	if b.Len() == 0 {
		return ""
	}
	return "This is what the reader's home screen is showing right now. Use it to answer " +
		"directly rather than fetching the same thing again:\n\n" + strings.TrimSpace(b.String())
}

// cardHTML is the rendered content of one card, or "" if it has none. Named
// for what it returns rather than for the card, because cardBody is the body as
// the page shows it — with the way through to the service on the end of it.
func cardHTML(id string) string {
	for _, c := range Cards {
		if c.ID == id {
			return c.CachedHTML
		}
	}
	return ""
}

func cardName(id string) string {
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
