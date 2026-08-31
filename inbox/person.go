package inbox

// /@somebody is the conversation with them.
//
// It was a profile: a name, a tick, a join date, a status box, an apps grid and
// a list of their posts. That is a social network's page, and this is not a
// social network — internal/app/content.go already says so in as many words,
// where Save, Hide and Block were deleted because "those three are the controls
// of a feed… Mu has no feed." A profile is downstream of the same thing.
//
// What survives the deletion is the address. "Everything has an address —
// people, agents, services, conversations" is one of four commitments, and an
// address is worth having in proportion to what it lets you do. A page you look
// at does nothing. A page where you say something is the whole of it.
//
// So /@henrik is what the two of you have said to each other, with the way to
// say the next thing. Not a new screen: the inbox keyed by correspondent
// instead of by conversation, over the same record, rendered by the same
// ConversationView.
//
// # Three consequences, and the middle one is the point
//
// Compose is the empty state. Somebody you have never written to has no
// conversation, so the page offers the blank message — /inbox/new, which is
// already the page for writing one. There is no separate compose screen to
// keep.
//
// Offers, not redirects. It used to redirect straight there, which is the same
// idea taken one step too far: you click @micro expecting @micro, and the
// address bar says /inbox/new with nothing explaining that you were moved. The
// empty state of a thing is that thing, empty — so the page stands, names whose
// it is, and puts one button on it.
//
// It stops being public. What this renders is *your* history with them, so
// there is nothing to show a stranger and nothing for a crawler to take. That
// retires the problem writeLink was built to work around: a public profile
// published every account's mailbox to anybody who opened it, and the fix was
// to name the person and resolve the address inside the send. With no public
// page there is nothing to publish.
//
// And it is uniform. A person, an agent, a service — all of them answer the
// same question at the same kind of address, which is what turns an addressing
// scheme into something that does work rather than a way of spelling URLs.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// PersonHandler serves /@name.
func PersonHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.Trim(strings.TrimPrefix(r.URL.Path, "/@"), "/"))
	if name == "" {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}

	// Signed in, and not as a formality. There is no version of this page for
	// somebody with no account: it renders one person's correspondence, so a
	// stranger is not being refused a view, there is no view to refuse them.
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	them, err := auth.GetAccount(name)
	if err != nil {
		app.NotFound(w, r, "nobody here is called @"+name)
		return
	}

	// Your own page is a page.
	//
	// It redirected to /inbox, on the argument that mail to your own address is
	// just mail so this would be a page of everything you have ever been sent.
	// True while this was only a correspondence — and it is a profile now, so
	// the thing at /@you is you, which is the one identity on this instance you
	// cannot currently look at. Clicking your own name in /users and landing in
	// your inbox is the report that found it.
	you := them.ID == acc.ID

	var convs []thread.Thread
	if !you {
		convs = thread.With(acc.ID, namesFor(them.ID)...)
	}

	title := them.Name
	if strings.TrimSpace(title) == "" {
		title = "@" + them.ID
	}
	handle := "@" + them.ID

	// A profile: who, and whether they are about.
	//
	// It said "since when" too, and that came off — when an account was made
	// says nothing about the person and changes nothing you would do next. What
	// is left is presence, which is about right now.
	//
	// This said "Your conversation with @x" under the name, which described the
	// page rather than the person — right when the page was one exchange, and
	// wrong now: on your own page it would have had to say you were in
	// correspondence with yourself, and on somebody else's it labelled the
	// whole page with the relationship before saying anything about them. The
	// handle is what belongs under a display name, because the display name is
	// the changeable one and the handle is the address.
	var b strings.Builder
	b.WriteString(`<div class="ib-person">`)
	b.WriteString(`<p class="ib-person-sub">` + html.EscapeString(handle) + `</p>`)
	b.WriteString(personFacts(them))
	// New message belongs on a page that already has one.
	//
	// On an empty page it was the fourth thing saying the same thing: the name,
	// "Your conversation with @x", a New message pill, "Nothing between you and
	// @x yet", and a Start a conversation button — five lines for one available
	// action, three of them naming the same person. Reported exactly that way.
	//
	// Where there are conversations it earns its place, because there it means
	// "a different one from these".
	head := `<div class="ib-person-head">` + sendMailTo(handle) + `</div>`

	// Nobody you have spoken to yet is still a page.
	//
	// This redirected to /inbox/new, on the reasoning that the empty state of a
	// conversation is a blank message and there was no sense keeping two compose
	// screens. What that does to a reader is different from what it does on
	// paper: you click @micro expecting to see @micro, and the address bar says
	// /inbox/new. The page you asked for is gone, and nothing explains that you
	// were sent somewhere. A redirect is the wrong shape for an empty state —
	// the empty state of a thing is that thing, empty.
	//
	// So the page stands, says whose it is, and offers the one action.
	if you {
		// Your own page carries no way to write to yourself and no
		// correspondence, because your correspondence is the inbox and this is
		// not a second one. What is left is what a profile is: who you are on
		// this instance, and the way to the settings that change it.
		b.WriteString(`<div class="ib-person-empty">` +
			app.Link("Account settings", "/account") + `</div></div>`)
		app.Respond(w, r, app.Response{
			Title:       title,
			Description: handle + " on this instance",
			HTML:        b.String(),
		})
		return
	}

	if len(convs) == 0 {
		b.WriteString(`<div class="ib-person-empty">` +
			`<p>Nothing between you yet.</p>` + sendMailTo(handle) + `</div></div>`)
		app.Respond(w, r, app.Response{
			Title:       title,
			Description: "Your conversation with " + title,
			HTML:        b.String(),
		})
		return
	}

	// The newest, whole. The rest as a list rather than stacked: one person on
	// several channels is several conversations, and concatenating them would
	// read as one exchange that jumps transport mid-sentence.
	//
	// The reply goes to them, said rather than worked out. See
	// conversationPaneTo — inferring it from who spoke last leaves a
	// conversation you started with no Reply on it at all.
	b.WriteString(head)

	first := convs[0]
	msgs := thread.Messages(acc.ID, first.ID, MessagesShown)
	b.WriteString(conversationPaneTo(acc.ID, &first, msgs,
		len(msgs) >= MessagesShown, true, "", replyAddressFor(them.ID, &first)))
	if len(convs) > 1 {
		b.WriteString(alsoWith(title, convs[1:]))
	}
	b.WriteString(`</div>`)

	app.Respond(w, r, app.Response{
		Title:       title,
		Description: "Your conversation with " + title,
		HTML:        b.String(),
	})
}

