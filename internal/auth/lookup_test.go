package auth

// Resolving a name to an account must not depend on map order.
//
// GetAccountByName matched name-or-id in one pass over a map and returned the
// first hit. Go randomises map iteration, so when one account's id was the same
// word as another account's name, which one came back was undefined — and this
// is the function a credit transfer resolves its recipient with. Credits went
// to whichever account the runtime reached first, and the receipt recorded the
// bare id, so a wrong answer looked exactly like a right one.
//
// Two accounts answering to one word is a question for the person, not
// something to settle by picking one.

import (
	"os"
	"strings"
	"testing"
)

func lookupAccount(t *testing.T, id, name string) {
	t.Helper()
	if err := Create(&Account{ID: id, Name: name, Secret: "s"}); err != nil {
		t.Fatal(err)
	}
}

func TestANameThatIsAlsoSomebodysIdIsRefusedNotGuessed(t *testing.T) {
	dir, err := os.MkdirTemp("", "mu-lookup")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	defer os.RemoveAll(dir)

	// The live shape: one account whose id is "asim", another whose display
	// name is "asim" and whose id is a number.
	lookupAccount(t, "asim", "Asim")
	lookupAccount(t, "3834", "asim")

	// Whatever the map does today, it must do the same thing every time, and
	// "pick one" is not an acceptable answer when the caller is moving money.
	for i := 0; i < 50; i++ {
		acc, err := GetAccountByName("asim")
		if err == nil {
			t.Fatalf("run %d: resolved ambiguously to %q (%s) instead of refusing",
				i, acc.ID, acc.Name)
		}
		if !strings.Contains(err.Error(), "exact id") {
			t.Fatalf("run %d: unhelpful error %q — it has to say how to disambiguate", i, err)
		}
	}

	// The exact id still resolves, which is the way out of the ambiguity.
	if acc, err := GetAccountByName("3834"); err != nil || acc.ID != "3834" {
		t.Errorf("the exact id did not resolve: %v", err)
	}
}

func TestAnUnambiguousNameStillResolves(t *testing.T) {
	dir, err := os.MkdirTemp("", "mu-lookup2")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	defer os.RemoveAll(dir)

	lookupAccount(t, "u-1", "Solo")

	for _, q := range []string{"Solo", "solo", "  SOLO ", "u-1"} {
		acc, err := GetAccountByName(q)
		if err != nil || acc.ID != "u-1" {
			t.Errorf("GetAccountByName(%q) = %v, %v; want u-1", q, acc, err)
		}
	}
	if _, err := GetAccountByName("nobody"); err == nil {
		t.Error("a name nobody has resolved to an account")
	}
	if _, err := GetAccountByName(""); err == nil {
		t.Error("an empty name resolved to an account")
	}
}

// Two accounts sharing a display name is the other half of the same problem.
func TestTwoAccountsWithTheSameNameAreAmbiguous(t *testing.T) {
	dir, err := os.MkdirTemp("", "mu-lookup3")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	defer os.RemoveAll(dir)

	lookupAccount(t, "twin-a", "Twin")
	lookupAccount(t, "twin-b", "Twin")

	if _, err := GetAccountByName("Twin"); err == nil {
		t.Error("a name two accounts share resolved to one of them")
	}
}
