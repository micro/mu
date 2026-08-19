package app

// The small shapes every page draws, defined once.
//
// Sixty-six packages in this repository ship a <style> block of their own, and
// between them they use nine greys, nine corner radii, and four class names for
// the same hairline pill — .ib-tag, .els-where, .th-tool, .peek-where. That is
// not a taste problem that more care would have avoided. A page has no way to
// say "the usual thing", so every page invents one, and the site cannot look
// like itself.
//
// mu.css already carries the tokens — --text-muted, --card-border,
// --border-radius, a spacing scale — and the pages that hand-roll a stylesheet
// use none of them: /inbox used zero and sixty-two literal hex values. The
// tokens were never the missing piece. What was missing is a component small
// enough that reaching for it is easier than typing `border:1px solid #eee`.
//
// The rule for what belongs here: a shape that appears on more than one page
// and carries no meaning of its own. A pill, a row of controls. Anything true
// of one page only stays with that page — this is not a place to collect
// markup, it is a place to stop redrawing it.

import (
	"html"
	"strings"
)

// Pill is a small hairline label: which channel a message came in on, what kind
// of thing a row is, how many there are.
func Pill(label string) string {
	return `<span class="pill">` + html.EscapeString(label) + `</span>`
}

// PillLink is a pill you can click — a filter, a mailbox, a switcher — with the
// current one filled rather than outlined.
func PillLink(label, href string, on bool) string {
	cls := "pill pill-link"
	if on {
		cls += " on"
	}
	return `<a class="` + cls + `" href="` + href + `">` + html.EscapeString(label) + `</a>`
}

// Actions is the bar over something you are reading: where you came from on the
// left, what you can do to it on the right.
//
// The alternative is what /inbox had — a back link, then a Mark unread button
// on its own line, then Delete on another, each a loose form stacked above the
// conversation they act on. A reader cannot see those as one group, because on
// the page they are not one. Controls are given as rendered HTML because some
// of them are forms with a token in; back is a link.
func Actions(back string, controls ...string) string {
	var b strings.Builder
	b.WriteString(`<div class="actions"><div class="actions-back">` + back + `</div>`)
	if len(controls) > 0 {
		b.WriteString(`<div class="actions-set">`)
		for _, c := range controls {
			b.WriteString(c)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