// sendMailTo is the way to write to somebody, as an icon with a word.
//
// Mail and chat are different promises — one is answered when they get to it,
// the other is answered now — and "Send a message" named neither. It also read
// as a sentence where a button belongs. So the label says which door it is:
// Send mail, because mail is what /inbox/new writes.
//
// The envelope and the bubble carry the distinction faster than the words do,
// which is the point of having both: the icon separates the two at a glance and
// the word says which is which for anybody who does not read icons.
//
// # No Chat button yet
//
// It belongs here and there is nowhere for it to go. service/chat has topic
// rooms and no concept of a room that is these two people — see task #96 — so
// the only thing this could link to today is a room id assembled from two
// account names, which anybody could guess and open. A private conversation
// with a URL-guessable address is not a private conversation, and a button that
// lies about that is worse than a button that is not there yet.
func sendMailTo(handle string) string {
	to := html.EscapeString(url.QueryEscape(handle))
	return `<a class="btn ib-act" href="/inbox/new?to=` + to + `">` +
		iconMail + `Send mail</a>`
}

// iconMail is an envelope, drawn rather than fetched.
//
// Inline SVG because it is two lines of markup and the alternative is a request
// for an image that has to exist, be cached and be found again by whoever moves
// it. currentColor so it takes the button's own colour rather than needing one
// of its own for every place a button appears.
const iconMail = `<svg class="ib-act-icon" viewBox="0 0 16 16" aria-hidden="true">` +
	`<rect x="1.5" y="3.5" width="13" height="9" rx="1.5" fill="none" ` +
	`stroke="currentColor" stroke-width="1.3"/>` +
	`<path d="M2 4.5l6 4 6-4" fill="none" stroke="currentColor" ` +
	`stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/></svg>`

