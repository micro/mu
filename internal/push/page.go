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
	if strings.HasSuffix(r.URL.Path, "/test") {
		if !Subscribed(acc.ID) {
			app.RespondJSON(w, map[string]any{"ok": false, "error": "no device is registered yet"})
			return
		}
		Send(acc.ID, Notification{
			Title: "Test notification",
			Body:  "This is what mail and reminders will look like.",
			URL:   "/account",
			Tag:   "mu-test",
		})
		app.RespondJSON(w, map[string]any{"ok": true})
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
		`<button class="push-go" id="push-go" type="button">Turn on for this device</button>` +
		`<button class="push-test d-none" id="push-test" type="button">Send a test</button>` +
		// And the way out. /push/unsubscribe existed from the beginning with
		// nothing calling it: the card could be turned on and never off, so
		// the only way to stop notifications was to revoke the permission in
		// browser settings — which is a different thing, does not tell this
		// instance, and leaves the device on the list being sent to forever.
		`<button class="push-off d-none" id="push-off" type="button">Turn off</button>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</div>` + cardCSS + cardJS
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
.push-head{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap}
.push-state{font-size:12px;color:#888}
.push-note{font-size:13px;color:#888;line-height:1.55;margin:6px 0 10px}
.push-go{font:inherit;font-size:13px;padding:7px 16px;border:1px solid #111;background:#111;color:#fff;border-radius:6px;cursor:pointer}
.push-go[disabled]{opacity:.5;cursor:default}
.push-test{font:inherit;font-size:13px;padding:7px 16px;border:1px solid var(--card-border,#ddd);
  background:var(--card-background,#fff);color:var(--text-primary,#111);border-radius:6px;cursor:pointer}
.push-test[disabled]{opacity:.5;cursor:default}
.push-off{font:inherit;font-size:13px;padding:7px 16px;border:1px solid var(--card-border,#ddd);
  background:transparent;color:var(--text-muted,#888);border-radius:6px;cursor:pointer}
.push-off[disabled]{opacity:.5;cursor:default}
.push-bad{color:var(--danger,#c33)}
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
    fetch('/push/test', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {'X-CSRF-Token': document.getElementById('push-csrf').value}
    }).then(function(res){ return res.json(); }).then(function(data){
      test.disabled = false;
      test.textContent = 'Send a test';
      if (!(data && data.ok)) say((data && data.error) || 'That did not work.');
    }).catch(function(){
      test.disabled = false;
      test.textContent = 'Send a test';
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
