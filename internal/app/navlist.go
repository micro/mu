package app

// A sidebar that lists your things, not just the places they live.
//
// Inbox, Agents, Tools and Services were four links. That is a set of pages,
// and the thing being built is a client — so Inbox and Agents are headings with
// what you have underneath them, the way a mail client puts your mailboxes in
// the rail rather than making you open a page to find out what they are.
//
// Tools and Services stay single entries, and the difference is whose the list
// is. Your mailboxes and your agents are yours, there are a handful, and they
// are what you navigate between. The catalogue is the instance's, it runs to
// dozens, and it is something you browse rather than switch between — putting
// twenty-five services in the rail would be a table of contents, not navigation.
//
// The items come from function variables rather than imports: this file is the
// shell every page is drawn in, and it must not depend on the packages it draws
// links to. internal/server fills them in — see hooks.go.

import (
	"html"
	"net/url"
	"strings"
)

// NavItem is one thing under a heading: what to call it, and where it goes.
type NavItem struct {
	Label string
	Href  string
	// Key marks which item is the current one, matched against the request path
	// so the rail can show where you are. Empty means never current.
	Key string
	// Badge is a short count shown against the item — unread mail, for the
	// mailboxes. Empty for nothing to say, which is the ordinary case: a badge
	// reading zero is a badge that has stopped meaning anything.
	Badge string
}

// Wired at boot to whoever owns each list. Nil is a build without that package,
// and reads as "nothing to list", which draws the heading alone.
var (
	// NavMailboxes is the account's mailboxes, one per agent that has mail.
	NavMailboxes func(account string) []NavItem
	// NavAgents is the account's agents.
	NavAgents func(account string) []NavItem
	// NavServices is the catalogue, for somebody signed in.
	NavServices func(account string) []NavItem
)

// navChildren renders the items under one heading, or nothing at all.
//
// Nothing at all is the common case for a new account and it is the right one:
// a heading with an empty list under it is a promise the page has not kept, and
// the heading is still a link to the page that explains itself.
func navChildren(items []NavItem, current string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="nav-kids">`)
	for _, it := range items {
		cls := "nav-kid"
		if it.Key != "" && it.Key == current {
			cls += " on"
		}
		badge := ""
		if it.Badge != "" {
			badge = `<span class="nav-badge">` + html.EscapeString(it.Badge) + `</span>`
		}
		b.WriteString(`<a class="` + cls + `" href="` + html.EscapeString(it.Href) + `">` +
			`<span class="label">` + html.EscapeString(it.Label) + `</span>` + badge + `</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// navMailboxes is the Inbox heading and the boxes under it.
func navMailboxes(account, path string) string {
	if account == "" || NavMailboxes == nil {
		return ""
	}
	return navChildren(NavMailboxes(account), strings.TrimPrefix(path, "/inbox/"))
}

// navAgents is the Agents heading and the roster under it.
func navAgents(account, path string) string {
	if account == "" || NavAgents == nil {
		return ""
	}
	return navChildren(NavAgents(account), strings.TrimPrefix(path, "/agent/"))
}

// navServices is the Services heading and the catalogue under it.
//
// This was the odd one out and it read as unfinished: Inbox and Agents had
// their things underneath them and Services had a separate group further down
// the rail, under a second heading also called Services, holding only what
// somebody had pinned. Two headings with one name, one of them usually empty.
//
// So the catalogue goes where it says it is. It is longer than the other two —
// this is the instance's list rather than yours, and it runs to dozens — which
// is exactly what nesting is for: indented under a heading it reads as one
// group you can skim past, where twenty-five entries flush with Home and Inbox
// would be a table of contents.
//
// Signed in only, like the others. A first-time visitor gets the four nouns and
// nothing else, which is the whole of what the rail is for on the way in.
//
// Matched on the path's first segment, because a service's page is its name and
// its sub-pages are underneath: /news/tech should light News.
func navServices(account, path string) string {
	if account == "" || NavServices == nil {
		return ""
	}
	return navChildren(NavServices(account), firstSegment(path))
}

// firstSegment is the leading path element, without its slash.
func firstSegment(path string) string {
	rest := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// navPath is the path the rail should highlight against, from the request.
func navPath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return raw
}
