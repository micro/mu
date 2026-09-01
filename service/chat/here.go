package chat

// Who is on this instance, on the page about talking to them.
//
// This was a strip on Home: a count you could open to see the names, a bubble
// that slid a chat over the page, and the last thing anybody had said in it.
// Each piece answered the objection to the one before it, and the whole was
// still wrong — it put a room with strangers in it on the screen a person
// opens every day. Nobody is on the other end, and a product that keeps saying
// so is a product bolting multiplayer onto something nobody asked to share.
//
// It is not useless, it was in the wrong place. Somebody on /chat has already
// decided they want to talk to a person; "who is here" is the question that
// page exists to answer, asked of people instead of rooms. So it lives here,
// where finding it is a decision, and Home is one person's screen.
//
// # What did not come with it
//
// The count, the disclosure, the bubble and the last-line-said all went. They
// were there to make a strip on a busy front page bearable, and a list at the
// top of the page you came to for exactly this needs none of it: it is the
// names, or nothing.

import (
	"html"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"mu/internal/auth"
)

// hereShown caps the names. Beyond this it stops being a line and starts being
// a page of handles, and who else has an account is what /users answers.
const hereShown = 24

// whoIsHere is the people on this instance right now, or nothing.
//
// Empty when nobody is — including when it is only you, because a list of one
// naming the reader is the instance telling somebody they are alone, which is
// the sentence this has already been through once and taken out.
//
// People, not programs. The instance's own agent holds a real account and is
// always present in the sense presence means, so it would be a permanently lit
// name that is nobody. Filtered on acc.Agent rather than on the id, because
// the flag is the rule and the id is one instance of it.
func whoIsHere() string {
	var names []string
	for _, id := range auth.OnlineUsers() {
		acc, err := auth.GetAccount(id)
		if err != nil || acc == nil || acc.Agent || acc.Banned {
			continue
		}
		names = append(names, acc.ID)
	}
	if len(names) < 2 {
		return ""
	}
	sort.Strings(names)

	rest := 0
	if len(names) > hereShown {
		rest = len(names) - hereShown
		names = names[:hereShown]
	}

	var b strings.Builder
	b.WriteString(`<div class="here-block"><h3>Online</h3><div class="here-strip">`)
	for _, id := range names {
		b.WriteString(`<a class="here-who here-on" href="/@` + url.PathEscape(id) + `">` +
			`<span class="here-dot"></span>@` + html.EscapeString(id) + `</a>`)
	}
	if rest > 0 {
		b.WriteString(`<span class="here-who">and ` + strconv.Itoa(rest) + ` more</span>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}
