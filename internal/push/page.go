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
func Card(r *http.Request, accountID string) string {
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
		`<div class="push-head"><strong>Notifications</strong>` +
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
		`<button class="btn" id="push-go" type="button">Turn on for this device</button>` +
		`<button class="btn btn-quiet push-test d-none" id="push-test" type="button">Send a test</button>` +
		// And the way out. /push/unsubscribe existed from the beginning with
		// nothing calling it: the card could be turned on and never off, so
		// the only way to stop notifications was to revoke the permission in
		// browser settings — which is a different thing, does not tell this
		// instance, and leaves the device on the list being sent to forever.
		`<button class="btn btn-quiet push-off d-none" id="push-off" type="button">Turn off</button>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</div>` + cardCSS + cardJS
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

const cardCSS = `<style>
/* Layout only. The buttons were three bespoke rules here — their own padding,
 * their own font-size, their own border — which is how a card ends up with
 * controls that are a different height from every other control in the product.
 * They use .btn and .btn-quiet now, and the control scale in mu.css decides
 * how big a button is. */
.push-head{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}
.push-state{font-size:12px;color:#888}
.push-note{font-size:13px;color:#888;line-height:1.55;margin:6px 0 10px}
.push-bad{color:var(--danger,#c33)}
.push-log{margin:10px 0 12px;border-top:1px solid var(--card-border,#eee);padding-top:8px}
.push-log-head{font-size:11px;font-weight:600;letter-spacing:.04em;text-transform:uppercase;
  color:var(--text-muted,#999);margin:0 0 6px}
.push-log-row{display:flex;align-items:baseline;gap:8px;flex-wrap:wrap;font-size:12px;
  color:var(--text-secondary,#555);padding:3px 0}
.push-log-what{color:var(--text-primary,#111)}
.push-from{color:var(--text-muted,#999)}
.push-log-state{margin-left:auto}
.push-log-when{color:var(--text-muted,#999);white-space:nowrap}
.push-card .btn{margin-right:6px}
</style>`

// cardJS is the three steps, in the order the browser insists on — plus the
// step that keeps it on.
//
// Turning it on used to be the only time a subscription reached the server, and
// there are several ordinary ways for the server to stop having it: a browser
// rotates its endpoint and the old one starts returning 410, which prunes the
// device; an instance is restored from a backup taken before you subscribed; a
// deploy goes out from a machine with a stale store. None of those touch the
// browser, which is still holding a perfectly good subscription and will go on
// holding it — so the only way back was a person noticing the card had gone
// quiet and pressing the button again. That is the "it keeps getting reset".
//
// So the page re-posts whatever the browser is holding, every load. It is
// matched on the endpoint and so is idempotent, it costs one request, and it
// closes the loop without anybody having to notice anything.
const cardJS = `<script>
(function(){
  var go = document.getElementById('push-go');
  if (!go) return;
  var state = document.getElementById('push-state');
  function say(t){ if (state) state.textContent = t; }

  if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
    go.disabled = true;
    say('This browser cannot show notifications.');
    return;
  }
  if (Notification.permission === 'denied') {
    go.disabled = true;
    say('Blocked in this browser’s settings.');
    return;
  }

  // base64url to bytes: what the subscription call wants the key as.
  function keyBytes(s){
    var pad = '='.repeat((4 - s.length % 4) % 4);
    var raw = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'));
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }
  function b64(buf){
    var bytes = new Uint8Array(buf), s = '';
    for (var i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
    return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  // Hand one subscription to the server. Idempotent: it is matched on the
  // endpoint, so sending the same one twice updates it rather than doubling it.
  function tell(sub){
    var raw = sub.toJSON();
    return fetch('/push/subscribe', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': document.getElementById('push-csrf').value
      },
      body: JSON.stringify({
        endpoint: sub.endpoint,
        keys: {p256dh: raw.keys.p256dh, auth: raw.keys.auth},
        label: navigator.platform || ''
      })
    }).then(function(res){ return res.json(); });
  }

  var test = document.getElementById('push-test');
  var off = document.getElementById('push-off');

  function on(){
    say('On for this device.');
    go.classList.add('d-none');
    if (test) test.classList.remove('d-none');
    if (off) off.classList.remove('d-none');
  }

  function offNow(){
    go.classList.remove('d-none');
    go.disabled = false;
    if (test) test.classList.add('d-none');
    if (off) off.classList.add('d-none');
    say('Off.');
  }

  // Turning it off, properly: the browser stops holding a subscription and
  // this instance stops holding a row. Doing either alone is what leaves the
  // two disagreeing — a device the server goes on sending to forever, or a
  // browser that silently re-registers on the next page load, which is exactly
  // what the re-post below does.
  if (off) off.addEventListener('click', function(){
    off.disabled = true;
    off.textContent = 'Turning off…';
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      var endpoint = sub ? sub.endpoint : '';
      // Unsubscribe the browser first. If this page then fails to reach the
      // server the worst case is a row nothing can deliver to, which the send
      // path prunes on the first 410 — the other order leaves a browser that
      // re-registers itself.
      var done = sub ? sub.unsubscribe() : Promise.resolve();
      return done.then(function(){
        return fetch('/push/unsubscribe', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': document.getElementById('push-csrf').value
          },
          body: JSON.stringify({endpoint: endpoint})
        });
      });
    }).then(function(){
      off.disabled = false;
      off.textContent = 'Turn off';
      offNow();
    }).catch(function(){
      off.disabled = false;
      off.textContent = 'Turn off';
      say('That did not work.');
    });
  });

  // Proof, on demand. Only offered once this device is actually subscribed —
  // before that there is nothing for it to arrive on.
  if (test) test.addEventListener('click', function(){
    test.disabled = true;
    test.textContent = 'Sending…';
    // This browser's own subscription, so the server sends there and nowhere
    // else. Without it the test proved that *a* device works, which is not
    // what the answer said and not what anybody wanted to know.
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).catch(function(){ return null; }).then(function(sub){
      return fetch('/push/test', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': document.getElementById('push-csrf').value
        },
        body: JSON.stringify({endpoint: sub ? sub.endpoint : ''})
      });
    }).then(function(res){ return res.json(); }).then(function(data){
      test.disabled = false;
      test.textContent = 'Send a test';
      // Both outcomes out loud. Saying nothing on success is indistinguishable
      // from the button being dead, which is what it looked like — and if the
      // notification itself does not arrive, "the push service took it" is the
      // fact that tells you the problem is on the device and not here.
      //
      // Named, because which push service took it is most of the diagnosis:
      // web.push.apple.com is a phone, fcm.googleapis.com is usually not.
      if (!data || !data.ok) { say((data && data.error) || 'That did not work.'); return; }
      if (data.here) {
        say('Sent to this device' + (data.where ? ' via ' + data.where : '') +
            '. If nothing appears, the push service took it and the device did not show it.');
      } else {
        // No endpoint to send: this page could not name its own subscription,
        // so it went to everything registered. Say that rather than claim a
        // device the server never checked.
        say('Sent to all ' + (data.devices || 0) + ' registered device(s) — this browser ' +
            'could not name its own, so turn notifications off and on again here.');
      }
    }).catch(function(){
      test.disabled = false;
      test.textContent = 'Send a test';
      say('Could not reach the server.');
    });
  });

  // What the browser is already holding, told to the server again.
  //
  // This is the half that was missing. It also answers the question the server
  // cannot: "on" for the account is not "on here", and only this browser knows
  // whether this device is one of them.
  if (Notification.permission === 'granted') {
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      if (!sub) return;
      return tell(sub).then(function(data){ if (data && data.ok) on(); });
    }).catch(function(){ /* leave the button as it is */ });
  }

  go.addEventListener('click', function(){
    go.disabled = true;
    say('Asking…');
    Notification.requestPermission().then(function(p){
      if (p !== 'granted') { go.disabled = false; say('Not allowed.'); return; }
      return navigator.serviceWorker.ready.then(function(reg){
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: keyBytes(document.getElementById('push-key').value)
        });
      }).then(tell).then(function(data){
        if (data && data.ok) { on(); }
        else { go.disabled = false; say((data && data.error) || 'That did not work.'); }
      });
    }).catch(function(err){
      go.disabled = false;
      say('That did not work: ' + err.message);
    });
  });
})();
</script>`
