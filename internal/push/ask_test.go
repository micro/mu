package push

// The offer, and the three ways it goes away.
//
// Turning notifications on was only reachable from /account — a page somebody
// visits to change a setting they already knew they wanted. The claim that
// makes an inbox worth having with the page closed was invisible from the
// screen where it is true.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/settings"
)

func TestTheOfferCarriesEverythingItNeeds(t *testing.T) {
	// A key, so the instance can send. keys() mints and stores one on first
	// use, so this is normally already true; set it so the test does not
	// depend on that having happened.
	if PublicKey() == "" {
		t.Fatal("this instance cannot mint a VAPID key, so nothing can be offered")
	}

	r := httptest.NewRequest("GET", "/inbox", nil)
	got := Ask(r, "asker")
	if got == "" {
		t.Fatal("the inbox offers nothing on an instance that can send")
	}

	// The four ids cardJS binds. It early-returns without push-go and reads the
	// other three by id, so a missing one is a button that throws when pressed
	// rather than a button that does nothing visible.
	for _, id := range []string{"push-go", "push-state", "push-key", "push-csrf"} {
		if !strings.Contains(got, `id="`+id+`"`) {
			t.Errorf("the offer has no #%s, which cardJS reaches for", id)
		}
	}

	// The script itself, and not a second copy of the subscribe dance. If this
	// ever stops being cardJS the two will drift, and the one that drifts is
	// the one nobody visits.
	if !strings.Contains(got, "pushManager.subscribe") {
		t.Error("the offer ships no way to subscribe")
	}
	if strings.Count(got, "pushManager.subscribe") != strings.Count(cardJS, "pushManager.subscribe") {
		t.Error("the offer carries its own copy of the subscribe path rather than cardJS")
	}

	// Hidden until the browser has been asked. The server cannot know whether
	// this device is already subscribed — only the browser can — so drawing it
	// visible would offer it to somebody who already has it every time they
	// open their inbox.
	open := strings.Index(got, `id="push-ask"`)
	if open < 0 {
		t.Fatal("the offer has no wrapper to show or hide")
	}
	end := strings.Index(got[open:], ">")
	if !strings.Contains(got[open:open+end], "hidden") {
		t.Error("the offer renders visible, so a device that already has " +
			"notifications is asked to turn them on")
	}

	// Two labels, and only two. It shipped as a banner with a sentence, a
	// filled button and a "Not now" — reported as "way too big". A control is
	// smaller than the thing it controls, and "Turn on notifications" already
	// says what pressing it does, so the explainer and the dismissal both went:
	// a button at the end of a row is not in anybody's way, which is what the
	// dismissal existed to fix.
	if !strings.Contains(got, ">Turn on notifications</button>") {
		t.Error("nothing says how to turn notifications on")
	}
	if !strings.Contains(got, ">Turn off</button>") {
		t.Error("a device that has them on is not offered the way off, so the " +
			"control has one state and is a button rather than a toggle")
	}
	if strings.Contains(got, "Not now") {
		t.Error("the dismissal is back — there is nothing to dismiss when the " +
			"control is a toggle at the end of the actions row")
	}
	for _, explainer := range []string{"Get told when", "with this page closed"} {
		if strings.Contains(got, explainer) {
			t.Errorf("the control carries an explainer (%q); the label is the "+
				"explanation", explainer)
		}
	}

	// The off button is cardJS's own — it swaps the pair, so this needs no
	// state of its own and cannot disagree with the card on /account.
	if !strings.Contains(got, `id="push-off"`) {
		t.Error("the way off is not the id cardJS drives, so something here is " +
			"deciding the state a second time")
	}
}

// Nobody signed out is offered anything either.
func TestASignedOutReaderIsNotOffered(t *testing.T) {
	r := httptest.NewRequest("GET", "/inbox", nil)
	if got := Ask(r, ""); got != "" {
		t.Errorf("a signed-out reader is offered notifications: %q", got)
	}
}

// An instance that cannot send does not ask.
//
// Without a VAPID key the button would request a browser permission nothing on
// this server can use — a prompt with no feature behind it, which is worse
// than the feature being absent.
func TestNoKeyMeansNoOffer(t *testing.T) {
	was := settings.Get("VAPID_PRIVATE_KEY")
	settings.Set("VAPID_PRIVATE_KEY", "not base64url at all !!!")
	t.Cleanup(func() { settings.Set("VAPID_PRIVATE_KEY", was) })

	if PublicKey() != "" {
		t.Fatal("an unusable key still produced a public one, so the check that " +
			"turns the feature off in the UI never fires")
	}

	r := httptest.NewRequest("GET", "/inbox", nil)
	if got := Ask(r, "asker"); got != "" {
		t.Errorf("an instance that cannot send still offers notifications: %q", got)
	}
}
