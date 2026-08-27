package auth

// Claiming: an account that was opened on somebody's behalf becomes theirs,
// keeping everything it already holds.

import (
	"strings"
	"testing"
	"time"
)

// unclaimed builds the kind of account Claim exists to convert.
//
// Production stopped creating them — see the package comment — but instances
// that ran the old path still hold some, and Claim is how the conversation
// somebody had by email survives their signing up. So the fixture is here
// rather than the behaviour being deleted along with the door that made them.
func unclaimed(t *testing.T, addr string) *Account {
	t.Helper()
	addr = NormaliseAddress(addr)
	id := "unclaimed_" + strings.SplitN(addr, "@", 2)[0]

	mutex.Lock()
	acc := &Account{
		ID:              id,
		Name:            strings.SplitN(addr, "@", 2)[0],
		Created:         time.Now(),
		Email:           addr,
		EmailVerified:   true,
		EmailVerifiedAt: time.Now(),
		// No secret: bcrypt cannot match a hash of nothing against any input,
		// which is what stops one of these being signed in to.
		Unclaimed: true,
	}
	accounts[id] = acc
	mutex.Unlock()
	return acc
}

// It cannot sign in. There is no password, and no password must not mean any
// password.
func TestAnUnclaimedAccountCannotSignIn(t *testing.T) {
	acc := unclaimed(t, "nopass@example.com")
	for _, guess := range []string{"", " ", "password"} {
		if _, err := Login(acc.ID, guess); err == nil {
			t.Fatalf("signed in to an unclaimed account with %q", guess)
		}
	}
}

// Claiming keeps the account and everything filed under it.
func TestClaimingKeepsWhatWasThere(t *testing.T) {
	acc := unclaimed(t, "claimer@example.com")
	old := acc.ID

	// The record follows. Registered the way internal/thread registers it.
	var renamedFrom, renamedTo string
	Renamed(func(o, n string) { renamedFrom, renamedTo = o, n })

	if err := Claim(old, "claimer", "a good password"); err != nil {
		t.Fatalf("claiming: %v", err)
	}

	if _, err := GetAccount(old); err == nil {
		t.Error("the old id still resolves, so there are two accounts for one person")
	}
	got, err := GetAccount("claimer")
	if err != nil {
		t.Fatalf("the claimed account is not there under its new name: %v", err)
	}
	if got.Unclaimed {
		t.Error("the account is still marked unclaimed after being claimed")
	}
	if got.Email != "claimer@example.com" {
		t.Errorf("the address did not come with it: %q", got.Email)
	}
	// And it can sign in now, which is the point of claiming it.
	if _, err := Login("claimer", "a good password"); err != nil {
		t.Errorf("cannot sign in to the account just claimed: %v", err)
	}
	if renamedFrom != old || renamedTo != "claimer" {
		t.Errorf("the record was told %q → %q, want %q → %q — the conversation "+
			"somebody was invited to keep is filed under an id that no longer exists",
			renamedFrom, renamedTo, old, "claimer")
	}
}

// An account that has been claimed cannot be claimed again, and one person's
// claim cannot take another's username.
func TestClaimingIsOnlyEverOnce(t *testing.T) {
	acc := unclaimed(t, "twice@example.com")
	if err := Claim(acc.ID, "twiceclaimed", "a good password"); err != nil {
		t.Fatal(err)
	}
	if err := Claim("twiceclaimed", "somethingelse", "another password"); err == nil {
		t.Error("a claimed account was claimed again, which would let anybody with " +
			"the old invite rename somebody's account")
	}

	other := unclaimed(t, "collide@example.com")
	if err := Claim(other.ID, "twiceclaimed", "a password"); err == nil {
		t.Error("claiming took a username that already belongs to somebody else")
	}
	if strings.TrimSpace(other.ID) == "" {
		t.Error("the account lost its id on a failed claim")
	}
}
