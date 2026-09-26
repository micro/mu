package group

import (
	"mu/internal/data"
	"mu/internal/dir"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMembershipLifecycleAndPersistence(t *testing.T) {
	g, err := Create("group-owner", "Family", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"", "stranger", "invited"} {
		if Member(g.ID, who) {
			t.Fatal("outsider admitted")
		}
	}
	if err := Change(g.ID, "group-owner", "invite", "invited", ""); err != nil {
		t.Fatal(err)
	}
	if Member(g.ID, "invited") {
		t.Fatal("invitation granted access")
	}
	_, pending, err := List("invited")
	if err != nil || len(pending) != 1 {
		t.Fatal("missing invitation", err)
	}
	if _, err := Read(g.ID, "invited"); err == nil {
		t.Fatal("invitee can read group before acceptance")
	}
	if err := Change(g.ID, "stranger", "accept", "invited", ""); err == nil {
		t.Fatal("stranger accepted somebody else's invite")
	}
	if err := Change(g.ID, "invited", "accept", "", ""); err != nil {
		t.Fatal(err)
	}
	if !Member(g.ID, "invited") {
		t.Fatal("accepted member refused")
	}
	if err := Change(g.ID, "invited", "invite", "another", ""); err == nil {
		t.Fatal("member invited without admin role")
	}
	if err := Change(g.ID, "group-owner", "remove", "group-owner", ""); err == nil {
		t.Fatal("ownerless group")
	}
	// Reload exactly the persisted representation.
	mu.Lock()
	loaded = false
	records = map[string]Group{}
	mu.Unlock()
	if err := Load(); err != nil || !Member(g.ID, "invited") {
		t.Fatal("membership lost on restart", err)
	}
	if err := Change(g.ID, "group-owner", "role", "invited", "owner"); err != nil {
		t.Fatal(err)
	}
	if err := Change(g.ID, "group-owner", "remove", "group-owner", ""); err != nil {
		t.Fatal(err)
	}
	if Member(g.ID, "group-owner") {
		t.Fatal("former owner retained access")
	}
	if err := Change(g.ID, "invited", "delete", "", ""); err != nil {
		t.Fatal(err)
	}
	if Member(g.ID, "invited") {
		t.Fatal("deleted group retained access")
	}
}
func TestExpiredInviteAndFailedSave(t *testing.T) {
	g, err := Create("failure-owner", "Private", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := Change(g.ID, "failure-owner", "invite", "expired", ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	copy := clone(records[g.ID])
	inv := copy.Invitations["expired"]
	inv.Expires = time.Now().Add(-time.Hour)
	copy.Invitations["expired"] = inv
	records[g.ID] = copy
	mu.Unlock()
	if err := Change(g.ID, "expired", "accept", "", ""); err == nil {
		t.Fatal("expired invite accepted")
	}
	// A directory at the atomic destination forces the write to fail.
	before, err := data.LoadFile(storeKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.DeleteFile(storeKey); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir.Data(), storeKey)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path); data.SaveFile(storeKey, string(before)) })
	if err := Change(g.ID, "failure-owner", "invite", "not-saved", ""); err == nil {
		t.Fatal("failed save acknowledged")
	}
	_, pending, _ := List("not-saved")
	if len(pending) != 0 {
		t.Fatal("failed write changed live membership")
	}
}
