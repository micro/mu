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

	"mu/internal/app"
	"mu/internal/auth"
)

// SubscribeHandler serves POST /push/subscribe and POST /push/unsubscribe.
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
		Unsubscribe(acc.ID, body.Endpoint)
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
		`<p class="push-note">Mail, briefings and answers turn up on this device when the ` +
		`page is closed. The text is encrypted on its way through — the push service ` +
		`forwards bytes it cannot read.</p>` +
		`<button class="push-go" id="push-go" type="button">Turn on for this device</button>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</div>` + cardCSS + cardJS
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

  function on(){
    say('On for this device.');
    go.textContent = 'On';
    go.disabled = true;
  }

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
