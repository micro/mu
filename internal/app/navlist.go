package app

// The sidebar is four nouns.
//
// It grew a nested list under Inbox — one entry per agent that had mail, with
// an unread count on each — on the argument that a mail client puts your
// mailboxes in the rail rather than making you open a page to find them. That
// is true of a mail client. It is not true of this, where the rail already sits
// beside Home, Agents, Tools, Services and Account, and a fifth of them
// unfolding into a sub-list of its own makes the sidebar a table of contents
// rather than a place to stand.
//
// The mailboxes are on /inbox, at the top of the list they filter, which is
// where a filter belongs — beside the thing it filters, not in the furniture
// two levels up. The rail says Inbox.
//
// NavItem stays because /inbox uses it to draw that strip.

import "net/url"

// NavItem is one thing in a list: what to call it, and where it goes.
type NavItem struct {
	Label string
	Href  string
	// Key marks which item is the current one, matched against the request path.
	// Empty means never current.
	Key string
	// Badge is a short count shown against the item — unread mail, for the
	// mailboxes. Empty for nothing to say, which is the ordinary case: a badge
	// reading zero is a badge that has stopped meaning anything.
	Badge string
}

// navPath is the path the rail should highlight against, from the request.
func navPath(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Path
	}
	return raw
}
