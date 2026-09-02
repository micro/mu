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
// # A control, not an announcement
//
// It shipped as a bordered banner across the top of the inbox: a sentence
// explaining the feature, a filled button, and a "Not now" to make it go away.
// That is the shape of something being announced, and it was the largest thing
// on a page whose job is to show what arrived. Reported as "way too big".
//
// So it is one quiet control on the row the page's actions are already on,
// opposite New, and it says what pressing it does: "Turn on notifications", or
// "Turn off" once this device has them. There is nothing to dismiss, because
// there is nothing being pushed at anybody — a toggle sitting at the end of a
// row is not in the way, which is what the "Not now" existed to fix.
//
// It still renders hidden and reveals itself, for the reason that has not
// changed: whether *this device* is subscribed is a fact only the browser
// knows, and it decides which of the two labels is true. A browser that cannot
// take notifications at all, or has had the permission denied, gets nothing —
// /account is where that is explained.

import (
	"html"
	"net/http"

	"mu/internal/auth"
)

// Ask is the control: one quiet button that says what pressing it does.
//
// Empty when this instance cannot send at all — no VAPID key means the button
// would ask for a permission nothing can use.
//
// Both labels ship. push-go is the one cardJS drives, and push-off is the one
// it reveals when the device is subscribed — the same pair the card on
// /account uses, so this needs no state of its own and cannot disagree with it.
func Ask(r *http.Request, accountID string) string {
	key := PublicKey()
	if key == "" || accountID == "" {
		return ""
	}
	return `<span class="push-ask" id="push-ask" hidden>` +
		// The noun first, then the verb.
		//
		// It was the button alone — "Turn on notifications", and once a device
		// was subscribed just "Turn off", with "On for this device." written
		// beside it. So in the state you spend all your time in, the row read
		// "Turn off · On for this device" and named the thing nowhere. A
		// control whose label only says what pressing it does is fine while
		// the label is a whole sentence and stops being fine the moment it
		// shortens.
		//
		// So the label is what it controls, and the button is what pressing it
		// does. That also lets both buttons be two words instead of one being
		// four, which is what made the pair look like two different controls.
		`<span class="push-what">Notifications</span>` +
		`<button class="btn-quiet push-ask-go" id="push-go" type="button">` +
		`Turn on</button>` +
		`<button class="btn-quiet push-ask-go d-none" id="push-off" type="button">` +
		`Turn off</button>` +
		// Empty, so it takes no room until there is something to say — and not
		// hidden, which is what it was for one commit. cardJS writes every
		// failure into this element: a permission refused, a subscribe that
		// did not take, a server that would not have it. Hidden, pressing the
		// button and having nothing happen would be indistinguishable from a
		// dead control, which is the report this whole feature started from.
		`<span class="push-state" id="push-state"></span>` +
		`<input type="hidden" id="push-key" value="` + html.EscapeString(key) + `">` +
		`<input type="hidden" id="push-csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`</span>` + askCSS + cardJS + askJS
}

const askCSS = `<style>
/* At the end of the row the page's actions are on, pushed to the right by the
 * auto margin — opposite New rather than under a banner of its own.
 *
 * It was a bordered box across the full width with a sentence in it, which is
 * the shape of an announcement. A control is smaller than the thing it
 * controls. */
.push-ask{margin-left:auto;display:inline-flex;align-items:center;gap:8px}
.push-ask[hidden]{display:none}
.push-ask .push-state{font-size:12px;color:var(--text-muted,#888)}
/* The noun, in the page's ordinary voice — this is a label rather than a
   heading, and it sits on the row with the button it names. */
.push-ask .push-what{font-size:13px;color:var(--text-muted,#888)}
.push-ask-go{font-size:13px}
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

  // A control that cannot work is worse than no control. cardJS disables the
  // button and writes the reason into push-state for both of these; on this
  // row there is no room to read a reason, so there is nothing to show.
  if (!('serviceWorker' in navigator) || !('PushManager' in window) ||
      typeof Notification === 'undefined' || Notification.permission === 'denied') {
    return; // stays hidden
  }

  // Reveal once, whichever way round it is. cardJS decides which of the two
  // buttons is showing — it swaps them on load if this device already has a
  // subscription, and again after turning on or off — so all this has to do is
  // stop hiding the pair. Nothing here duplicates that decision.
  ask.hidden = false;
})();
</script>`
