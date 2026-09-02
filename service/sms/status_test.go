package sms

// Delivery receipts, and the question they exist to answer.
//
// "It was slow" was unanswerable. The record stopped at the handover — twilio.Send
// returns when the provider accepts, not when a phone buzzes — so a message that
// took a minute to arrive and one that took a second looked identical here, and
// the only way to guess was to change something and see whether anybody
// complained again.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The send asks to be told. Without StatusCallback on the request there is no
// receipt to record, and every test below is about a fact that never arrives.
func TestTheSendAsksWhatBecomesOfIt(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	t.Setenv("TWILIO_WEBHOOK_URL", "")
	t.Setenv("TWILIO_FROM", "+14155550100")

	form, err := sendForm(ChannelSMS, "+14155550123", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got := form.Get("StatusCallback"); got != "https://example.test/sms/status" {
		t.Errorf("StatusCallback = %q, so nothing ever reports what became of a message", got)
	}
}

// And does not ask on a box the provider cannot reach. An unreachable callback
// is not a missing feature, it is Twilio retrying against whatever answers.
func TestNoCallbackWhereNobodyCanReachUs(t *testing.T) {
	t.Setenv("MU_DOMAIN", "localhost:8099")
	t.Setenv("TWILIO_WEBHOOK_URL", "")
	t.Setenv("TWILIO_FROM", "+14155550100")

	form, err := sendForm(ChannelSMS, "+14155550123", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got := form.Get("StatusCallback"); got != "" {
		t.Errorf("a development box asked to be called back at %q", got)
	}
}

// The address the operator says reaches us wins, because it is the one known to.
func TestTheConfiguredWebhookAddressDecides(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	t.Setenv("TWILIO_WEBHOOK_URL", "https://proxy.example.net/sms/webhook")
	if got := callbackURL(); got != "https://proxy.example.net/sms/status" {
		t.Errorf("callbackURL = %q, want the host that is known to reach this instance", got)
	}
}

// A receipt names the provider's id and nothing else — no account, no session —
// so unless the send filed where the message went, there is nothing to match.
//
// This is the bug the SID field replaced: Send assigned the provider's id to
// m.ID on the value it returned and never stored it, so the record carried this
// database's id under the one name a receipt could have looked it up by.
func TestASentMessageIsFindableByTheProvidersID(t *testing.T) {
	setup(t)
	t.Setenv("TWILIO_FROM", "+14155550100")

	orig := send
	send = func(to, body string) (string, error) { return "SM0123456789", nil }
	t.Cleanup(func() { send = orig })

	m := recordOn(ChannelSMS, "someone", "out", "+14155550123", "hello", 1, "SM0123456789")
	noteSend("someone", m.ID, "SM0123456789")

	if m.SID != "SM0123456789" {
		t.Errorf("the provider's id is not on the message: %q", m.SID)
	}
	if m.ID == m.SID {
		t.Error("this record's id and the provider's are the same field again")
	}
	owner, id := sentAs("SM0123456789")
	if owner != "someone" || id != m.ID {
		t.Errorf("sentAs(SM0123456789) = %q/%q, want someone/%s", owner, id, m.ID)
	}
	// And the id survives being read back, which is where the old bug showed.
	found := false
	for _, h := range History("someone", 10) {
		if h.ID == m.ID {
			found = true
			if h.SID != "SM0123456789" {
				t.Errorf("read back, the provider's id is %q", h.SID)
			}
		}
	}
	if !found {
		t.Fatal("the message is not in the history")
	}
}

// Recording what the provider said, on the message it is about.
func TestAReceiptLandsOnTheMessage(t *testing.T) {
	setup(t)
	m := recordOn(ChannelSMS, "someone", "out", "+14155550123", "hello", 1, "SM1")
	noteSend("someone", m.ID, "SM1")

	if !SetStatus("SM1", "delivered") {
		t.Fatal("a receipt for a message this instance sent was not recorded")
	}
	got := find(t, "someone", m.ID)
	if got.Status != "delivered" {
		t.Errorf("status = %q, want delivered", got.Status)
	}
	if got.StatusAt.IsZero() {
		t.Error("nothing recorded when it landed, which is the half of 'slow' that is not ours")
	}
	// And the message is still a message. Update replaces a record's data
	// outright, so writing two fields without merging deletes the text.
	if got.Text != "hello" || got.Number != "+14155550123" {
		t.Errorf("recording a receipt emptied the message: %+v", got)
	}
}

// Receipts arrive out of order — sent and delivered are two posts and either
// can be the one that is retried — so the last word stays the last word.
func TestALateReceiptDoesNotUndoADelivery(t *testing.T) {
	setup(t)
	m := recordOn(ChannelSMS, "someone", "out", "+14155550123", "hello", 1, "SM2")
	noteSend("someone", m.ID, "SM2")

	SetStatus("SM2", "delivered")
	SetStatus("SM2", "sent") // the earlier post, arriving late
	if got := find(t, "someone", m.ID).Status; got != "delivered" {
		t.Errorf("a late 'sent' walked a delivered message back to %q", got)
	}
	// A failure after a delivery is a different matter: both are settled, and
	// the newer one is what the provider currently believes.
	SetStatus("SM2", "failed")
	if got := find(t, "someone", m.ID).Status; got != "failed" {
		t.Errorf("status = %q, want failed", got)
	}
}

// Anything the provider does not actually say is not written into somebody's
// history by a public endpoint.
func TestOnlyRealStatusesAreRecorded(t *testing.T) {
	setup(t)
	m := recordOn(ChannelSMS, "someone", "out", "+14155550123", "hello", 1, "SM3")
	noteSend("someone", m.ID, "SM3")

	for _, bad := range []string{"", "<b>delivered</b>", "probably", "DROP TABLE"} {
		if SetStatus("SM3", bad) {
			t.Errorf("%q was accepted as a delivery status", bad)
		}
	}
	if got := find(t, "someone", m.ID).Status; got != "" {
		t.Errorf("status = %q after only nonsense was posted", got)
	}
}

// A receipt for something this instance did not send is dropped, not filed
// against whatever happens to be first in a list.
func TestAReceiptForNothingIsDropped(t *testing.T) {
	setup(t)
	if SetStatus("SM-never-sent", "delivered") {
		t.Error("a receipt naming an unknown message was recorded against something")
	}
}

// The page says something when there is something to say, and nothing when
// there is not. A tick beside every message that worked is a page about itself.
func TestThePageIsQuietWhenNothingWentWrong(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	now := time.Now()

	for _, c := range []struct {
		what string
		m    Message
		want string
	}{
		{"delivered promptly", Message{Direction: "out", Status: "delivered",
			At: now.Add(-time.Hour), StatusAt: now.Add(-time.Hour).Add(2 * time.Second)}, ""},
		{"delivered slowly", Message{Direction: "out", Status: "delivered",
			At: now.Add(-time.Hour), StatusAt: now.Add(-time.Hour).Add(42 * time.Second)}, "delivered after 42s"},
		{"failed", Message{Direction: "out", Status: "failed", At: now.Add(-time.Hour)}, "not delivered"},
		{"just sent, nothing back yet", Message{Direction: "out", At: now}, ""},
		{"never heard anything", Message{Direction: "out", At: now.Add(-time.Hour)}, "no delivery receipt"},
		{"inbound", Message{Direction: "in", At: now.Add(-time.Hour)}, ""},
	} {
		if got := deliveryNote(c.m); got != c.want {
			t.Errorf("%s: note = %q, want %q", c.what, got, c.want)
		}
	}
}

// The receipt endpoint is public — the provider has no session — so the
// provider's own signature is the credential, exactly as on the inbound
// webhook. Without that, anybody who knows the URL can mark other people's
// messages failed.
func TestAnUnsignedReceiptIsRefused(t *testing.T) {
	setup(t)
	t.Setenv("TWILIO_ACCOUNT_SID", "AC"+strings.Repeat("0", 32))
	t.Setenv("TWILIO_AUTH_TOKEN", strings.Repeat("a", 32))
	t.Setenv("SMS_VERIFY_INBOUND", "")

	m := recordOn(ChannelSMS, "someone", "out", "+14155550123", "hello", 1, "SM4")
	noteSend("someone", m.ID, "SM4")

	body := url.Values{"MessageSid": {"SM4"}, "MessageStatus": {"failed"}}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/sms/status", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	StatusHandler(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("an unsigned receipt got %d, want 403", w.Code)
	}
	if got := find(t, "someone", m.ID).Status; got != "" {
		t.Errorf("an unsigned receipt marked the message %q", got)
	}
}

// find reads one message back out of the record.
func find(t *testing.T, owner, id string) Message {
	t.Helper()
	for _, m := range History(owner, 200) {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no message %s for %s", id, owner)
	return Message{}
}
