package mail

import (
	"context"
	"encoding/json"
	"errors"
	smtpd "github.com/emersion/go-smtp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
)

func TestPersonalSenderRulesAreScopedAndAuthenticated(t *testing.T) {
	const owner = "sender-rules-owner"
	if err := changeAllowedSender(owner, "Peerlist.io", false); err != nil {
		t.Fatal(err)
	}
	defer changeAllowedSender(owner, "peerlist.io", true)
	for _, from := range []string{"person@notpeerlist.io", "person@peerlist.io.evil.test", "person@mail.peerlist.io"} {
		if personalSenderAllowed(owner, from) {
			t.Fatal("domain overmatched", from)
		}
	}
	if !personalSenderAllowed(owner, "person@peerlist.io") || personalSenderAllowed("other-owner", "person@peerlist.io") {
		t.Fatal("rule isolation lost")
	}
	for _, invalid := range []string{"*", "*.io", "peerlist.io/path", "Name <x@peerlist.io>", "@localhost"} {
		if err := changeAllowedSender(owner, invalid, false); err == nil {
			t.Fatal("accepted invalid rule", invalid)
		}
	}
	var saved map[string][]string
	if err := data.LoadJSON("mail_allowed_senders.json", &saved); err != nil || len(saved[owner]) != 1 {
		t.Fatal("rule not persisted", err)
	}
	result := classifyInbound(owner, "person@peerlist.io", false, "unknown sender", SpamResult{})
	if !result.IsSpam {
		t.Fatal("allow entry bypassed authentication")
	}
	if classifyInbound(owner, "person@peerlist.io", true, "unknown sender", SpamResult{IsSpam: true}).IsSpam {
		t.Fatal("authenticated rule not applied")
	}
	if !classifyInbound("other-owner", "person@peerlist.io", true, "unknown sender", SpamResult{}).IsSpam {
		t.Fatal("recipient inherited another mailbox's rule")
	}
	if !isWhitelistedDomain("peerlist.io") {
		t.Fatal("Peerlist missing from built-in list")
	}
}

func TestExistingJunkIsRecoverableWithoutAgentEvents(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mail.test")
	const owner = "junk-recipient"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	if err := SendMessageTo(Delivery{FromID: "verify@unknown-vendor.test", ToID: owner, Subject: "Verify email", Body: "Existing junk", MessageID: "<junk-test@unknown-vendor.test>", Spam: true}); err != nil {
		t.Fatal(err)
	}
	m := FindMessageByMessageID("<junk-test@unknown-vendor.test>")
	if m == nil || !m.Spam || m.ToID != owner {
		t.Fatal("mail not retained in Junk")
	}
	var rsp service.SourceResponse
	if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &service.SourceRequest{ID: m.ID}, &rsp); err != nil || rsp.Item != nil {
		t.Fatal("Junk exposed to source consumer", err)
	}
	var read ReadResponse
	if err := (Server{}).Read(service.WithAccount(context.Background(), owner), &ReadRequest{ID: m.ID}, &read); err == nil {
		t.Fatal("agent could read Junk directly")
	}
	if err := NotSpamMessage("someone-else", m.ID); err == nil {
		t.Fatal("foreign mailbox released Junk")
	}
	if err := NotSpamMessage(owner, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := (Server{}).Source(service.WithAccount(context.Background(), owner), &service.SourceRequest{ID: m.ID}, &rsp); err != nil || rsp.Item == nil {
		t.Fatal("released mail absent from Inbox", err)
	}
	files, err := persist.List("outbox/log")
	if err != nil {
		t.Fatal(err)
	}
	projected := false
	for _, file := range files {
		b, err := persist.Read("outbox/log/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var e event.Record
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatal(err)
		}
		if e.Resource != m.ID {
			continue
		}
		if e.Type == event.MailAccepted {
			t.Fatal("junk arrival or release could start an agent")
		}
		if e.Type == "mail.received" {
			projected = true
		}
	}
	if !projected {
		t.Fatal("release did not project to Inbox")
	}
}

