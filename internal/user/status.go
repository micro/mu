package user

// What somebody says they are doing, and how to write to them.
//
// A profile with a name and a join date on it is a record. Nobody visits a
// record twice. The two things that make /@somebody worth opening are a line
// saying what they are up to and a way to say something back, and it had
// neither — the status field had been taken out with the stream it used to
// feed, and the message link pointed at a compose screen that no longer exists.
//
// # Writing to a person is writing to a person
//
// Not to their agent, and no new channel for it. service/mail already draws the
// line and states it: "untagged mail to your own address is just mail — every
// newsletter would otherwise start a run." So asim@ is the person and
// asim+research@ is their agent, the addressing scheme has always said which,
// and Write here is /inbox/new pointed at the first one. It arrives in their
// inbox, it threads, they reply, and nothing wakes.
//
// That is the whole person-to-person surface, and it is the product's own claim
// pointed sideways: an address is the smallest interface there is.

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// statusLimit bounds a status. A sentence about what you are doing, not a post.
const statusLimit = 140

// AddressFor is where mail for an account arrives, filled in by the server.
//
// A hook because this package is internal/ and the mail service is product —
// see the layering rule. One string is not a reason to invert that, and
// inbox.Address is the same hook for the same reason.
var AddressFor func(accountID string) string

// addressOf is an account's address, or "" on an instance with no mail.
func addressOf(accountID string) string {
	if AddressFor == nil {
		return ""
	}
	return strings.TrimSpace(AddressFor(accountID))
}

// Status is what an account says it is doing, and when it said so.
func Status(accountID string) (text string, at time.Time) {
	profileMutex.RLock()
	defer profileMutex.RUnlock()
	if p := profiles[strings.ToLower(accountID)]; p != nil {
		return p.Status, p.UpdatedAt
	}
	return "", time.Time{}
}

// SetStatus records what somebody is doing. An empty status clears it.
func SetStatus(accountID, text string) {
	id := strings.ToLower(strings.TrimSpace(accountID))
	if id == "" {
		return
	}
	text = strings.TrimSpace(text)
	if len([]rune(text)) > statusLimit {
		text = strings.TrimSpace(string([]rune(text)[:statusLimit]))
	}

	profileMutex.Lock()
	p := profiles[id]
	if p == nil {
		p = &Profile{UserID: id}
		profiles[id] = p
	}
	p.Status, p.UpdatedAt = text, time.Now()
	b, err := json.Marshal(profiles)
	profileMutex.Unlock()

	// Written on every change rather than on a timer. Statuses are set by hand
	// and rarely — the file was being loaded at init and saved by nothing at
	// all, which is why the field it held had no way of surviving a restart and
	// eventually stopped being set.
	if err == nil {
		data.SaveFile("profiles.json", string(b)) //nolint:errcheck
	}
}

// StatusHandler serves the POST from the box on your own profile.
//
// At /profile/status and not /status. /status is this instance's health page
// and has been for a long time; registering a second handler on it did not
// fail a test or a build — net/http panics at boot on a duplicate pattern, so
// the whole server went down on the next deploy with everything green behind
// it. A route is not checked by anything until it is registered.
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !auth.ValidCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}
	SetStatus(sess.Account, r.FormValue("status"))
	http.Redirect(w, r, "/@"+sess.Account, http.StatusSeeOther)
}

// statusBlock is the status on somebody's profile, and the box to set it on
// your own.
func statusBlock(accountID string, own bool, csrf string) string {
	text, at := Status(accountID)

	if !own {
		if text == "" {
			return ""
		}
		return `<p class="pf-status">` + html.EscapeString(text) +
			`<span class="pf-status-when">` + html.EscapeString(app.TimeAgo(at)) + `</span></p>`
	}

	// Your own: the same line, editable in place. No separate settings page for
	// one field — the place you change what your profile says is your profile.
	return `<form class="pf-status-form" method="post" action="/profile/status">` +
		`<input type="hidden" name="csrf_token" value="` + html.EscapeString(csrf) + `">` +
		`<input class="pf-status-in" type="text" name="status" maxlength="` +
		strconv.Itoa(statusLimit) + `" placeholder="What are you up to?" value="` +
		html.EscapeString(text) + `">` +
		`<button type="submit" class="pill">Save</button></form>`
}

// writeLink is the way to say something to somebody, or nothing on an instance
// that cannot carry it.
//
// Their address, not their agent's. See the package comment.
func writeLink(accountID string) string {
	addr := addressOf(accountID)
	if addr == "" {
		return ""
	}
	return `<p class="pf-write"><a class="lcta lcta-second" href="/inbox/new?to=` +
		html.EscapeString(url.QueryEscape(addr)) + `">Write</a>` +
		`<code class="pf-addr">` + html.EscapeString(addr) + `</code></p>`
}
