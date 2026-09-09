package thread

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEvidenceIsOwnedScopedBoundedAndExpires(t *testing.T) {
	reset(t)
	th := Open("alice", "web", "evidence")
	now := time.Now()
	e := Evidence{Tool: "web_fetch", Service: "web", Result: strings.Repeat("x", 9000), At: now, Expires: now.Add(time.Hour)}
	KeepEvidence("bob", th.ID, e)
	if got := EvidenceFor("alice", th.ID, []string{"web"}, now); len(got) != 0 {
		t.Fatal("foreign write")
	}
	KeepEvidence("alice", th.ID, e)
	for _, owner := range []string{"bob", ""} {
		if len(EvidenceFor(owner, th.ID, []string{"web"}, now)) != 0 {
			t.Fatal("foreign read")
		}
	}
	if len(EvidenceFor("alice", th.ID, []string{"news"}, now)) != 0 {
		t.Fatal("scope bypass")
	}
	if len(EvidenceFor("alice", th.ID, []string{"web"}, now.Add(2*time.Hour))) != 0 {
		t.Fatal("stale observation")
	}
	got := EvidenceFor("alice", th.ID, []string{"web"}, now)
	if len(got) != 1 || !got[0].Truncated || len(got[0].Result) != 8000 {
		t.Fatal("result bounds")
	}
	if len(Messages("alice", th.ID, 0)) != 0 {
		t.Fatal("evidence became dialogue")
	}
	Forget("alice")
	if len(EvidenceFor("alice", th.ID, []string{"web"}, now)) != 0 {
		t.Fatal("evidence survived account deletion")
	}
}

func TestRecallOnlyOwnDialogue(t *testing.T) {
	reset(t)
	for _, x := range []struct{ owner, client string }{{"alice", "web"}, {"bob", "web"}, {"alice", "mail"}, {"alice", "chat"}} {
		th := Open(x.owner, x.client, x.client)
		Add(Message{Account: x.owner, Thread: th.ID, Role: RolePerson, Text: "My project is Mu", At: time.Now()})
	}
	got := Recall(context.Background(), "alice", "", []string{"project"})
	if len(got) != 1 || got[0].Account != "alice" || got[0].Client != "web" || got[0].Role != RolePerson {
		t.Fatalf("unexpected recall: %+v", got)
	}
	if len(Recall(context.Background(), "", "", []string{"project"})) != 0 {
		t.Fatal("guest recall")
	}
	if len(Recall(context.Background(), "alice", got[0].Thread, []string{"project"})) != 0 {
		t.Fatal("current thread duplicated")
	}
}
