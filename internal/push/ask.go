package push

// The offer, where the thing being offered actually happens.
//
// Turning notifications on lived on /account, in a card between the passkey
// list and the legal links, which is a page somebody visits to change a
// setting they already knew they wanted. Nothing anywhere else said the
// product could tell you when something arrived — so the one claim that makes
// a self-hosted inbox worth having with the page closed was visible only to
// somebody who went looking for it.
//
// The inbox is where things arrive. It is the screen where "I would like to be
// told about this" is a thought somebody actually has, and it is the screen
// they are on when it is true.
//
// # The card is still the card
//
// This is not a second one. /account keeps the full thing — the device list,
// the receipts, the test send, the worker version, the way off — because that
// is a page for reading about a setting. This is one line and one button: the
// offer, and no administration.
//
// It reuses cardJS unchanged, the way the panel on Home reuses the room's own
// socket rather than reimplementing a chat client. The script binds by id and
// guards every element it does not find, so the ids here are the four it
// needs and the rest of the card being absent is a case it already handles. A
// second implementation of the subscribe dance — permission, worker,
// applicationServerKey, the base64url encoding, the re-post that keeps a
// rotated endpoint alive — is the thing most likely to drift from the first.
//
// # And it goes away
//
// Three ways, because there are three different "no" here and only one of them
// is a decision:
//
//   - This device is already subscribed. Not a decision, a fact, and the
//     server cannot know it — only the browser can, which is why this renders
//     hidden and the script reveals it rather than the other way round.
//   - The browser cannot do it, or the permission is already denied. Offering
//     is noise; /account explains.
//   - "Not now". That is the decision, and it is remembered per browser,
//     because a subscription is per browser. Somebody who dismissed it on a
//     laptop has said nothing about their phone.

import (
	"html"
	"net/http"

	"mu/internal/auth"
)

// Ask is the offer: one line, a button, and a way to say no.
//
// Empty when this instance cannot send at all — no VAPID key means the button
// would ask for a permission nothing can use.
func Ask(r *http.Request, accountID string) string {
	key := PublicKey()
	if key == "" || accountID == "" {
		return ""
	}
	return `<div class="push-ask" id="push-ask" hidden>` +
		`<span class="push-ask-say">Get told when something arrives, ` +
		`with this page closed.</span>` +
		`<button class="btn" id="push-go" type="button">Turn on notifications</button>` +
		`<button class="btn btn-quiet" id="push-ask-no" type="button">Not now</button>` +
		`<span class="push-state" id="push-state"></span>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</div>` + askCSS + cardJS + askJS
}

const askCSS = `<style>
/* One line, quiet, and out of the way of the list under it. It is an offer,
 * not a notice: a bordered banner at the top of the inbox is the shape of
 * something being announced, and this is something being made available. */
.push-ask{display:flex;align-items:center;gap:10px;flex-wrap:wrap;
  margin:0 0 14px;padding:10px 12px;border:1px solid var(--card-border,#eee);
  border-radius:6px;background:var(--card-bg,#fff)}
.push-ask[hidden]{display:none}
.push-ask-say{font-size:13px;color:var(--text-secondary,#555);margin-right:auto}
.push-ask .push-state{flex-basis:100%}
</style>`

// askJS decides whether the offer is worth making, and takes it away when it
// is not.
//
// cardJS runs first and does the work; this only shows and hides. The order
// matters and is why Ask concatenates them that way round: cardJS re-posts an
// existing subscription on load, and this reads the result.
const askJS = `<script>
(function(){
  var ask = document.getElementById('push-ask');
  var go = document.getElementById('push-go');
  if (!ask || !go) return;

  var KEY = 'mu_push_not_now';
  function said(){ try { return localStorage.getItem(KEY) === '1'; } catch (e) { return false; } }
  function remember(){ try { localStorage.setItem(KEY, '1'); } catch (e) {} }

  // A control that cannot work is worse than no control. cardJS disables the
  // button and writes the reason into push-state for both of these; on this
  // page there is nothing to read the reason, so there is nothing to show.
  var usable = ('serviceWorker' in navigator) && ('PushManager' in window) &&
    (typeof Notification !== 'undefined') && Notification.permission !== 'denied';

  function show(){ if (!said()) ask.hidden = false; }
  function hide(){ ask.hidden = true; }

  if (!usable) return; // stays hidden

  // Already on for this device? Only the browser knows — "on" for the account
  // is not "on here", which is the whole reason the card corrects itself after
  // asking. Ask before offering.
  if (Notification.permission === 'granted') {
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      if (sub) return;    // this device has it; say nothing
      show();
    }).catch(show);
  } else {
    show();
  }

  // cardJS hides the button by adding d-none once the server has the
  // subscription. That is the success signal, and watching for it is how this
  // stays out of the transaction rather than running a second copy of it.
  new MutationObserver(function(){
    if (go.classList.contains('d-none')) hide();
  }).observe(go, {attributes: true, attributeFilter: ['class']});

  var no = document.getElementById('push-ask-no');
  if (no) no.addEventListener('click', function(){
    remember();
    hide();
  });
})();
</script>`
