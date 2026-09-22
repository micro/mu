package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

func TestPaidAccountCanReachAssistant(t *testing.T) {
	// Stop at provider selection after the real access gate, without making any
	// model requests. Payment is already an accepted trust signal elsewhere.
	t.Setenv("AI_PROVIDER", "unconfigured-test-provider")
	t.Setenv("AGENT_MODEL", "")
	old := auth.HasPaid
	t.Cleanup(func() { auth.HasPaid = old; auth.RemoveAccountForTest("paid_assistant_test") })
	paid := false
	auth.HasPaid = func(id string) bool { return id == "paid_assistant_test" && paid }
	acc := &auth.Account{ID: "paid_assistant_test"}
	auth.SetAccountForTest(acc)
	if _, err := runNative(acc.ID, "hello", QueryOpts{NoTools: true}); err == nil || errors.Is(err, ErrNoProvider) {
		t.Fatal("unverified unpaid account passed access gate")
	}
	paid = true
	if _, err := runNative(acc.ID, "hello", QueryOpts{NoTools: true}); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("paid account still blocked: %v", err)
	}
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/agent", strings.NewReader("{"))
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	handleQuery(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("paid HTTP request blocked before parsing: %d", w.Code)
	}
	// The durable queue rechecks access both on submission and on execution.
	replies.once.Do(func() {})
	oldJobs, oldActive, oldErr := replies.jobs, replies.active, replies.err
	t.Cleanup(func() { replies.jobs, replies.active, replies.err = oldJobs, oldActive, oldErr })
	replies.jobs, replies.active, replies.err = map[string]pendingReply{}, map[string]string{}, nil
	th := thread.Open(acc.ID, thread.WebClient, "paid reply")
	if err := SubmitReply(acc.ID, th.ID, "hello", "paid-test"); err != nil {
		t.Fatal(err)
	}
	asked := false
	processReply(func(req AskRequest) (Answer, error) { asked = true; return Answer{Text: "hello"}, nil })
	if !asked {
		t.Fatal("paid queued reply was rejected on execution")
	}
	acc.Banned = true
	auth.SetAccountForTest(acc)
	if _, err := runNative(acc.ID, "hello", QueryOpts{NoTools: true}); err == nil || errors.Is(err, ErrNoProvider) {
		t.Fatal("payment bypassed ban")
	}
}
