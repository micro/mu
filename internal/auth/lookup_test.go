package auth

// Money resolves by username, and a username is the account id.
//
// The two fields are not interchangeable. ID is the username: unique, enforced
// unique at signup, validated by ValidateUsername, and what the product shows
// as @handle, as the mail local part, as author_id. Name is a display name —
// free text, not unique, not validated, and at Google sign-in it is whatever
// the Google profile happens to say.
//
// GetAccountByName matched either and returned the first hit from a map, so
// "pay asim" credited the account whose username is asim, or a different
// account that had typed "asim" into its display name, depending on Go's
// randomised iteration order that call. A transfer of 100 credits went to the
// second kind.
//
// Matching a display name was the error underneath the coin flip. If a display
// name could receive money, setting yours to a popular handle would be a way to
// collect other people's.

import (
	"os"
	"testing"
)

func lookupAccount(t *testing.T, id, name string) {
	t.Helper()
	if err := Create(&Account{ID: id, Name: name, Secret: "s"}); err != nil {
		t.Fatal(err)
	}
}

func homeDir(t *testing.T, tag string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mu-lookup-"+tag)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.RemoveAll(dir) })
}

// The exact shape that lost the credits: username "asim" on one account,
// display name "Asim" on another.
func TestADisplayNameNeverAnswersForAUsername(t *testing.T) {
	homeDir(t, "collide")

	lookupAccount(t, "asim", "Asim Aslam") // the username

	// The colliding account, installed rather than created. 3834 was a real id
	// on micro.mu — it starts with a digit, which ValidateUsername has always
	// refused, and it existed because Create did not ask until now. It has
	// since been deleted and no new one can be made, but the shape it caused is
	// what this test is for, and any id that shares a word with a display name
	// can still cause it. Creating the fixture through Create would now fail,
	// and would be testing the rule rather than the collision.
	SetAccountForTest(&Account{ID: "3834", Name: "Asim", Secret: "s"})
	t.Cleanup(func() { RemoveAccountForTest("3834") })

	// Every time, not most times. The bug was that this varied.
	for i := 0; i < 50; i++ {
		acc, err := AccountByUsername("asim")
		if err != nil {
			t.Fatalf("run %d: the username did not resolve: %v", i, err)
		}
		if acc.ID != "asim" {
			t.Fatalf("run %d: \"asim\" resolved to account %q (%q) — that is somebody else's money",
				i, acc.ID, acc.Name)
		}
	}

	// The display name is not an identifier and resolves to nothing.
	if acc, err := AccountByUsername("Asim"); err == nil && acc.ID != "asim" {
		t.Errorf("a display name resolved to account %q", acc.ID)
	}
	// The other account is reachable, by its own username.
	if acc, err := AccountByUsername("3834"); err != nil || acc.ID != "3834" {
		t.Errorf("the second account is unreachable by its username: %v", err)
	}
}

func TestUsernameLookupIsCaseInsensitiveAndTrimmed(t *testing.T) {
	homeDir(t, "case")
	lookupAccount(t, "solo", "Solo Display")

	for _, q := range []string{"solo", "SOLO", "  Solo "} {
		acc, err := AccountByUsername(q)
		if err != nil || acc.ID != "solo" {
			t.Errorf("AccountByUsername(%q) = %v, %v; want solo", q, acc, err)
		}
	}
	for _, q := range []string{"", "nobody", "Solo Display"} {
		if acc, err := AccountByUsername(q); err == nil {
			t.Errorf("AccountByUsername(%q) resolved to %q", q, acc.ID)
		}
	}
}

// Two accounts can share a display name — nothing stops it — and that must
// remain harmless, because nothing resolves by one.
func TestSharedDisplayNamesAreHarmless(t *testing.T) {
	homeDir(t, "twins")
	lookupAccount(t, "twin_a", "Twin")
	lookupAccount(t, "twin_b", "Twin")

	a, err := AccountByUsername("twin_a")
	if err != nil || a.ID != "twin_a" {
		t.Fatalf("twin_a did not resolve: %v", err)
	}
	b, err := AccountByUsername("twin_b")
	if err != nil || b.ID != "twin_b" {
		t.Fatalf("twin_b did not resolve: %v", err)
	}
	if _, err := AccountByUsername("Twin"); err == nil {
		t.Error("a shared display name resolved to an account")
	}
}
