package agent

import (
	"mu/internal/thread"
	"mu/service/mail"
	"testing"
)

func TestCheckinLegacyMigrationRetainsBothReplies(t *testing.T) {
	owner := "checkin-legacy-migration"
	defer thread.Forget(owner)
	source := thread.Open(owner, "mail", "original-checkin")
	id := thread.Add(thread.Message{Account: owner, Thread: source.ID, Text: "Hello", From: "agent@" + mail.ConfiguredDomain(), To: owner + "+checkin@test", Ref: "<original@test>"})
	target := thread.Open(owner, thread.WebClient, "checkin:"+source.ID)
	thread.Add(thread.Message{Account: owner, Thread: target.ID, Text: "Hello", Ref: "checkin-import:" + id})
	thread.Add(thread.Message{Account: owner, Thread: source.ID, Text: "Mail reply", Ref: "<mail@test>"})
	thread.Add(thread.Message{Account: owner, Thread: target.ID, Text: "Web reply", Ref: "web-reply"})
	got, err := consolidateCheckin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != target.ID || len(thread.List(owner, 0)) != 1 || len(thread.Messages(owner, got.ID, 0)) != 3 {
		t.Fatal("legacy migration lost or duplicated messages")
	}
	got, err = consolidateCheckin(owner, source)
	if err != nil || got.ID != target.ID || len(thread.Messages(owner, got.ID, 0)) != 3 {
		t.Fatal("migration not idempotent")
	}
}
