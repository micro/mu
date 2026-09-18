package inbox

import (
	"fmt"
	"mu/internal/thread"
	"mu/service/mail"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantComposerUsesQueue(t *testing.T) {
	saved := Continue
	defer func() { Continue = saved }()
	calls := 0
	var id string
	Continue = func(owner, threadID, text, ref string) error {
		calls++
		th := thread.Get(owner, threadID)
		if th == nil || th.Client != thread.WebClient || th.Subject != "Plan" {
			t.Fatal("not an assistant conversation")
		}
		if text != "Help me plan" || ref != "compose:"+threadID {
			t.Fatal("message or retry reference changed")
		}
		id = threadID
		return nil
	}
	w := httptest.NewRecorder()
	sent(w, httptest.NewRequest("POST", "/inbox/new", nil), "compose_queue_owner", form{Mode: "assistant", Kind: kindMessage, Subject: "Plan", Body: "Help me plan", To: "unused@example.com"})
	if calls != 1 || w.Code != 303 || w.Header().Get("Location") != "/?session="+id {
		t.Fatalf("not queued: %d %s", w.Code, w.Body.String())
	}
	// On failure the draft and conversation remain available for a retry.
	Continue = func(owner, threadID, text, ref string) error { return fmt.Errorf("queue unavailable") }
	w = httptest.NewRecorder()
	sent(w, httptest.NewRequest("POST", "/inbox/new", nil), "compose_queue_owner", form{Mode: "assistant", Kind: kindMessage, On: id, Subject: "Plan", Body: "Help me plan"})
	if !strings.Contains(w.Body.String(), "Help me plan") || !strings.Contains(w.Body.String(), `name="on" value="`+id+`"`) {
		t.Fatal("failed send discarded draft or retry thread")
	}
}

func TestAssistantPartyAliases(t *testing.T) {
	owner := "party_alias_owner"
	th := thread.Open(owner, mailClient, "alias-test")
	for _, p := range []thread.Party{
		{Kind: thread.RolePerson},
		{Kind: thread.RolePerson, Key: mail.EmailForUser(owner, mail.ConfiguredDomain()), Name: "Asim"},
		{Kind: thread.RolePerson, Key: mail.AgentMailbox},
		{Kind: thread.RoleAgent, Key: "micro", Name: "Micro"},
		{Kind: thread.RoleAgent},
	} {
		thread.Join(owner, th.ID, p)
	}
	if got := partyLine(owner, th); got != "" {
		t.Fatalf("duplicate assistant parties: %s", got)
	}
	thread.Join(owner, th.ID, thread.Party{Kind: thread.RolePerson, Key: "other@example.com", Name: "Other"})
	got := partyLine(owner, th)
	if !strings.Contains(got, "Other") || strings.Count(got, "Micro") != 1 || strings.Count(got, "You") != 1 {
		t.Fatalf("lost real participant or duplicated identities: %s", got)
	}
}

func TestInboxAssistantOpensHome(t *testing.T) {
	owner := "inbox_home_redirect"
	th := thread.Open(owner, thread.WebClient, "redirect-test")
	r := httptest.NewRequest("GET", "/inbox?id="+th.ID, nil)
	w := httptest.NewRecorder()
	conversation(w, r, owner, th.ID)
	if w.Code != 303 || w.Header().Get("Location") != "/?session="+th.ID {
		t.Fatal("assistant still has a second inbox reader")
	}
	w = httptest.NewRecorder()
	conversation(w, r, "someone_else", th.ID)
	if w.Code != 404 {
		t.Fatal("redirect bypassed thread ownership")
	}
}
