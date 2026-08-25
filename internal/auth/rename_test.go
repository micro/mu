package auth

import (
	"testing"
	"time"
)

// The plain case: a name somebody's software chose, replaced by one they did.
func TestAnAccountCanTakeANameItsOwnerChose(t *testing.T) {
	homeDir(t, "rename")

	if err := Create(&Account{ID: "hlorahulr", Name: "Rahul R", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("hlorahulr", "rahul"); err != nil {
		t.Fatalf("rename refused: %v", err)
	}

	if _, err := GetAccount("rahul"); err != nil {
		t.Fatal("the new name does not resolve")
	}
	if _, err := GetAccount("hlorahulr"); err == nil {
		t.Fatal("the old name still resolves to an account")
	}
	acc, _ := GetAccount("rahul")
	if acc.Name != "Rahul R" {
		t.Errorf("the display name did not survive: %q", acc.Name)
	}
}

// What the account carries has to come with it, or this is not a rename — it is
// a new account and a lost history. The stores register a renamer for exactly
// this; the test stands in for one.
func TestWhatIsFiledUnderTheOldNameFollowsIt(t *testing.T) {
	homeDir(t, "rename-follows")

	var moved [][2]string
	before := renamers
	renamers = append(renamers, func(old, new string) { moved = append(moved, [2]string{old, new}) })
	t.Cleanup(func() { renamers = before })

	if err := Create(&Account{ID: "a30006179", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("a30006179", "alireza"); err != nil {
		t.Fatal(err)
	}

	if len(moved) != 1 || moved[0] != [2]string{"a30006179", "alireza"} {
		t.Fatalf("the stores were not told to move their records: %v", moved)
	}
}

// The one that matters most. A username is a mailbox: releasing a vacated name
// hands the next person the mail still being sent to it, password resets
// included.
func TestAVacatedUsernameIsNotHandedToSomebodyElse(t *testing.T) {
	homeDir(t, "rename-hold")

	if err := Create(&Account{ID: "wa400601", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("wa400601", "wanda"); err != nil {
		t.Fatal(err)
	}

	// Not by signing up as it.
	if err := Create(&Account{ID: "wa400601", Secret: "s"}); err == nil {
		t.Error("somebody else signed up as the vacated username")
	}
	// Nor by renaming into it.
	if err := Create(&Account{ID: "someone_else", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("someone_else", "wa400601"); err == nil {
		t.Error("somebody else renamed into the vacated username")
	}
}

// The rule is the same rule, reached by a different door.
func TestRenameRefusesAUsernameTheRuleForbids(t *testing.T) {
	homeDir(t, "rename-rules")

	if err := Create(&Account{ID: "startingname", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"admin", "agent", "postmaster", "n1gger", "abc", "3834", "Agent"} {
		if err := Rename("startingname", bad); err == nil {
			t.Errorf("Rename to %q succeeded, want refused", bad)
		}
	}
	// And a name already in use by somebody here.
	if err := Create(&Account{ID: "occupied", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("startingname", "occupied"); err == nil {
		t.Error("renamed onto a username somebody is using")
	}
}

// The first change is free because the name being replaced was assigned rather
// than chosen. The second waits.
func TestTheFirstChangeIsFreeAndTheNextOneWaits(t *testing.T) {
	homeDir(t, "rename-interval")

	if err := Create(&Account{ID: "dfa57", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if at := CanRenameAt("dfa57"); !at.IsZero() {
		t.Fatal("an account that never renamed was made to wait")
	}
	if err := Rename("dfa57", "denis"); err != nil {
		t.Fatal(err)
	}
	if err := Rename("denis", "denisg"); err == nil {
		t.Fatal("a second rename was allowed immediately")
	}
	if at := CanRenameAt("denis"); at.IsZero() {
		t.Fatal("the page would offer a form that only says no")
	}

	// Once the interval has passed it is allowed again.
	acc, _ := GetAccount("denis")
	acc.Renamed = time.Now().Add(-renameEvery - time.Hour)
	if err := Rename("denis", "denisg"); err != nil {
		t.Fatalf("rename refused after the interval: %v", err)
	}
}

// Renaming to what you already are is not a rename, and must not burn the
// interval or file your own name as one you have left.
func TestRenamingToYourOwnNameChangesNothing(t *testing.T) {
	homeDir(t, "rename-noop")

	if err := Create(&Account{ID: "samename", Secret: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := Rename("samename", "samename"); err == nil {
		t.Fatal("renaming to the current name was treated as a change")
	}
	if at := CanRenameAt("samename"); !at.IsZero() {
		t.Error("a refused rename still started the interval")
	}
	acc, _ := GetAccount("samename")
	if len(acc.Former) != 0 {
		t.Errorf("a refused rename filed a former name: %v", acc.Former)
	}
}
