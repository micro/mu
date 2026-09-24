package inbox

import (
	"mu/internal/event"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/chat"
	"testing"
	"time"
)

func TestProjectionReplaysWithoutDuplicatesOrReadStateReset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	at := time.Now().Add(-time.Hour)
	e := event.Record{Service: "mail", Type: "mail.received", Account: "owner", Resource: "source", Version: "1"}
	m := &service.SourceMessage{ID: "source", Ref: "message-id", Conversation: "conversation", Text: "stored body", HTML: "<p>stored body</p>", Subject: "Re: subject", From: "sender@example.com", At: at}
	if err := cacheArrival(e, m, true); err != nil {
		t.Fatal(err)
	}
	th := thread.ByRef("owner", "message-id")
	if th == nil {
		t.Fatal("missing thread")
	}
	thread.MarkSeen("owner", th.ID)
	m.Text = "updated body"
	e.Version = "2"
	if err := cacheArrival(e, m, true); err != nil {
		t.Fatal(err)
	}
	got := thread.Messages("owner", th.ID, 0)
	if len(got) != 1 || got[0].Text != "updated body" || got[0].Source.Version != "2" {
		t.Fatalf("%+v", got)
	}
	if thread.UnreadCount("owner") != 0 {
		t.Fatal("replay reset read state")
	}
	// Deletion requires no service call, including when that service is offline.
	e.Type = "mail.deleted"
	if err := indexArrival(e); err != nil {
		t.Fatal(err)
	}
	if thread.Messages("owner", th.ID, 0)[0].SourceHTML != "" {
		t.Fatal("deleted HTML retained")
	}
}
func TestOutgoingChatIsReadAndHistoricalArrivalKeepsTime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	at := time.Now().Add(-24 * time.Hour)
	e := event.Record{Service: "chat", Account: "owner", Resource: "id"}
	m := &service.SourceMessage{Ref: "ref", Conversation: "chat", Text: "my words", From: "owner@example.com", Direction: "out", At: at}
	if err := cacheArrival(e, m, true); err != nil {
		t.Fatal(err)
	}
	th := thread.ByRef("owner", "ref")
	if !th.Updated.Equal(at) || thread.Unread(*th) {
		t.Fatalf("incorrect historical state: %+v", th)
	}
}

func TestArrivalReadsThroughServiceAPIOnlyDuringIndexing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := service.Register(chat.Spec); err != nil {
		t.Fatal(err)
	}
	id, err := chat.KeepSaved("project-owner", chat.Said{Conv: "xmpp_conversation", From: "sender@example.com", To: "owner@example.com", Text: "stored source"})
	if err != nil {
		t.Fatal(err)
	}
	e := event.Record{Type: event.ChatRecorded, Service: "chat", Account: "project-owner", Resource: id}
	if err := indexArrival(e); err != nil {
		t.Fatal(err)
	}
	th := thread.ByRef("project-owner", id)
	if th == nil {
		t.Fatal("arrival was not projected")
	}
	// The source can disappear while the cached page still loads. Its deletion
	// fact, rather than an on-demand join, is responsible for invalidation.
	chat.Forget("project-owner")
	if got := thread.Messages("project-owner", th.ID, 0); len(got) != 1 || got[0].Text != "stored source" {
		t.Fatalf("%+v", got)
	}
}
