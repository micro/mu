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
// # Everyone, not only the online ones
//
// Online-only was the first version of this and it drew nothing almost all the
// time, because the usual number of other people on a personal instance at any
// given second is zero. A strip that is empty on the day you look at it teaches
// you it is broken. So: everyone with an account, the online ones first and
// lit, the rest with when they were last here — which is the difference between
// "nobody is about" and "this is a server with four people on it, two of whom
// were here today".
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
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// hereShown caps the strip.
//
// It is one line at the width Home gives it and stops being scannable long
// before it stops fitting. Every online person is inside this bound in any
// instance a person runs; the cap only ever bites on the offline tail, which is
// sorted so the useful end of it survives.
const hereShown = 12

// hereStale is how long ago counts as worth printing.
//
// A year-old last-seen against a name is not "when they were here", it is a
// fact about an account that has been abandoned. Past this the name still
// draws — they are on the instance and that is the point — without a number
// that makes the strip look like a graveyard.
const hereStale = 90 * 24 * time.Hour

// person is one name in the strip.
type person struct {
	id     string
	online bool
	seen   time.Time
	known  bool
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
	// there is nobody to talk to, is the part that had to go.
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
			`<span class="here-dot"></span>@` + html.EscapeString(p.id)
		if when := hereWhen(p); when != "" {
			out += `<span class="here-when">` + html.EscapeString(when) + `</span>`
		}
		out += `</a>`
	}
	if others > 0 {
		out += app.Link("Chat", "/chat")
	}
	return out + `</div></div>`
}

// hereWhen is how long ago somebody was last here, or nothing.
//
// Nothing for the online — the lit dot says it, and "1 min ago" beside a live
// dot is the same fact twice. Nothing for an account presence has never seen
// either: after a restart that is everybody, and a strip of names each labelled
// with the same wrong time is worse than a strip of names.
func hereWhen(p person) string {
	if p.online || !p.known || p.seen.IsZero() {
		return ""
	}
	if time.Since(p.seen) > hereStale {
		return ""
	}
	return app.TimeAgo(p.seen)
}

// roster is everybody on the instance, in the order the strip reads them.
//
// Online first and alphabetical among themselves, so the lit end of the strip
// does not reshuffle while somebody is looking at it. Then everybody else by
// how recently they were here, which is the order that puts the people this
// instance actually has at the front of the tail. Accounts presence has never
// heard of go last, alphabetically — after a restart that is all of them, and
// the strip has to have some order on the day it knows nothing.
func roster() []person {
	live := map[string]bool{}
	for _, id := range auth.OnlineUsers() {
		live[id] = true
	}

	var people []person
	for _, acc := range auth.AllAccounts() {
		if acc == nil || acc.ID == "" {
			continue
		}
		// People, not programs.
		//
		// The instance's own agent holds a real account — EnsureMicro creates
		// it on every install as soon as there is a human admin — so it turned
		// up in the strip as @micro, listed among the people who use this
		// server. It is the server.
		//
		// Filtered on acc.Agent rather than on the id, because the id is one
		// instance of the rule and the flag is the rule: "a person and a
		// program are not the same caller" is what auth.Account.Agent was added
		// to say, and anything else that gets an account for the same reason is
		// out of this strip without a second edit here.
		if acc.Agent {
			continue
		}
		seen, known := auth.LastSeen(acc.ID)
		people = append(people, person{
			id: acc.ID, online: live[acc.ID], seen: seen, known: known,
		})
	}

	sort.Slice(people, func(i, j int) bool {
		a, b := people[i], people[j]
		if a.online != b.online {
			return a.online
		}
		if a.online {
			return a.id < b.id
		}
		if ak, bk := a.known && !a.seen.IsZero(), b.known && !b.seen.IsZero(); ak != bk {
			return ak
		} else if ak {
			return a.seen.After(b.seen)
		}
		return a.id < b.id
	})

	if len(people) > hereShown {
		people = people[:hereShown]
	}
	return people
}