// personFacts is the two things a profile says about somebody that are true
// whether or not you have ever spoken: since when, and whether they are about.
//
// Both are facts this instance already held and neither was on any page. The
// join date is what turns a handle into a person with a history here — "@sara,
// since March" is somebody who was here before you, and that is most of what
// you want to know about a name you do not recognise. Presence is the other
// half, and it is the half that decides whether writing to them is a message
// or a conversation.
//
// Above the way to write to them on purpose. Knowing they are here changes what
// you do next; finding out afterwards does not.
//
// Silent about presence rather than saying "Offline". Most people are, most of
// the time, so the word would be on this page nearly always and would be the
// loudest thing on it — the same argument that keeps Home's brief quiet on a
// quiet day. Here is worth saying; not here is the default.
func personFacts(them *auth.Account) string {
	// No join date.
	//
	// It was the first fact this page said about somebody and it is the least
	// useful one: when an account was made says nothing about the person, does
	// not change what you would do next, and on an instance a year old it says
	// the same thing about nearly everybody. It was reached for because a
	// profile felt like it ought to have a fact under the name, which is the
	// wrong reason to put anything on a page.
	//
	// What is left is presence, which is about right now and is the one thing
	// here that changes what you do.
	var parts []string
	for _, id := range auth.OnlineUsers() {
		if id == them.ID {
			parts = append(parts, `<p class="ib-person-fact ib-person-live">`+
				`<span class="here-dot"></span>Here now</p>`)
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "")
}

// replyAddressFor is where a reply to this person goes, on the channel this
// conversation is on.
//
// A text is answered with a text and mail with mail — the transport is a
// property of the conversation, not of the person — so the thread's own key is
// the answer wherever it has one. Their address is the fallback, which is the
// mail case and the one a conversation started from this page will be.
func replyAddressFor(accountID string, t *thread.Thread) string {
	if k := strings.TrimSpace(t.Key); k != "" && t.Client != mailClient {
		return k
	}
	if addr, ok := addressOfPerson("@" + accountID); ok {
		return addr
	}
	return ""
}

// alsoWith is the other conversations with the same person.
//
// Named for what they are rather than "Threads". Somebody who has written to
// you by mail and by WhatsApp has two of these, and the useful thing to say is
// which channel each one is, because that is the difference a reader is
// actually navigating by.
func alsoWith(who string, rest []thread.Thread) string {
	var b strings.Builder
	b.WriteString(`<div class="ib-person-also"><h3>Also with ` +
		html.EscapeString(who) + `</h3><ul class="ib-person-list">`)
	for i := range rest {
		t := rest[i]
		subject := strings.TrimSpace(t.Subject)
		if subject == "" {
			subject = "Untitled"
		}
		b.WriteString(`<li><a href="/inbox?id=` + html.EscapeString(url.QueryEscape(t.ID)) + `">` +
			html.EscapeString(subject) + `</a>` +
			`<span class="pill">` + html.EscapeString(app.ClientName(t.Client)) + `</span>` +
			`<span class="ib-person-when">` + html.EscapeString(app.TimeAgo(t.Updated)) +
			`</span></li>`)
	}
	b.WriteString(`</ul></div>`)
	return b.String()
}

// namesFor is every way this instance's record might name one of its accounts.
//
// The id, the address mail reaches them at, and their display name — a
// conversation records whichever the channel knew, and a page that matched only
// one of them would be empty for somebody you have plainly been talking to.
//
// The address through the same resolution the To box uses, so the two agree by
// construction: if writing to @henrik reaches an address, that address is one
// this page looks for.
func namesFor(accountID string) []string {
	out := []string{accountID}
	if addr, ok := addressOfPerson("@" + accountID); ok {
		out = append(out, addr)
	}
	if acc, err := auth.GetAccount(accountID); err == nil {
		if n := strings.TrimSpace(acc.Name); n != "" {
			out = append(out, n)
		}
	}
	return out
}
