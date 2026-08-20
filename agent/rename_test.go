package agent

// Renaming an agent moves its address with it.
//
// The tag was assigned once at creation and never revisited, and the tag is
// what Slug returns — so it was the page (/agent/test), the endpoint (POST
// /agent/test) and the mail address (you+test@) all at once. Renaming the agent
// to Research moved the label on /agents and nothing else: the page you were
// linked to and the endpoint the Connect page told you to POST to still said
// test, and no screen showed the word "test" anywhere, so there was nothing to
// read that explained why.
//
// The other half is that an address must not vanish. Whatever was pointing at
// the old one — a cron job, a curl in somebody's notes, mail already in flight —
// keeps resolving.

import (
	"strings"
	"testing"
)

// renamed makes an agent, renames it, and hands back what it became.
func renamed(t *testing.T, owner, from, to string) *Agent {
	t.Helper()
	a, _, err := CreateAgent(owner, from, External, "", "", nil, false)
	if err != nil {
		t.Fatalf("create %q: %v", from, err)
	}
	got, err := UpdateAgent(owner, a.ID, to, "", "", nil)
	if err != nil {
		t.Fatalf("rename to %q: %v", to, err)
	}
	return got
}

// The endpoint says what the agent is called.
//
// This is the bug as reported: renamed the agent, the endpoint stayed the same.
func TestRenamingAnAgentMovesItsEndpoint(t *testing.T) {
	a := renamed(t, "rename-endpoint", "Test", "Research")

	if a.Tag != "research" {
		t.Errorf("tag is %q after renaming Test to Research — the address, the page "+
			"and POST /agent/<name> all read off this", a.Tag)
	}
	if got := Slug(a); got != "research" {
		t.Errorf("slug is %q, so the endpoint is still POST /agent/%s", got, got)
	}
	if got := Path(a.Owner, a.ID); got != "/agent/research" {
		t.Errorf("page is %s, want /agent/research", got)
	}
}

// The old name keeps working, because it is an address and somebody wrote it
// down.
func TestTheOldNameStillResolves(t *testing.T) {
	const owner = "rename-old"
	a := renamed(t, owner, "Test", "Research")

	id, ok := BySlug(owner, "test")
	if !ok || id != a.ID {
		t.Errorf("BySlug(test) = %q,%v — a link or a cron job written before the "+
			"rename now 404s", id, ok)
	}
	if got := ForTag(owner, "test"); got == nil || got.ID != a.ID {
		t.Error("mail to you+test@ no longer reaches the agent it was addressed to")
	}
	// And the new one, obviously.
	if id, ok := BySlug(owner, "research"); !ok || id != a.ID {
		t.Errorf("BySlug(research) = %q,%v", id, ok)
	}
}

// Editing anything else must not churn the address.
//
// The prompt, the description and the tool scope all arrive through the same
// call. If any of them moved the tag, every save would break the agent's
// address and pile up a former one.
func TestEditingWithoutRenamingLeavesTheAddressAlone(t *testing.T) {
	const owner = "rename-noop"
	a, _, err := CreateAgent(owner, "Research", External, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	tag := a.Tag

	for i, prompt := range []string{"be brief", "be brisk", "be brief"} {
		got, err := UpdateAgent(owner, a.ID, "Research", prompt, "notes", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Tag != tag {
			t.Fatalf("edit %d moved the tag from %q to %q", i, tag, got.Tag)
		}
		if len(got.Former) != 0 {
			t.Fatalf("edit %d recorded a former tag %v for a name that did not change",
				i, got.Former)
		}
	}
}

// A rename cannot take a name another agent is using, or the two would share an
// inbox — which is the thing tagFor exists to prevent.
func TestARenameCannotTakeAnotherAgentsName(t *testing.T) {
	const owner = "rename-collide"
	first, _, err := CreateAgent(owner, "Research", External, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	second := renamed(t, owner, "Test", "Research")

	if second.Tag == first.Tag {
		t.Fatalf("both agents answer at %q, so one reads the other's mail", first.Tag)
	}
	if second.Tag != "research2" {
		t.Errorf("second agent got %q, want research2", second.Tag)
	}
	// The word itself still belongs to the agent that had it.
	if id, _ := BySlug(owner, "research"); id != first.ID {
		t.Error("renaming a second agent to Research took the name off the first")
	}
}

// A former name cannot be taken either, or an old link would start landing on a
// different agent than it was written for.
func TestARenameCannotTakeAFormerName(t *testing.T) {
	const owner = "rename-former"
	first := renamed(t, owner, "Test", "Research") // "test" is now first's former
	second := renamed(t, owner, "Scratch", "Test")

	if second.Tag == "test" {
		t.Fatal("a second agent took a name the first still answers to — an old " +
			"link now reaches the wrong agent")
	}
	// And the old link still goes where it was written to go.
	if id, _ := BySlug(owner, "test"); id != first.ID {
		t.Error("BySlug(test) no longer reaches the agent that was called Test")
	}
}

// A live name beats a former one, so an agent that legitimately holds a word is
// never shadowed by whoever used to.
func TestALiveNameBeatsAFormerOne(t *testing.T) {
	const owner = "rename-live"
	old := renamed(t, owner, "Research", "Reading") // "research" is now former
	fresh, _, err := CreateAgent(owner, "Research", External, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Tag != "research" {
		t.Fatalf("the freed name was not reused: got %q", fresh.Tag)
	}
	if id, _ := BySlug(owner, "research"); id != fresh.ID {
		t.Error("/agent/research reaches the agent that used to hold the name " +
			"rather than the one that holds it now")
	}
	if got := ForTag(owner, "research"); got == nil || got.ID != fresh.ID {
		t.Error("mail to you+research@ goes to the previous holder")
	}
	// The one that moved on is still reachable at what it is called now.
	if id, _ := BySlug(owner, "reading"); id != old.ID {
		t.Error("the renamed agent is not at its own name")
	}
}

// Renaming repeatedly does not grow the record without bound.
func TestFormerNamesAreCapped(t *testing.T) {
	const owner = "rename-many"
	a, _, err := CreateAgent(owner, "n0", External, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= formerLimit+4; i++ {
		if a, err = UpdateAgent(owner, a.ID, "n"+strings.Repeat("x", i), "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(a.Former) > formerLimit {
		t.Errorf("%d former names kept, cap is %d", len(a.Former), formerLimit)
	}
	// The most recent ones are the ones kept — a name eight renames ago is not
	// the one anybody has written down.
	if last := a.Former[len(a.Former)-1]; last != "n"+strings.Repeat("x", formerLimit+3) {
		t.Errorf("the newest former name is %q, so the list dropped the wrong end", last)
	}
}

// The rename survives a round trip through storage, or it only holds until the
// page is reloaded.
func TestARenameIsWrittenDown(t *testing.T) {
	const owner = "rename-persist"
	a := renamed(t, owner, "Test", "Research")

	got := For(owner, a.ID)
	if got == nil {
		t.Fatal("the agent is gone")
	}
	if got.Tag != "research" {
		t.Errorf("reloaded tag is %q, want research", got.Tag)
	}
	if len(got.Former) != 1 || got.Former[0] != "test" {
		t.Errorf("reloaded former names are %v, want [test] — the old address stops "+
			"resolving on the next read", got.Former)
	}
}
