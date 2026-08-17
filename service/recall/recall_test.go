package recall

// Somebody else's conversations are somebody else's.
//
// This is the one service in the catalogue whose whole subject is private
// correspondence — every email, every DM, everything anybody has ever typed at
// an agent. The interesting tests are all the same test: that the account
// asking is the only account answered.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"mu/internal/service"
	"mu/internal/thread"
)

func ctxFor(owner string) context.Context {
	return service.WithAccount(context.Background(), owner)
}

// two accounts, each with a conversation, one word in common.
func setup(t *testing.T) (mine, yours string, mineID, yoursID string) {
	t.Helper()
	n := time.Now().UnixNano()
	mine = fmt.Sprintf("recall-mine-%d", n)
	yours = fmt.Sprintf("recall-yours-%d", n)

	m := thread.Open(mine, "mail", "<root@example.com>")
	thread.Add(thread.Message{Thread: m.ID, Account: mine,
		Text: "the invoice for March is overdue", From: "client@example.com"})
	thread.Add(thread.Message{Thread: m.ID, Account: mine, Role: thread.RoleAgent,
		Text: "I have chased it."})

	y := thread.Open(yours, "web", "session-1")
	thread.Add(thread.Message{Thread: y.ID, Account: yours,
		Text: "my own invoice, which is none of their business"})

	return mine, yours, m.ID, y.ID
}

func TestSearchAnswersOnlyTheAccountAsking(t *testing.T) {
	mine, yours, _, _ := setup(t)

	var rsp SearchResponse
	if err := (Server{}).Search(ctxFor(mine), &SearchRequest{Query: "invoice"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "March") {
		t.Errorf("my own conversation was not found:\n%s", rsp.Text)
	}
	if strings.Contains(rsp.Text, "none of their business") {
		t.Fatalf("searching returned another account's messages:\n%s", rsp.Text)
	}
	// Where it was said, and how to read the rest of it — a result nothing
	// follows from is a dead end.
	if !strings.Contains(rsp.Text, "mail") || !strings.Contains(rsp.Text, "conversation:") {
		t.Errorf("a result does not say where it happened or which conversation:\n%s", rsp.Text)
	}
	// And who said it: an agent quoting a stranger's mail back as though the
	// account holder wrote it is worse than not finding it.
	if !strings.Contains(rsp.Text, "client@example.com") {
		t.Errorf("a message from somebody else is not attributed to them:\n%s", rsp.Text)
	}

	// Nobody at all gets nothing.
	if err := (Server{}).Search(context.Background(), &SearchRequest{Query: "invoice"}, &rsp); err == nil {
		t.Error("an unauthenticated caller searched somebody's history")
	}
	_ = yours
}

func TestAConversationCannotBeReadByItsIdAlone(t *testing.T) {
	mine, yours, mineID, yoursID := setup(t)

	var rsp ConversationResponse
	if err := (Server{}).Conversation(ctxFor(mine), &ConversationRequest{ID: mineID}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "overdue") || !strings.Contains(rsp.Text, "chased") {
		t.Errorf("my own conversation did not come back whole:\n%s", rsp.Text)
	}

	// The id is not a capability. Anyone can quote one — they are in search
	// results, in URLs, in anything an agent has written down.
	if err := (Server{}).Conversation(ctxFor(yours), &ConversationRequest{ID: mineID}, &rsp); err == nil {
		t.Fatalf("another account read my conversation by knowing its id:\n%s", rsp.Text)
	}
	if err := (Server{}).Conversation(ctxFor(mine), &ConversationRequest{ID: yoursID}, &rsp); err == nil {
		t.Fatal("I read somebody else's conversation by knowing its id")
	}
}

func TestListNamesOnlyYourOwnConversations(t *testing.T) {
	mine, yours, _, _ := setup(t)

	var rsp ListResponse
	if err := (Server{}).List(ctxFor(mine), &ListRequest{}, &rsp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Text, "invoice for March") {
		t.Errorf("my conversation is not listed:\n%s", rsp.Text)
	}
	if strings.Contains(rsp.Text, "none of their business") {
		t.Errorf("another account's conversation is listed:\n%s", rsp.Text)
	}

	// Narrowed to one client, which is the only filter there is.
	if err := (Server{}).List(ctxFor(mine), &ListRequest{Client: "web"}, &rsp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rsp.Text, "invoice for March") {
		t.Error("a mail conversation was listed under web")
	}
	_ = yours
}

// An empty query is a mistake, and answering it with the whole record is the
// wrong way to fail.
func TestAnEmptyQueryReturnsNothing(t *testing.T) {
	mine, _, _, _ := setup(t)

	var rsp SearchResponse
	if err := (Server{}).Search(ctxFor(mine), &SearchRequest{Query: "  "}, &rsp); err == nil {
		t.Errorf("an empty query was answered with %q", rsp.Text)
	}
	if got := thread.Search(mine, "", "", 10); got != nil {
		t.Errorf("an empty query matched %d messages in the store", len(got))
	}
}

// Deleting an account takes the transcript with it.
//
// The record is written by the machinery rather than by a service, so nothing
// owned it and nothing cleared it: deleting an account left every conversation
// it had ever had on disk.
func TestDeletingAnAccountForgetsEverythingItSaid(t *testing.T) {
	mine, yours, mineID, yoursID := setup(t)

	Delete(mine)

	if got := thread.Messages(mine, mineID, 0); len(got) != 0 {
		t.Errorf("%d messages survived the account being deleted", len(got))
	}
	if thread.Get(mine, mineID) != nil {
		t.Error("the conversation survived the account being deleted")
	}
	if got := thread.Search(mine, "invoice", "", 10); len(got) != 0 {
		t.Errorf("a deleted account's messages are still searchable (%d)", len(got))
	}
	// And nobody else's record was touched.
	if thread.Get(yours, yoursID) == nil {
		t.Fatal("deleting one account deleted another account's conversations")
	}
}
