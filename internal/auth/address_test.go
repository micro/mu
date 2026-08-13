package auth

import "testing"

func TestNormaliseAddressTakesOneSpellingAndRefusesTheRest(t *testing.T) {
	for in, want := range map[string]string{
		"  ASIM@Aslam.me ":  "asim@aslam.me",
		"a.b+tag@sub.co.uk": "a.b+tag@sub.co.uk",
		"":                  "",
		"asim":              "",
		"@aslam.me":         "",
		"asim@":             "",
		"asim@aslam":        "", // no dot, so not a domain anybody can deliver to
		"asim@@aslam.me":    "", // two ats is two addresses or none
		"asim@.me":          "",
		"asim@aslam..me":    "",
		"Asim <a@b.com>":    "", // a header, not an address
		"a@b.com, c@d.com":  "",
	} {
		if got := NormaliseAddress(in); got != want {
			t.Errorf("NormaliseAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTheAccountsOwnEmailCountsAsProvedAndAnUnverifiedOneDoesNot(t *testing.T) {
	acc := &Account{ID: "asim", Email: "asim@aslam.me", EmailVerified: true}
	if !acc.Owns("ASIM@Aslam.me ") {
		t.Error("the address on the account, verified, is not being read as theirs")
	}

	acc.EmailVerified = false
	if acc.Owns("asim@aslam.me") {
		t.Error("an unverified account email counts as proof, which is no proof at all")
	}
}

func TestExtraAddressesAreOwnedAndTheAccountEmailComesFirst(t *testing.T) {
	acc := &Account{
		ID: "asim", Email: "asim@aslam.me", EmailVerified: true,
		Addresses: []string{"asim@work.com", "ASIM@ASLAM.ME"},
	}
	got := acc.Verified()
	if len(got) != 2 {
		t.Fatalf("want the two distinct addresses, got %v", got)
	}
	if got[0] != "asim@aslam.me" {
		t.Errorf("the account's own address should lead, got %v", got)
	}
	if !acc.Owns("asim@work.com") {
		t.Error("a proved address is not owned")
	}
	if acc.Owns("someone@example.com") {
		t.Error("an address nobody proved is owned")
	}
}

func TestNilAccountOwnsNothing(t *testing.T) {
	var acc *Account
	if acc.Owns("asim@aslam.me") || acc.Verified() != nil {
		t.Error("a missing account is answering questions about addresses")
	}
}

func TestAnAddressBelongsToOneAccount(t *testing.T) {
	mutex.Lock()
	accounts["one"] = &Account{ID: "one"}
	accounts["two"] = &Account{ID: "two"}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "one")
		delete(accounts, "two")
		mutex.Unlock()
	})

	if err := AddVerifiedAddress("one", "shared@example.com"); err != nil {
		t.Fatalf("adding: %v", err)
	}
	// Twice is not an error — the second proof says the same thing as the first.
	if err := AddVerifiedAddress("one", "SHARED@example.com"); err != nil {
		t.Fatalf("proving the same address again: %v", err)
	}
	if n := len(VerifiedAddresses("one")); n != 1 {
		t.Fatalf("want one address, got %d", n)
	}

	// The second account cannot take it, because AccountForAddress decides whose
	// mail arrived and two claimants make that a coin toss.
	if err := AddVerifiedAddress("two", "shared@example.com"); err == nil {
		t.Error("two accounts both hold the same address")
	}
	if acc := AccountForAddress("shared@example.com"); acc == nil || acc.ID != "one" {
		t.Errorf("want the address to belong to one, got %v", acc)
	}
}

func TestAProvedAddressCanBeGivenUpButTheAccountsOwnCannot(t *testing.T) {
	mutex.Lock()
	accounts["asim"] = &Account{ID: "asim", Email: "asim@aslam.me", EmailVerified: true}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "asim")
		mutex.Unlock()
	})

	if err := AddVerifiedAddress("asim", "asim@work.com"); err != nil {
		t.Fatalf("adding: %v", err)
	}
	if err := RemoveVerifiedAddress("asim", "asim@work.com"); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if OwnsAddress("asim", "asim@work.com") {
		t.Error("a removed address is still owned")
	}

	// The sign-in address is not this operation's to remove: it is what a
	// password reset goes to, and it is changed at /account by proving a new one.
	if err := RemoveVerifiedAddress("asim", "asim@aslam.me"); err == nil {
		t.Error("the account's own address was removed from the wrong place")
	}
	if !OwnsAddress("asim", "asim@aslam.me") {
		t.Error("the account's own address stopped being theirs")
	}
}

func TestAddingAnAddressLeavesTheSignInAddressAlone(t *testing.T) {
	mutex.Lock()
	accounts["asim"] = &Account{ID: "asim", Email: "asim@aslam.me", EmailVerified: true}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		delete(accounts, "asim")
		mutex.Unlock()
	})

	if err := AddVerifiedAddress("asim", "attacker@example.com"); err != nil {
		t.Fatalf("adding: %v", err)
	}
	// The whole security argument for keeping these apart. Proving you can read
	// a mailbox must never re-point where a password reset goes, or the code
	// flow in service/email is an account takeover with a friendly name.
	acc, _ := GetAccount("asim")
	if acc.Email != "asim@aslam.me" || !acc.EmailVerified {
		t.Errorf("proving an address changed the address on the account: %q", acc.Email)
	}
}
