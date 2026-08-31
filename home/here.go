package home

// Who is on this instance, under the box.
//
// # Online, not Here
//
// "Here" was meant as here-on-this-page and reads as here-in-the-room, which is
// a claim this cannot make: presence is a three-minute window over the whole
// instance, so somebody reading their mail in another tab is on the strip. The
// dot means they are using this server right now, and Online is the word for
// that — the one every product this could be compared to already uses, which is
// worth more than a word we would have to teach.
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
	"strconv"

	"mu/internal/auth"
)

// hereShown caps the names, not the count.
//
// The revealed row is one line at the width Home gives it and stops being
// scannable long before it stops fitting. A number this size is only ever
// reached by an instance with a real crowd on it, and there the cap is the
// difference between a line and a paragraph.
//
// It was applied in roster, which was fine while the strip was the names and
// wrong the moment the strip became a number: twelve of forty people here, and
// the count would have said twelve. See names.
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
func hereStrip(present []person, viewerID string) string {
	if len(present) == 0 {
		return ""
	}

	// The way to them, and only when there is a them. See the note above about
	// the two useless sentences: an invitation to go and talk, on a day when
	// there is nobody to talk to, is the part that had to go. It is drawn under
	// the names rather than at the end of them — see below.
	others := 0
	for _, p := range present {
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
	out := `<div class="here-block">` + sectionRule("Online") + `<div class="here-line">`

	// How many, not who.
	//
	// It listed every name, every time, and a row of usernames is a thing you
	// read once and then never again: on a busy instance it is a wall, and on a
	// quiet one it is the same four names every day. The number is the part
	// that changes and the part somebody actually wants — is anyone about — and
	// it says it in two words instead of a line of handles.
	//
	// Who they are is one click away, because that is a question somebody asks
	// rather than one the page should keep answering.
	//
	// # Except when it is one
	//
	// Alone, the count is the name: "1 person" is longer than "@you", says
	// less, and summarising a list of one is how a page ends up telling
	// somebody they are by themselves. The strip still draws — the lit dot
	// beside your own name is the instance saying it knows you are here — and
	// it still says nothing about being alone. See the note at the top.
	if len(present) == 1 {
		out += names(present)
	} else {
		out += `<button type="button" class="here-count" aria-expanded="false" ` +
			`aria-controls="here-who" onclick="muHereWho(this)">` +
			strconv.Itoa(len(present)) + ` people</button>`
	}

	// The way in, beside the count rather than under it.
	//
	// Only when there is somebody on the other end of it: an invitation to go
	// and talk, on a day when there is nobody to talk to, is the part that had
	// to go. A bubble rather than words — see openChat. Presence is on this
	// page, so the conversation is too: seeing that somebody is here and then
	// navigating away from the page that told you is the friction the panel
	// removes. See panelHTML.
	if others > 0 {
		out += openChat()
	}
	out += `</div>`

	// The names, when asked for.
	if len(present) > 1 {
		out += `<div class="here-strip" id="here-who" hidden>` + names(present) + `</div>`
	}

	// And what was last said, when the room is live. Nothing when it is not,
	// which is most instances most of the time — a chat that says it is
	// happening when it is not is worse than one that says nothing.
	if others > 0 {
		out += latest()
	}
	return out + `</div>`
}

// names is the roster as links, lit, up to the cap.
//
// The cap is on the names and not on the roster, which is the whole of it now
// that there is a count: roster used to truncate, so on an instance with forty
// people here the number would have said twelve. A cap that silently changes a
// number is worse than one that hides a name, and the remainder is stated
// rather than dropped.
func names(present []person) string {
	rest := 0
	if len(present) > hereShown {
		rest = len(present) - hereShown
		present = present[:hereShown]
	}
	var out string
	for _, p := range present {
		cls := "here-who"
		if p.online {
			cls += " here-on"
		}
		out += `<a class="` + cls + `" href="/@` + url.PathEscape(p.id) + `">` +
			`<span class="here-dot"></span>@` + html.EscapeString(p.id) + `</a>`
	}
	if rest > 0 {
		out += `<span class="here-who">and ` + strconv.Itoa(rest) + ` more</span>`
	}
	return out
}

// roster is who is on this instance right now — all of them.
//
// Present only. It returned every account for one commit, sorted online-first
// with a last-seen against the rest, and on an instance with a real signup list
// that is twelve strangers under a heading saying HERE — none of them here, in
// alphabetical order because presence had never heard of any of them. Who else
// has an account is a real question and /users is the page for it.
//
// Uncapped: the cap belongs to the names, which is a display decision, and not
// to this, which is now also the count. See hereShown.
//
// Alphabetical, so the strip does not reshuffle under somebody reading it. The
// alternative was the reader first, which puts the least informative name in
// the most valuable position.
func roster() []person {
	var present []person
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
		present = append(present, person{id: acc.ID, online: true})
	}

	sort.Slice(present, func(i, j int) bool { return present[i].id < present[j].id })
	return present
}
