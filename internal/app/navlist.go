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

// navPath is the path the rail should highlight against, from the request.
func navPath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return raw
}
