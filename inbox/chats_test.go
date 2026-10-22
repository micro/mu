package inbox

import (
	"mu/internal/group"
	"mu/internal/thread"
	"testing"
	"time"
)

func TestChatsMembershipAndOrder(t *testing.T) {
	owner := "chat-composition-owner"
	g, err := group.Create(owner, "Shared plans", false)
	if err != nil {
		t.Fatal(err)
	}
	c, err := group.SaveConversation(g.ID, owner, "", "Weekend", "")
	if err != nil {
		t.Fatal(err)
	}
	personal := thread.Open(owner, thread.WebClient, "composition-personal")
	thread.Name(owner, personal.ID, "Private ideas")
	thread.Add(thread.Message{Account: owner, Thread: personal.ID, Text: "Latest", At: time.Now().Add(time.Minute)})
	stranger := thread.Open("composition-stranger", thread.WebClient, "secret")
	thread.Name("composition-stranger", stranger.ID, "Secret")
	got := Chats(owner)
	if len(got) != 3 || got[0].ID != personal.ID {
		t.Fatalf("wrong selection or ordering: %+v", got)
	}
	found := false
	for _, row := range got {
		if row.ID == c.ID {
			found = row.Shared && row.Context == g.Name
		}
	}
	if !found {
		t.Fatal("shared chat missing group context")
	}
	if err := group.Change(g.ID, owner, "invite", "composition-member", ""); err != nil {
		t.Fatal(err)
	}
	if len(Chats("composition-member")) != 0 {
		t.Fatal("invitation exposed history")
	}
	if err := group.Change(g.ID, "composition-member", "accept", "", ""); err != nil {
		t.Fatal(err)
	}
	if len(Chats("composition-member")) != 2 {
		t.Fatal("member cannot see chats")
	}
	if err := group.Change(g.ID, owner, "remove", "composition-member", ""); err != nil {
		t.Fatal(err)
	}
	if len(Chats("composition-member")) != 0 {
		t.Fatal("removed member sees chats")
	}
}
