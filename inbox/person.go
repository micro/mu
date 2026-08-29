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

	// Yourself is the one address that cannot be a conversation. Mail to your
	// own address is just mail — service/mail settles that — so this would be a
	// page of everything you have ever been sent, which is the inbox.
	if them.ID == acc.ID {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}

	convs := thread.With(acc.ID, namesFor(them.ID)...)

	title := them.Name
	if strings.TrimSpace(title) == "" {
		title = "@" + them.ID
	}
	handle := "@" + them.ID

	// What this page is, said once, under the name.
	//
	// It said nothing. The argument was that the shell already prints the name
	// and the conversation states the parties, so a heading would be a third
	// copy of somebody's name — which was true and answered the wrong question.
	// Reported by somebody on their own instance: they clicked a username on the
	// blog, landed on an exchange, and did not know what the page was. A name at
	// the top says who; nothing said what, and "an exchange with no label" reads
	// as a page you have arrived at by accident.
	//
	// So it names the relationship rather than the person again. One line.
	var b strings.Builder
	b.WriteString(`<div class="ib-person">`)
	b.WriteString(`<p class="ib-person-sub">Your conversation with ` +
		html.EscapeString(handle) + `</p>`)
	// New message belongs on a page that already has one.
	//
	// On an empty page it was the fourth thing saying the same thing: the name,
	// "Your conversation with @x", a New message pill, "Nothing between you and
	// @x yet", and a Start a conversation button — five lines for one available
	// action, three of them naming the same person. Reported exactly that way.
	//
	// Where there are conversations it earns its place, because there it means
	// "a different one from these".
	head := `<div class="ib-person-head"><a class="pill" href="/inbox/new?to=` +
		html.EscapeString(url.QueryEscape(handle)) + `">New message</a></div>`

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
	if len(convs) == 0 {
		b.WriteString(`<div class="ib-person-empty">` +
			`<p>Nothing here yet.</p>` +
			`<a class="btn" href="/inbox/new?to=` +
			html.EscapeString(url.QueryEscape(handle)) + `">Start a conversation</a>` +
			`</div></div>`)
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
