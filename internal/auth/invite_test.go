package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetInvitesForTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	inviteMu.Lock()
	invites = map[string]*Invite{}
	inviteMu.Unlock()

	requestMu.Lock()
	requests = map[string]*InviteRequest{}
	requestMu.Unlock()

	return filepath.Join(home, ".mu", "data")
}

func TestInviteLifecycle(t *testing.T) {
	dataDir := resetInvitesForTest(t)

	if err := ValidateInvite(""); err == nil {
		t.Fatal("ValidateInvite accepted an empty code")
	}
	if err := ValidateInvite("missing"); err == nil {
		t.Fatal("ValidateInvite accepted an unknown code")
	}

	code, err := CreateInvite("person@example.com", "admin-1")
	if err != nil {
		t.Fatalf("CreateInvite returned error: %v", err)
	}
	if len(code) != 32 {
		t.Fatalf("CreateInvite code length = %d, want 32", len(code))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "invites.json")); err != nil {
		t.Fatalf("CreateInvite did not persist invites.json: %v", err)
	}
	if err := ValidateInvite(code); err != nil {
		t.Fatalf("ValidateInvite rejected new code: %v", err)
	}

	ConsumeInvite(code, "acct-1")
	if err := ValidateInvite(code); err == nil {
		t.Fatal("ValidateInvite accepted a consumed code")
	}

	list := ListInvites()
	if len(list) != 1 {
		t.Fatalf("ListInvites returned %d invites, want 1", len(list))
	}
	if list[0].UsedBy != "acct-1" || list[0].UsedAt.IsZero() {
		t.Fatalf("ConsumeInvite did not record usage: %#v", list[0])
	}
}

func TestInviteRequestLifecycleAndOrdering(t *testing.T) {
	dataDir := resetInvitesForTest(t)

	if err := CreateInviteRequest("   ", "reason", "127.0.0.1"); err == nil {
		t.Fatal("CreateInviteRequest accepted a blank email")
	}

	longReason := strings.Repeat("x", 501)
	if err := CreateInviteRequest(" First@Example.COM ", longReason, "1.1.1.1"); err != nil {
		t.Fatalf("CreateInviteRequest returned error: %v", err)
	}
	if err := CreateInviteRequest("second@example.com", "second", "2.2.2.2"); err != nil {
		t.Fatalf("CreateInviteRequest returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "invite_requests.json")); err != nil {
		t.Fatalf("CreateInviteRequest did not persist invite_requests.json: %v", err)
	}

	MarkInviteRequestSent(" FIRST@example.com ")
	if err := CreateInviteRequest("first@example.com", "updated", "3.3.3.3"); err != nil {
		t.Fatalf("CreateInviteRequest update returned error: %v", err)
	}

	list := ListInviteRequests()
	if len(list) != 2 {
		t.Fatalf("ListInviteRequests returned %d requests, want 2", len(list))
	}
	if list[0].Email != "second@example.com" || list[0].Invited {
		t.Fatalf("pending requests should sort before invited ones, got first %#v", list[0])
	}
	if list[1].Email != "first@example.com" || !list[1].Invited {
		t.Fatalf("invited request should sort last, got %#v", list[1])
	}
	if got := len(list[1].Reason); got != len("updated") {
		t.Fatalf("duplicate request with a non-empty reason should update reason length to %d, got %d", len("updated"), got)
	}
	if list[1].IP != "3.3.3.3" {
		t.Fatalf("duplicate request with a non-empty IP should update IP, got %q", list[1].IP)
	}

	DeleteInviteRequest(" FIRST@example.com ")
	list = ListInviteRequests()
	if len(list) != 1 || list[0].Email != "second@example.com" {
		t.Fatalf("DeleteInviteRequest left requests %#v, want only second@example.com", list)
	}
}