func TestJunkActionsRequireOwnerAndCSRF(t *testing.T) {
	const owner = "junk-controls"
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	m := &Message{ID: "junk-control-id", ToID: owner, FromID: "verify@controls.test", Subject: "Verify", Body: "Hello", Spam: true, CreatedAt: time.Now()}
	mutex.Lock()
	addMessage(m)
	mutex.Unlock()
	request := func(id, action string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", junkURL, strings.NewReader(url.Values{"msg_id": {id}, "action": {action}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		JunkHandler(w, r)
		return w
	}
	if w := request(m.ID, "allow_domain", false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if personalSenderAllowed(owner, m.FromID) {
		t.Fatal("CSRF changed allowlist")
	}
	if w := request("foreign-message", "allow_domain", true); w.Code != 404 {
		t.Fatal("missing message accepted", w.Code)
	}
	if w := request(m.ID, "allow_domain", true); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !personalSenderAllowed(owner, m.FromID) || junkMessage(owner, m.ID) != nil {
		t.Fatal("allow and release failed")
	}
	defer changeAllowedSender(owner, "controls.test", true)
}

func TestSMTPAppliesPersonalAllowlistPerRecipient(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mail.test")
	for _, owner := range []string{"allowed-recipient", "other-recipient"} {
		auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
		defer auth.RemoveAccountForTest(owner)
	}
	if err := changeAllowedSender("allowed-recipient", "unfamiliar.test", false); err != nil {
		t.Fatal(err)
	}
	defer changeAllowedSender("allowed-recipient", "unfamiliar.test", true)
	s := &Session{from: "person@unfamiliar.test", to: []string{"allowed-recipient@mail.test", "other-recipient@mail.test"}, spfPass: true}
	raw := "From: person@unfamiliar.test\r\nTo: allowed-recipient@mail.test, other-recipient@mail.test\r\nSubject: Hello\r\nMessage-ID: <per-recipient@unfamiliar.test>\r\nContent-Type: text/plain\r\n\r\nA message for both recipients."
	if err := s.Data(strings.NewReader(raw)); err == nil {
		t.Fatal("mixed allowed and unallowed recipients accepted")
	}
	if FindMessageByMessageID("<per-recipient@unfamiliar.test>") != nil {
		t.Fatal("rejected transaction partially delivered")
	}
	s.to = []string{"allowed-recipient+test@mail.test"}
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	mutex.RLock()
	defer mutex.RUnlock()
	count := 0
	for _, m := range messages {
		if m.MessageID == "<per-recipient@unfamiliar.test>" {
			count++
			if m.Spam != (m.ToID == "other-recipient") {
				t.Fatal("allow rule crossed recipient boundary", m.ToID, m.Spam)
			}
			if m.Arrival == nil || m.Arrival.Shared {
				t.Fatal("personal delivery acquired shared-agent authority")
			}
		}
	}
	if count != 1 {
		t.Fatal("missing recipient delivery", count)
	}
}

func TestMailPreferencesPagesAndIsolation(t *testing.T) {
	const owner = "preferences-reader"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	w := get(senderSettingsURL, SenderSettingsHandler)
	for _, want := range []string{"Email address or domain", "Allowed senders", "No personal allowed senders", `name="_csrf"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatal("missing settings content", want)
		}
	}
	if err := changeAllowedSender(owner, "example.org", false); err != nil {
		t.Fatal(err)
	}
	defer changeAllowedSender(owner, "example.org", true)
	w = get(senderSettingsURL, SenderSettingsHandler)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "example.org") || !strings.Contains(w.Body.String(), "Remove") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = get(junkURL, JunkHandler)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "No junk mail.") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = get(junkURL+"&id=foreign", JunkHandler)
	if w.Code != 404 {
		t.Fatal("foreign message exposed", w.Code)
	}
}

func TestSMTPRejectsUnknownBeforeStorage(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mail.test")
	const owner = "blocked-recipient"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	for _, aligned := range []bool{false, true} {
		s := &Session{from: "stranger@unlisted.test", to: []string{owner + "@mail.test"}, spfPass: aligned}
		raw := "From: stranger@unlisted.test\r\nSubject: Hello\r\nMessage-ID: <blocked@unlisted.test>\r\n\r\nHello"
		var smtpErr *smtpd.SMTPError
		if err := s.Data(strings.NewReader(raw)); !errors.As(err, &smtpErr) || smtpErr.Code != 550 {
			t.Fatalf("expected SMTP rejection, got %v", err)
		}
		if FindMessageByMessageID("<blocked@unlisted.test>") != nil {
			t.Fatal("blocked mail was stored")
		}
	}
}

func TestSMTPInboundAllowSources(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mail.test")
	const owner = "admission-recipient"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	if err := changeAllowedSender(owner, "personal.test", false); err != nil {
		t.Fatal(err)
	}
	defer changeAllowedSender(owner, "personal.test", true)
	RecordOutbound("<outbound-admission@mail.test>", "friend@correspondent.test")
	if err := AddAllowedSender("operator@custom.test"); err != nil {
		t.Fatal(err)
	}
	defer RemoveAllowedSender("operator@custom.test")
	for _, from := range []string{"notice@peerlist.io", "person@personal.test", "friend@correspondent.test", "operator@custom.test"} {
		t.Run(from, func(t *testing.T) {
			s := &Session{from: from, to: []string{owner + "@mail.test"}, spfPass: true}
			id := "<admission-" + from + ">"
			raw := "From: " + from + "\r\nSubject: Hello\r\nMessage-ID: " + id + "\r\n\r\nHello there, this is a normal message."
			if err := s.Data(strings.NewReader(raw)); err != nil {
				t.Fatal(err)
			}
			if FindMessageByMessageID(id) == nil {
				t.Fatal("allowed mail not stored")
			}
		})
	}
	s := &Session{from: "person@personal.test", to: []string{owner + "@mail.test"}}
	if err := s.Data(strings.NewReader("From: person@personal.test\r\nSubject: Hello\r\n\r\nHello")); err == nil {
		t.Fatal("unauthenticated personal allow bypass")
	}
}
