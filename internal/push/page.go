package push

// Turning notifications on, and the two endpoints behind the button.
//
// The whole flow is three steps and every one of them can be refused, which is
// why it is a button rather than something done on your behalf: the browser
// asks the person, the push service issues a subscription, and this records it.
// Asking for permission unprompted is the single most reliable way to have it
// denied permanently, so nothing here runs until somebody taps.

import (
	"encoding/json"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// SubscribeHandler serves POST /push/subscribe, /push/unsubscribe and
// /push/test.
func SubscribeHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	// A device saying it woke up holding one.
	//
	// The record ended at "the push service accepted it", which is three
	// quarters of an answer. A notification FCM takes and the handset never
	// shows is indistinguishable, from here, from one that was never sent —
	// the server cannot see a service worker. So the service worker says so,
	// including when it woke up and could not render anything, which is the
	// case that used to return silently and leave nothing anywhere.
	//
	// No CSRF: this is posted by a service worker that may be running with no
	// page open, it carries a session, and the worst a forged one can do is
	// mark a notification the account already received as received.
	//
	// # Which has to be checked before the CSRF check, not after it
	//
	// It was after. The paragraph above described an exemption the code never
	// reached, because StrictCSRF ran first and answered 403 to every receipt a
	// service worker ever posted — a worker has no page and so no token to
	// send. The comment was right, the order was wrong, and the effect was that
	// the one instrument built to answer "did it reach the handset" recorded
	// nothing, for every notification, on every device. Hours were then spent
	// looking at the sending half, which was working.
	//
	// Nothing about it is visible either: the worker's receipt() ends in
	// .catch(function(){}) — deliberately, since a receipt must never break a
	// notification — so the 403 was swallowed on the device too.
	if strings.HasSuffix(r.URL.Path, "/received") {
		var said struct {
			Tag   string `json:"tag"`
			Shown bool   `json:"shown"`
			Why   string `json:"why"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&said)
		Received(acc.ID, said.Tag, said.Shown, said.Why)
		app.RespondJSON(w, map[string]any{"ok": true})
		return
	}

	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}

	// A notification you asked for, now, on demand.
	//
	// The card can say "On for this device" and be telling the truth while
	// nothing has ever arrived, because the things that send one — mail, a
	// reminder firing — are all somebody else doing something. That leaves no
	// way to tell a working subscription from a broken one except waiting, and
	// waiting for a negative is not a test. This is the button.

	// SendNow rather than Send, or it is not a test at all: Send hands each
	// device to a goroutine and returns, so answering ok after calling it said
	// "ok" whether the push service took the notification, timed out, or refused
	// it outright. SendNow blocks and reports what happened.
	//
	// # And it has to be this device
	//
	// It sent to the account and reported the first device that accepted, while
	// the page said "It should appear on this device." Those are different
	// claims. An account can hold several subscriptions — an old browser, a
	// laptop, the same phone before it re-subscribed — and the one that answers
	// first is not a fact about who is looking at the screen. So the button
	// could report success having proved that some other handset works, and
	// send somebody hunting for the fault on a device nothing was sent to.
	//
	// The page holds exactly one subscription: the browser's own. It sends that
	// endpoint, and the notification goes there or nowhere. Then "this device"
	// is a claim the server actually checked.
	if strings.HasSuffix(r.URL.Path, "/test") {
		var ask struct {
			Endpoint string `json:"endpoint"`
		}
		// A page cached before this change sends no body, and an empty
		// endpoint still means every device — with an answer that says so
		// rather than one that says "this device".
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&ask)

		note := Notification{
			Title: "Test notification",
			Body:  "This is what mail and reminders will look like.",
			URL:   "/account",
			Tag:   "mu-test",
		}
		if to := strings.TrimSpace(ask.Endpoint); to != "" {
			if err := SendToDevice(acc.ID, to, note); err != nil {
				app.RespondJSON(w, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			app.RespondJSON(w, map[string]any{"ok": true, "here": true, "where": Where(to)})
			return
		}
		if err := SendNow(acc.ID, note); err != nil {
			app.RespondJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		app.RespondJSON(w, map[string]any{"ok": true, "here": false, "devices": len(Devices(acc.ID))})
		return
	}

	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		app.RespondJSON(w, map[string]any{"ok": false, "error": "could not read that subscription"})
		return
	}

	if strings.HasSuffix(r.URL.Path, "/unsubscribe") {
		// No endpoint means every device.
		//
		// The card offers "turn it off", and the browser can only hand back the
		// subscription it is holding — which is nothing at all once permission
		// has been revoked, or on a device somebody has already thrown away.
		// Somebody turning notifications off means it, and leaving a phone they
		// no longer own on the list because it could not be named is not what
		// they asked for.
		if strings.TrimSpace(body.Endpoint) == "" {
			Forget(acc.ID)
		} else {
			Unsubscribe(acc.ID, body.Endpoint)
		}
		app.RespondJSON(w, map[string]any{"ok": true})
		return
	}

	added, err := Subscribe(acc.ID, Subscription{
		Endpoint: body.Endpoint,
		P256dh:   body.Keys.P256dh,
		Auth:     body.Keys.Auth,
		Label:    strings.TrimSpace(body.Label),
	})
	if err != nil {
		app.RespondJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Proof it works, immediately, on the device that just asked. A permission
	// prompt somebody accepted and then saw nothing from is one they turn off.
	//
	// Only for a device that was not already here. The page re-posts whatever
	// subscription the browser holds on every load, so without this the greeting
	// would arrive every time somebody opened /account.
	if added {
		Send(acc.ID, Notification{
			Title: "Notifications are on",
			Body:  "Mail, briefings and answers will turn up here.",
			URL:   "/inbox",
			Tag:   "mu-welcome",
		})
	}
	app.RespondJSON(w, map[string]any{"ok": true})
}

// Card is the control, for /account.
//
// Absent when the instance cannot send — no key, no button — because an offer
// that cannot be honoured is worse than no offer.
func Card(r *http.Request, accountID string, titles ...string) string {
	title := "Notifications"
	if len(titles) > 0 {
		title = titles[0]
	}
	key := PublicKey()
	if key == "" || accountID == "" {
		return ""
	}
	// What the server knows, which is not the same question as "is it on here".
	//
	// This said "Not on this device" / "On." from an account-wide check, so a
	// laptop that had never been subscribed read "On. One device." because the
	// phone had. The page corrects it once it has asked the browser — only the
	// browser knows about this device — and this is what renders before that.
	state := "Off."
	if Subscribed(accountID) {
		state = devicesLine(accountID)
	}

	// .card first, because that is what every other section on /account is and
	// what constrains the column. This was a bare .push-card drawing its own
	// border and padding, so it inherited no width at all and ran the full
	// width of the page beside cards that did not.
	return `<div class="card push-card">` +
		`<div class="push-head"><strong>` + html.EscapeString(title) + `</strong>` +
		`<span class="push-state" id="push-state">` + html.EscapeString(state) + `</span></div>` +
		// One line, and a true one.
		//
		// It said "Mail, briefings and answers turn up on this device" plus a
		// sentence about the push service forwarding bytes it cannot read.
		// Two of those three are wrong — nothing sends a push for a briefing
		// or for an answer, only mail arriving, a reminder firing and an
		// operator alert — and the encryption sentence answers a question
		// nobody standing at this card was asking. Somebody who reads a claim
		// about what will turn up, turns it on, and then sees none of it,
		// concludes the feature is broken rather than that the sentence was.
		`<p class="push-note">Mail and reminders reach this device with the page closed.</p>` +
		// What has actually arrived, which is the question somebody who turned
		// this on a month ago is really asking. "On for one device" is a fact
		// about a row in a file and stays true while every send is refused.
		lastLine(accountID) +
		recentLines(accountID) +
		// Which devices, and the way to remove one.
		//
		// Subscription.Label has carried "what the device is, as far as the
		// browser will say — for a page listing them, so somebody can tell
		// which one to remove" since it was added, and no page listed them.
		// So the card could say "On for three devices" and offer no way to find
		// out what the third one was. See devicesList.
		devicesList(accountID) +
		`<button class="btn" id="push-go" type="button">Turn on for this device</button>` +
		`<button class="btn btn-quiet push-test d-none" id="push-test" type="button">Send a test</button>` +
		// And the way out. /push/unsubscribe existed from the beginning with
		// nothing calling it: the card could be turned on and never off, so
		// the only way to stop notifications was to revoke the permission in
		// browser settings — which is a different thing, does not tell this
		// instance, and leaves the device on the list being sent to forever.
		`<button class="btn btn-quiet push-off d-none" id="push-off" type="button">Turn off</button>` +
		// Which copy of the app this device is running, and the way to fix it.
		//
		// A service worker is installed, not loaded: the copy that handles a push
		// is whatever the browser installed last, which on a phone that has had
		// the app on the home screen for months can be very old. An old one
		// cannot post a receipt, so the record says "sent, nothing back" — which
		// is what it says when the worker never woke at all. Same line, two
		// completely different faults, and no way to tell them apart from here.
		//
		// So the page asks the worker its version. Update forces the browser to
		// re-fetch and take the new one.
		`<p class="push-note push-worker" id="push-worker" hidden></p>` +
		`<button class="btn btn-quiet d-none" id="push-update" type="button">Update this device</button>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</div>`
}

// recentLines is the record, on the screen.
//
// # An instrument nobody can read is not an instrument
//
// History has been written on every send and on every receipt for as long as
// both have existed, it is exported, it has tests — and nothing in the product
// ever called it. The file filled up and no page showed a line of it. So the
// only way to answer "did that notification reach the handset" was to ask the
// person holding the handset, which is exactly the question the receipts were
// built to stop having to ask, and the reason "notifications don't work" has
// been going round in circles: two people guessing at a fact the server had
// already written down.
//
// Four states, and telling them apart is the whole point. Which one it stops at
// says which half of the system to look in:
//
//   - refused — the push service would not take it. Ours: keys, signature,
//     an endpoint that has expired.
//   - sent, nothing back — the push service took it and the device never woke.
//     Theirs, or the handset's: battery saver, a killed service worker, a
//     browser that dropped the subscription without saying so.
//   - woke, could not show — the device ran the worker and failed to render.
//     Ours again, and the payload is the suspect.
//   - arrived — it worked, and if nothing appeared on the screen the problem is
//     the operating system's notification settings and nothing here.
//
// Five rows. This is a receipt, not a log: the question is what happened
// recently, and a page of history answers it no better while making the card
// something you scroll past.
func recentLines(accountID string) string {
	sent := History(accountID, 5)
	if len(sent) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="push-log"><div class="push-log-head">Recent</div>`)
	for i := len(sent) - 1; i >= 0; i-- {
		s := sent[i]
		what := strings.TrimSpace(s.Title)
		if what == "" {
			what = "A notification"
		}
		state, cls := "", ""
		switch {
		case !s.OK:
			state, cls = "refused"+because(s.Error), " push-bad"
		case s.Got.IsZero():
			state, cls = "sent — the device has not said it arrived", " push-bad"
		case !s.Shown:
			state, cls = "woke the device, which could not show it"+because(s.Why), " push-bad"
		default:
			state = "arrived"
		}
		from := ""
		if s.From != "" {
			from = ` <span class="push-from">` + html.EscapeString(s.From) + `</span>`
		}
		b.WriteString(`<div class="push-log-row"><span class="push-log-what">` +
			html.EscapeString(what) + `</span>` + from +
			`<span class="push-log-state` + cls + `">` + html.EscapeString(state) + `</span>` +
			`<span class="push-log-when">` + html.EscapeString(ago(s.At)) + `</span></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// because appends a reason when there is one, and nothing when there is not —
// rather than "refused ()" or a colon with nothing after it.
func because(why string) string {
	if strings.TrimSpace(why) == "" {
		return ""
	}
	return ": " + why
}

// lastLine says what became of the last notification sent to this account.
//
// Three states worth telling apart: nothing has ever been sent, something was
// accepted by a push service, or something was refused and why. Only the first
// of those was visible before, and only by inference from the absence of a
// card.
func lastLine(accountID string) string {
	sent, failed, reason := LastResult(accountID)
	switch {
	case failed.After(sent) && reason != "":
		return `<p class="push-note push-bad">Last try, ` + ago(failed) + `: ` +
			html.EscapeString(reason) + `.</p>`
	case !sent.IsZero():
		return `<p class="push-note">Last one sent ` + ago(sent) + `.</p>`
	case Subscribed(accountID):
		return `<p class="push-note">Nothing has been sent to this account yet. ` +
			`Send a test to check it works.</p>`
	}
	return ""
}

// ago is a rough age. Rough on purpose: the question is "recently, or not at
// all", and a timestamp to the second invites reading precision into it.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + " minutes ago"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + " hours ago"
	default:
		return itoa(int(d.Hours()/24)) + " days ago"
	}
}

// devicesList is every device this account is subscribed on, and the way to
// take one off.
//
// The card could count them and not name them, so "On for three devices" was
// unanswerable if you had thrown one away or, as happens, deleted and
// reinstalled an app: a reinstall is a new subscription, and the old one sits
// in the list being sent to. Dead ones are pruned — a push service answers 404
// or 410 and sendTo removes them — but only when something is actually sent, so
// between notifications the list is longer than the truth.
//
// Nothing here is a secret from the reader: it is their own account's list,
// shown to them. The endpoint is the identity a removal names, so it rides on
// the button.
//
// One device is not a list. A single row saying "this device" under a control
// that already says it is on for this device is furniture; the count line above
// already says everything there is to say.
func devicesList(accountID string) string {
	list := Devices(accountID)
	if len(list) < 2 {
		return ""
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Added.After(list[j].Added) })

	var b strings.Builder
	b.WriteString(`<div class="push-devices">`)
	for _, d := range list {
		label := strings.TrimSpace(d.Label)
		if label == "" {
			// The browser did not say. Naming it by its push service is the
			// only true thing left, and it is better than "a device" because
			// two rows of "a device" cannot be told apart — which is the exact
			// problem this list exists to solve.
			label = Where(d.Endpoint)
		}
		b.WriteString(`<div class="push-device" data-endpoint="` +
			html.EscapeString(d.Endpoint) + `">`)
		b.WriteString(`<span class="push-dev-name">` + html.EscapeString(label) + `</span>`)
		b.WriteString(`<span class="push-dev-when">` + html.EscapeString(deviceWhen(d)) + `</span>`)
		b.WriteString(`<button type="button" class="btn-quiet push-dev-off">Remove</button>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// deviceWhen is the one fact about a row that decides whether to remove it.
//
// Not when it was added, which is a fact about the past that says nothing about
// whether this device still exists. What was last sent to it does: a device
// that has been refusing since March is one you no longer have.
func deviceWhen(d Subscription) string {
	switch {
	case d.Failed.After(d.Sent) && d.Error != "":
		return "failing since " + ago(d.Failed)
	case !d.Sent.IsZero():
		return "last reached " + ago(d.Sent)
	default:
		return "added " + ago(d.Added)
	}
}

// devicesLine says how many, because "on" on a phone and "on" on a laptop are
// the same word for different situations.
func devicesLine(accountID string) string {
	switch n := len(Devices(accountID)); n {
	case 1:
		return "On for one device."
	default:
		return strings.TrimSpace("On for " + strings.ToLower(itoa(n)) + " devices.")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "No"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}
