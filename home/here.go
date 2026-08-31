package home

// Who is on this instance, under the box.
//
// Home had no people on it. The brief is about your day, the inbox is what
// arrived, the agents are yours and the cards are the world — four blocks and
// not one of them says that anybody else uses this server. On a thing whose
// whole premise is that it is a place rather than a page, that is the gap, and
// it is the one somebody notices as "there is no way to interact".
//
// # Not an activity feed
//
// The obvious answer was the last few things that happened anywhere — a post,
// a comment, someone joining — newest first. It reads well and it is the wrong
// block for this page: the Blog and Social cards two inches to the right are
// already that, and a feed here would restate them in a smaller font. What is
// missing is not events. It is people.
//
// # Only the ones who are here
//
// It listed everybody with an account for one commit, online first and lit, the
// rest with when they were last seen — on the argument that a strip which is
// empty on the day you look at it teaches you it is broken. That reasoning came
// from a one-person instance. On micro.mu it produced twelve strangers under a
// heading saying HERE, none of them here, alphabetical because presence had
// never heard of any of them, and the reader is the only name on it that means
// anything.
//
// So: who is actually here. It is never empty — you are on it, because you are
// reading it — and every name on it is a name the dot is true about. Who else
// has an account is a different question and /users is the page that answers
// it.
//
// # And it draws when you are alone
//
// Showing your own name to an audience of you is not much, and it is not
// nothing: it is the instance saying it knows you are here. What it must not do
// is say so in words. "Just you here" over a link to the chat shipped once and
// came straight back out — it told somebody they were alone and then invited
// them to go and talk about it, which is two useless sentences on the screen
// they see most often. A lit dot next to your own name makes the same statement
// and does not editorialise. The way through to the chat appears when there is
// somebody on the other end of it.

import (
	"html"
	"net/url"
	"sort"

	"mu/internal/app"
	"mu/internal/auth"
)

// hereShown caps the strip.
//
// It is one line at the width Home gives it and stops being scannable long
// before it stops fitting. A number this size is only ever reached by an
// instance with a real crowd on it, and there the cap is the difference
// between a line and a paragraph.
const hereShown = 12

// person is one name in the strip.
//
// online is on the struct rather than implied by being in the list, because
// the list is built from two sources — who has an account and who is present —
// and collapsing that into "everybody here is here" is how a name ends up lit
// for a reason nobody can point at.
type person struct {
	id     string
	online bool
}

// hereHTML is the strip: who is here, who else there is, and the way to them.
//
// Empty for a signed-out reader. Who uses a server is not a fact the server
// hands to anybody who asks for the front page — the strip names accounts, and
// on an instance that takes signups that would be a directory of its users,
// published, to anyone. Signed in you are one of them.
func hereHTML(viewerID string) string {
	if viewerID == "" {
		return ""
	}
	return hereStrip(roster(), viewerID)
}

// hereStrip draws a roster that has already been decided.
//
// Split from hereHTML because roster reads two package-level maps in auth —
// every account on the instance, and everybody presence has heard of — which in
// a test binary is every account every other test in this package created, in
// whatever presence state they left behind. Nothing that reads those can be
// asked "what do you do when I am the only one here", which is the case this
// block exists to get right. Given a list, this is a pure function of it.
func hereStrip(people []person, viewerID string) string {
	if len(people) == 0 {
		return ""
	}

	// The way to them, and only when there is a them. See the note above about
	// the two useless sentences: an invitation to go and talk, on a day when
	// there is nobody to talk to, is the part that had to go. It is drawn under
	// the names rather than at the end of them — see below.
	others := 0
	for _, p := range people {
		if p.online && p.id != viewerID {
			others++
		}
	}

	// One element, not two.
	//
	// #home-cards is a grid above 1024px and its children are placed into it in
	// turn, so a heading and a strip returned as siblings went into the two
	// tracks: HERE at the top of the rail and the names in the first cell of
	// the services column, on the same line, as if they were two blocks. The
	// wrapper is what gets the span.
	out := `<div class="here-block">` + sectionRule("Here") + `<div class="here-strip">`
	for _, p := range people {
		cls := "here-who"
		if p.online {
			cls += " here-on"
		}
		out += `<a class="` + cls + `" href="/@` + url.PathEscape(p.id) + `">` +
			`<span class="here-dot"></span>@` + html.EscapeString(p.id) + `</a>`
	}
	out += `</div>`
	// Its own line under the names, not the end of the row.
	//
	// In the row it was a link that appeared and disappeared as people came and
	// went, moving everything before it; and it is a different kind of thing
	// from the names beside it — they are people, this is a door.
	// "Go to chat", the same as the two blocks under it say "Go to inbox" and
	// "Go to agents". One word was doing a different job from its neighbours —
	// they name a destination and this named a subject.
	//
	// It becomes "Open chat" when there is a panel to open rather than a page
	// to go to. Until then the label would be describing something that does
	// not happen.
	if others > 0 {
		out += app.Link("Go to chat", "/chat")
	}
	return out + `</div>`
}

// roster is who is on this instance right now.
//
// Present only. It returned every account for one commit, sorted online-first
// with a last-seen against the rest, and on an instance with a real signup list
// that is twelve strangers under a heading saying HERE — none of them here, in
// alphabetical order because presence had never heard of any of them. Who else
// has an account is a real question and /users is the page for it.
//
// Alphabetical, so the strip does not reshuffle under somebody reading it. The
// alternative was the reader first, which puts the least informative name in
// the most valuable position.
func roster() []person {
	var people []person
	for _, id := range auth.OnlineUsers() {
		acc, err := auth.GetAccount(id)
		if err != nil || acc == nil {
			// Presence knows a name the account store does not. A deleted
			// account keeps its entry in the presence map until the window
			// closes, and a name with no account behind it is a dead link.
			continue
		}
		// People, not programs.
		//
		// The instance's own agent holds a real account — EnsureMicro creates it
		// on every install as soon as there is a human admin — so it turned up
		// in the strip as @micro, listed among the people who use this server.
		// It is the server, and it is always present in the sense presence
		// means, so it would be a permanently lit name that is nobody.
		//
		// Filtered on acc.Agent rather than on the id, because the id is one
		// instance of the rule and the flag is the rule: "a person and a program
		// are not the same caller" is what auth.Account.Agent was added to say,
		// and anything else that gets an account for the same reason is out of
		// this strip without a second edit here.
		if acc.Agent {
			continue
		}
		people = append(people, person{id: acc.ID, online: true})
	}

	sort.Slice(people, func(i, j int) bool { return people[i].id < people[j].id })
	if len(people) > hereShown {
		people = people[:hereShown]
	}
	return people
}
