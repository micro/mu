package group

import (
	"testing"
	"time"
)

func TestSharedSpaceInvitesIsolationAndRevocation(t *testing.T) {
	g, err := Create("space-owner", "Studio", false)
	if err != nil {
		t.Fatal(err)
	}
	defer Change(g.ID, g.Owner, "delete", "", "")
	token, err := InvitationLink(g.ID, g.Owner, false)
	if err != nil {
		t.Fatal(err)
	}
	if Member(g.ID, "new-member") {
		t.Fatal("link granted membership before acceptance")
	}
	if err := AcceptLink(g.ID, "new-member", "wrong"); err == nil {
		t.Fatal("bad token accepted")
	}
	if err := AcceptLink(g.ID, "new-member", token); err != nil {
		t.Fatal(err)
	}
	c, err := SaveConversation(g.ID, "new-member", "", "Launch", "none")
	if err != nil {
		t.Fatal(err)
	}
	if got, _, ok := ConversationFor(c.ID); !ok || got.ID != g.ID {
		t.Fatal("conversation lost its group")
	}
	if _, _, ok := ConversationFor(RoomPrefix + g.ID + "_forged"); ok {
		t.Fatal("forged room accepted")
	}
	if _, err := SaveConversation(g.ID, "new-member", c.ID, "Launch", ""); err == nil {
		t.Fatal("member changed assistant")
	}
	if err := Configure(g.ID, "new-member", 100, true); err == nil {
		t.Fatal("member changed budget")
	}
	NoteMessage(g.ID, c.ID, time.Now())
	fresh, _ := Read(g.ID, g.Owner)
	if Unread(fresh, g.Owner) != 1 {
		t.Fatal("missing unread")
	}
	MarkRead(g.ID, g.Owner, c.ID)
	fresh, _ = Read(g.ID, g.Owner)
	if Unread(fresh, g.Owner) != 0 {
		t.Fatal("not marked read")
	}
	if _, err := InvitationLink(g.ID, g.Owner, true); err != nil {
		t.Fatal(err)
	}
	if err := AcceptLink(g.ID, "another", token); err == nil {
		t.Fatal("revoked link worked")
	}
	if err := Change(g.ID, g.Owner, "remove", "new-member", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(g.ID, "new-member"); err == nil {
		t.Fatal("removed member has access")
	}
	if _, err := Conversations(g.ID, "new-member"); err == nil {
		t.Fatal("removed member can list conversations")
	}
	if _, ok := FromScope(Scope(g.ID)); !ok {
		t.Fatal("missing shared identity")
	}
}
func TestSubscriptionProtectsGroupDeletion(t *testing.T) {
	g, _ := Create("billing-owner", "Funded group", false)
	if err := ProtectBilling(g.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := Change(g.ID, g.Owner, "delete", "", ""); err == nil {
		t.Fatal("deleted a billing identity with a pending payment")
	}
	_ = ProtectBilling(g.ID, time.Time{})
	if err := Change(g.ID, g.Owner, "delete", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestCompactInvitationResolvesAndRevokes(t *testing.T) {
	g, err := Create("compact-owner", "Compact", false)
	if err != nil {
		t.Fatal(err)
	}
	defer Change(g.ID, "compact-owner", "delete", "", "")
	token, err := InvitationLink(g.ID, "compact-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 22 {
		t.Fatalf("token length %d", len(token))
	}
	id, err := InvitationGroup(token)
	if err != nil || id != g.ID {
		t.Fatal("could not resolve invitation", err)
	}
	if _, err := InvitationGroup("invalid"); err == nil {
		t.Fatal("invalid token resolved")
	}
	if err := AcceptLink(id, "compact-member", token); err != nil {
		t.Fatal(err)
	}
	if _, err := InvitationLink(id, "compact-owner", true); err != nil {
		t.Fatal(err)
	}
	if _, err := InvitationGroup(token); err == nil {
		t.Fatal("revoked token resolved")
	}
}
