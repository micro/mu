package hello

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mu/internal/ai"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/thread"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("MU_DOMAIN", "example.test")
	t.Setenv("MAIL_DOMAIN", "example.test")
	if err := data.SaveJSON(storeKey, &state{People: map[string]*introduction{}}); err != nil {
		t.Fatal(err)
	}
	oldAsk, oldSend := ask, send
	t.Cleanup(func() { ask = oldAsk; send = oldSend })
}
func TestBoundedIntroductionAndVerifiedClaim(t *testing.T) {
	setup(t)
	calls, sends := 0, 0
	ask = func(p *ai.Prompt) (string, error) {
		calls++
		if p.MaxTokens != 400 || p.Model == "" || p.Caller != "hello" {
			t.Fatal("unbounded model request")
		}
		if len(p.Context) != calls-1 {
			t.Fatal("lost history")
		}
		return "A useful answer", nil
	}
	send = func(to, subject, body, parent, id string) error {
		sends++
		if to != "person@example.net" || !strings.Contains(body, "/signup") || parent == "" || id == "" {
			t.Fatal("bad reply")
		}
		if sends == 3 && !strings.Contains(body, "end of this short introduction") {
			t.Fatal("missing limit message")
		}
		return nil
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("<message-%d@example.net>", i)
		if err := Receive("person@example.net", id, "Hello", "Help me plan a walk"); err != nil {
			t.Fatal(err)
		}
		if err := Receive("person@example.net", id, "Hello", "duplicate"); err != nil {
			t.Fatal(err)
		}
		process()
	}
	if calls != 3 || sends != 3 {
		t.Fatalf("got %d model calls and %d sends", calls, sends)
	}
	s, err := read()
	if err != nil || s.Used != 3 {
		t.Fatal("quota not persistent")
	}
	if err := Claim("hello_owner_test", "person@example.net"); err == nil {
		t.Fatal("unverified claim accepted")
	}
	if err := auth.Create(&auth.Account{ID: "hello_owner_test", Email: "person@example.net", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	if err := Claim("hello_owner_test", "person@example.net"); err != nil {
		t.Fatal(err)
	}
	if err := Claim("hello_owner_test", "person@example.net"); err != nil {
		t.Fatal(err)
	}
	th := thread.Find("hello_owner_test", "mail", "<message-0@example.net>")
	if th == nil {
		t.Fatal("no imported thread")
	}
	if len(thread.Messages("hello_owner_test", th.ID, 100)) != 6 {
		t.Fatal("missing or duplicated messages")
	}
	s, _ = read()
	p := s.People[key("person@example.net")]
	if p.Messages[0].Text != "" || p.Owner != "hello_owner_test" {
		t.Fatal("claim not committed")
	}
	if thread.ByRef("hello_owner_test", p.Messages[2].ReplyID) == nil {
		t.Fatal("replies cannot continue imported thread")
	}
	if err := Claim("another_account", "person@example.net"); err == nil {
		t.Fatal("foreign claim accepted")
	}
}
func TestRetryDoesNotRepeatModelAndDailyCap(t *testing.T) {
	setup(t)
	calls := 0
	ask = func(*ai.Prompt) (string, error) { calls++; return "Answer", nil }
	send = func(string, string, string, string, string) error { return fmt.Errorf("relay unavailable") }
	if err := Receive("retry@example.net", "<retry>", "Hello", "Question"); err != nil {
		t.Fatal(err)
	}
	process()
	s, _ := read()
	p := s.People[key("retry@example.net")]
	p.Messages[0].Next = time.Time{}
	data.SaveJSON(storeKey, s)
	send = func(string, string, string, string, string) error { return nil }
	process()
	if calls != 1 {
		t.Fatal("retry repeated model")
	}
	s, _ = read()
	s.Day = time.Now().UTC().Format("2006-01-02")
	s.Used = dailyLimit
	data.SaveJSON(storeKey, s)
	if err := Receive("new@example.net", "<new>", "Hello", "Question"); err == nil {
		t.Fatal("global cap bypassed")
	}
	if err := Receive("retry@example.net", "<retry>", "Hello", "Question"); err != nil {
		t.Fatal("deduplicated delivery rejected at cap")
	}
}
func TestExpiredContentIsRemoved(t *testing.T) {
	setup(t)
	s, _ := read()
	s.People[key("old@example.net")] = &introduction{Email: "old@example.net", Messages: []turn{{ID: "old", Text: "Private text", Answer: "Private answer", At: time.Now().Add(-31 * 24 * time.Hour), Sent: true}}}
	data.SaveJSON(storeKey, s)
	process()
	s, _ = read()
	p := s.People[key("old@example.net")]
	if !p.Expired || p.Email != "" || p.Messages[0].Text != "" || p.Messages[0].Answer != "" {
		t.Fatal("expired content retained")
	}
}

func TestCorruptStoreFailsClosed(t *testing.T) {
	setup(t)
	if err := data.SaveFile(storeKey, "broken JSON"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Receive("bad@example.net", "<bad>", "hello", "question"); err == nil {
			t.Fatal("corrupt store reset trial")
		}
	}
}
