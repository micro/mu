package auth

// The front door: somebody writes in, gets answered, and later signs up to keep
// what they already have.

import (
	"strings"
	"testing"

	"mu/internal/settings"
)

// A stranger's address gets an account, and the same address gets the same one.
//
// Two accounts for one person would mean the second message starting a second
// conversation, which is the failure the whole design exists to avoid.
func TestOneSenderGetsOneAccount(t *testing.T) {
	const addr = "stranger@example.com"
	first, err := Unclaimed(addr)
	if err != nil {
		t.Fatalf("opening an account: %v", err)
	}
	if !first.Unclaimed {
		t.Error("the account is not marked unclaimed, so it is an ordinary account " +
			"nobody signed up for")
	}
	// Address-cased differently, because a mail server is not case sensitive
	// about the domain and people type what they like.
	second, err := Unclaimed("Stranger@Example.com")
	if err != nil {
		t.Fatalf("second message: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("the same sender got two accounts (%s, %s) — their second message "+
			"starts a second conversation", first.ID, second.ID)
	}
}

// It cannot sign in. There is no password, and no password must not mean any
// password.
func TestAnUnclaimedAccountCannotSignIn(t *testing.T) {
	acc, err := Unclaimed("nopass@example.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, guess := range []string{"", " ", "password"} {
		if _, err := Login(acc.ID, guess); err == nil {
			t.Fatalf("signed in to an unclaimed account with %q", guess)
		}
	}
}

// The turns run out, and the answer that uses the last one is still given.
func TestTheTurnsRunOut(t *testing.T) {
	settings.Set("FREE_TURNS", "3")
	defer settings.Set("FREE_TURNS", "")

	acc, err := Unclaimed("counter@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := TurnsLeft(acc.ID); got != 3 {
		t.Fatalf("a new account has %d turns, want 3", got)
	}

	// Two spent, still going.
	for i := 1; i <= 2; i++ {
		if last := SpendTurn(acc.ID); last {
			t.Fatalf("turn %d reported as the last of 3", i)
		}
	}
	// The third is the last, and reports itself so the invitation goes out —
	// after the answer, not instead of it.
	if last := SpendTurn(acc.ID); !last {
		t.Error("the third turn of three did not report itself as the last, so " +
			"nobody is ever invited to sign up")
	}
	if got := TurnsLeft(acc.ID); got != 0 {
		t.Errorf("%d turns left after spending all of them", got)
	}
}

// The invitation goes out once.
func TestTheInvitationIsSentOnce(t *testing.T) {
	acc, err := Unclaimed("once@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if Invited(acc.ID) {
		t.Fatal("a new account is already marked as invited")
	}
	MarkInvited(acc.ID)
	if !Invited(acc.ID) {
		t.Error("marking an account invited did not take, so every message after " +
			"the limit sends another invitation")
	}
}

// Claiming keeps the account and everything filed under it.
func TestClaimingKeepsWhatWasThere(t *testing.T) {
	acc, err := Unclaimed("claimer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	old := acc.ID
	SpendTurn(old)

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
	if got.Turns != 0 {
		t.Errorf("%d turns carried over; a claimed account is governed by credits", got.Turns)
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
	acc, err := Unclaimed("twice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := Claim(acc.ID, "twiceclaimed", "a good password"); err != nil {
		t.Fatal(err)
	}
	if err := Claim("twiceclaimed", "somethingelse", "another password"); err == nil {
		t.Error("a claimed account was claimed again, which would let anybody with " +
			"the old invite rename somebody's account")
	}

	other, err := Unclaimed("collide@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := Claim(other.ID, "twiceclaimed", "a password"); err == nil {
		t.Error("claiming took a username that already belongs to somebody else")
	}
	if strings.TrimSpace(other.ID) == "" {
		t.Error("the account lost its id on a failed claim")
	}
}
