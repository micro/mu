package thread

import (
	"fmt"
	"testing"
	"time"
)

func TestCachedContentIsLocalDurableAndScoped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	at := time.Now().Add(-24 * time.Hour)
	th := OpenAt("owner", "mail", "conversation", at)
	source := &Source{Service: "unavailable", ID: "source-id", Version: "1"}
	id := Add(Message{Account: "owner", Thread: th.ID, Ref: "ref", Text: "cached body", SourceHTML: "<p>cached body</p>", Source: source, From: "sender", At: at})
	if id == "" {
		t.Fatal("not added")
	}
	Add(Message{Account: "owner", Thread: th.ID, Ref: "ref", Text: "updated body", SourceHTML: "<p>updated body</p>", Source: &Source{Service: "unavailable", ID: "source-id", Version: "2"}})
	if err := Flush(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	loaded = false
	mu.Unlock()
	got := Messages("owner", th.ID, 0)
	if len(got) != 1 || got[0].Text != "updated body" || got[0].SourceHTML != "<p>updated body</p>" {
		t.Fatalf("%+v", got)
	}
	if len(Messages("other", th.ID, 0)) != 0 {
		t.Fatal("cross-account read")
	}
	InvalidateSource("other", "unavailable", "source-id")
	if Messages("owner", th.ID, 0)[0].Text != "updated body" {
		t.Fatal("cross-account invalidation")
	}
	InvalidateSource("owner", "unavailable", "source-id")
	got = Messages("owner", th.ID, 0)
	if got[0].Text != "(message deleted)" || got[0].SourceHTML != "" {
		t.Fatal("deleted source still visible")
	}
}

func TestLongConversationDoesNotEvictHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	th := Open("owner", WebClient, "long")
	for n := 0; n < 5005; n++ {
		Add(Message{Account: "owner", Thread: th.ID, Text: fmt.Sprintf("message %d", n)})
	}
	got := Messages("owner", th.ID, 0)
	if len(got) != 5005 || got[0].Text != "message 0" {
		t.Fatalf("history lost: %d", len(got))
	}
}
