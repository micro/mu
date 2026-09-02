package mail

// The way out, end to end.

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A GET explains and offers a button; it does not act.
//
// Mail clients, scanners and link previewers fetch the URLs in a message. A GET
// that unsubscribed would unsubscribe people who never clicked anything.
func TestAGetDoesNotUnsubscribe(t *testing.T) {
	SetForwarding("gwen", true)
	tok := UnsubscribeToken("gwen")

	r := httptest.NewRequest("GET", "/mail/unsubscribe?t="+url.QueryEscape(tok), nil)
	w := httptest.NewRecorder()
	UnsubscribeHandler(w, r)

	if w.Code != 200 {
		t.Fatalf("GET returned %d", w.Code)
	}
	if !ForwardingOn("gwen") {
		t.Error("a GET turned forwarding off — a link preview in somebody's mail\n" +
			"client would unsubscribe them without a click")
	}
	if !strings.Contains(w.Body.String(), "<form method=\"POST\"") {
		t.Error("the page offers no way to actually do it")
	}
}

func TestAPostUnsubscribes(t *testing.T) {
	SetForwarding("hank", true)
	tok := UnsubscribeToken("hank")

	form := url.Values{"t": {tok}}
	r := httptest.NewRequest("POST", "/mail/unsubscribe", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	UnsubscribeHandler(w, r)

	if w.Code != 200 {
		t.Fatalf("POST returned %d", w.Code)
	}
	if ForwardingOn("hank") {
		t.Error("the unsubscribe did not take")
	}
}

// And it needs no session — that is the whole point.
func TestUnsubscribingNeedsNoAccount(t *testing.T) {
	SetForwarding("iris", true)
	tok := UnsubscribeToken("iris")

	form := url.Values{"t": {tok}}
	r := httptest.NewRequest("POST", "/mail/unsubscribe", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No session cookie of any kind.
	w := httptest.NewRecorder()
	UnsubscribeHandler(w, r)

	if ForwardingOn("iris") {
		t.Error("unsubscribing required a session. Somebody who wants these to\n" +
			"stop is by definition somebody who does not want to sign in first,\n" +
			"and the mail arrived at an address only they read.")
	}
}

// Nobody can unsubscribe anybody else.
func TestAForgedTokenChangesNothing(t *testing.T) {
	SetForwarding("jade", true)
	real := UnsubscribeToken("kyle")
	forged := "jade." + strings.SplitN(real, ".", 2)[1]

	form := url.Values{"t": {forged}}
	r := httptest.NewRequest("POST", "/mail/unsubscribe", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	UnsubscribeHandler(w, r)

	if w.Code != 404 {
		t.Errorf("a forged token returned %d, want 404", w.Code)
	}
	if !ForwardingOn("jade") {
		t.Error("a forged token turned somebody else's mail off")
	}
}
