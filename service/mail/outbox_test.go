package mail

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/userdb"
)

func queuedForTest(t *testing.T, owner string) []userdb.Record {
	t.Helper()
	rows, err := userdb.List("mail", owner, outboxCollection, "mine", nil, "", "", 200)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func makeQueued(t *testing.T, owner string, recipients ...string) userdb.Record {
	t.Helper()
	withDomain(t, "mu.test")
	_, err := queueReply(owner, "Specialist", "agent@mu.test", recipients[0], recipients[1:],
		"Re: Tomorrow", "A private answer", "<p>A private answer</p>", "<parent@example.com>", "<root@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	rows := queuedForTest(t, owner)
	if len(rows) != 1 {
		t.Fatalf("want one outgoing record, got %d", len(rows))
	}
	return rows[0]
}

func TestOutboxRetriesOnlyFailedRecipientsUsingTheOriginalMessage(t *testing.T) {
	owner := t.Name()
	rec := makeQueued(t, owner, "first@example.com", "second@example.com", "FIRST@example.com")
	m, err := readQueued(&rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Recipients) != 2 {
		t.Fatal("duplicate recipient was not removed")
	}
	calls := map[string]int{}
	relay := func(from, to string, raw []byte) error {
		calls[to]++
		if from != m.From || !bytes.Equal(raw, m.Message) || !bytes.Contains(raw, []byte(m.MessageID)) || !bytes.Contains(raw, []byte("<root@example.com> <parent@example.com>")) {
			t.Fatal("retry changed sender, message or threading")
		}
		if to == "second@example.com" && calls[to] == 1 {
			return fmt.Errorf("temporary SMTP failure")
		}
		return nil
	}
	if err := processQueued(owner, rec.ID, relay); err != nil {
		t.Fatal(err)
	}
	// Reloading is the restart boundary: progress exists only in the store.
	rows := queuedForTest(t, owner)
	got, err := readQueued(&rows[0])
	if err != nil || !got.Delivered["first@example.com"] || got.Delivered["second@example.com"] || got.LastError == "" {
		t.Fatalf("progress lost: %+v, %v", got, err)
	}
	if err := processQueued(owner, rec.ID, relay); err != nil {
		t.Fatal(err)
	}
	if calls["first@example.com"] != 1 || calls["second@example.com"] != 2 || len(queuedForTest(t, owner)) != 0 {
		t.Fatalf("incorrect retries: %+v", calls)
	}
}

func TestOutboxConcurrentAttemptsDoNotResendAcceptedMail(t *testing.T) {
	owner := t.Name()
	rec := makeQueued(t, owner, "one@example.com")
	var wg sync.WaitGroup
	calls := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = processQueued(owner, rec.ID, func(string, string, []byte) error { calls++; return nil })
		}()
	}
	wg.Wait()
	if calls != 1 {
		t.Fatalf("sent %d times", calls)
	}
}

func TestOutboxExpiryExplicitRetryAndOwnership(t *testing.T) {
	owner := t.Name()
	rec := makeQueued(t, owner, "one@example.com", "two@example.com")
	m, _ := readQueued(&rec)
	m.Created = time.Now().Add(-49 * time.Hour)
	m.Delivered["one@example.com"] = true
	if err := saveQueued(owner, rec.ID, m, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	relay := func(string, string, []byte) error { calls++; return nil }
	if err := processQueued("another-owner", rec.ID, relay); err == nil {
		t.Fatal("cross-owner send")
	}
	if err := processQueued(owner, rec.ID, relay); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("expired message was sent")
	}
	if err := retryQueued("another-owner", rec.ID); err == nil {
		t.Fatal("cross-owner retry")
	}
	if err := retryQueued(owner, rec.ID); err != nil {
		t.Fatal(err)
	}
	if err := processQueued(owner, rec.ID, relay); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("retry repeated a previously accepted recipient")
	}
}

func TestOutboxPersistsBeforeAcceptanceAndEncryptsThePayload(t *testing.T) {
	oldKey, oldEnabled := encKey, encEnabled
	encKey, encEnabled = bytes.Repeat([]byte{7}, 32), true
	t.Cleanup(func() { encKey, encEnabled = oldKey, oldEnabled })
	owner := t.Name()
	rec := makeQueued(t, owner, "private@example.com")
	payload, _ := rec.Data["payload"].(string)
	if !strings.HasPrefix(payload, encPrefix) || strings.Contains(payload, "private@example.com") || strings.Contains(payload, "private answer") {
		t.Fatal("outbox leaked plaintext")
	}
	m, err := readQueued(&rec)
	if err != nil || !bytes.Contains(m.Message, []byte("A private answer")) {
		t.Fatalf("could not recover encrypted message: %v", err)
	}
	// A store that cannot be written must never report acceptance.
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".mu", "data", "mail", "db", "outbox.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := enqueueMail(owner, m); err == nil {
		t.Fatal("unpersisted mail was accepted")
	}
}

func TestOutboxDeletionAndStatusDoNotCrossAccounts(t *testing.T) {
	rec := makeQueued(t, "outbox-owner", "private@example.com")
	other := makeQueued(t, "outbox-other", "other@example.com")
	w := httptest.NewRecorder()
	outboxPage(w, httptest.NewRequest("GET", "/mail?view=outbox", nil), "outbox-other")
	if strings.Contains(w.Body.String(), "private@example.com") || !strings.Contains(w.Body.String(), "other@example.com") {
		t.Fatal("outbox page crossed ownership")
	}
	deleteOutbox("outbox-owner")
	if _, err := userdb.Get("mail", "outbox-owner", outboxCollection, rec.ID); err == nil {
		t.Fatal("deleted owner's mail remains queued")
	}
	if _, err := userdb.Get("mail", "outbox-other", outboxCollection, other.ID); err != nil {
		t.Fatal("deleted another owner's mail")
	}
}

func TestOutboxRetryRequiresTheOwnersSessionAndCSRF(t *testing.T) {
	acc := mailAccount(t, "outboxcsrf")
	sess, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec := makeQueued(t, acc.ID, "one@example.com")
	m, _ := readQueued(&rec)
	if err := saveQueued(acc.ID, rec.ID, m, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	request := func(token string) *http.Request {
		form := url.Values{"id": {rec.ID}}
		if token != "" {
			form.Set("_csrf", token)
		}
		r := httptest.NewRequest("POST", "/mail?view=outbox", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		return r
	}
	r := request("")
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF accepted: %d", w.Code)
	}
	w = httptest.NewRecorder()
	Handler(w, request(auth.CSRFToken(r)))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("owned retry failed: %d, %s", w.Code, w.Body.String())
	}
	rows := queuedForTest(t, acc.ID)
	if len(rows) != 1 || rows[0].Data["pending"] != true {
		t.Fatal("retry did not become pending")
	}
}

func TestReplyAllReportsLocalDeliveryFailure(t *testing.T) {
	withDomain(t, "mu.test")
	if _, err := SendReplyAll("outbox-local", "Agent", "agent@mu.test", "missing-local-recipient@mu.test", nil, "Reply", "Answer", "", "", ""); err == nil {
		t.Fatal("missing local mailbox was reported as delivered")
	}
}
