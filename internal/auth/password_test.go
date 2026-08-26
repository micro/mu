package auth

import "testing"

// The lockout, written down so it cannot come back.
//
// An account made through Google holds a bcrypt hash of a random string it was
// never shown. That is a real hash, so CheckSecret reports a wrong password
// rather than an absent one, and the owner of the wallet key was told — every
// time — that they had mistyped their own password.
func TestAnAccountWithARandomSecretCanStillSetOne(t *testing.T) {
	homeDir(t, "password-google")

	// Exactly how findOrCreateGoogleAccount makes one.
	if err := Create(&Account{ID: "googleuser", Name: "G", Secret: randomish}); err != nil {
		t.Fatal(err)
	}

	if HasSecret("googleuser") {
		t.Error("an account nobody chose a password for reports having one")
	}
	// The old advice was "set a password first", and this is that being possible.
	if err := SetSecret("googleuser", "a good password"); err != nil {
		t.Fatalf("could not set a password: %v", err)
	}
	if !HasSecret("googleuser") {
		t.Error("a password was set and the account does not report having one")
	}
	if err := CheckSecret("googleuser", "a good password"); err != nil {
		t.Fatalf("the password that was just set does not check out: %v", err)
	}
	// And the random one it was created with is gone.
	if err := CheckSecret("googleuser", randomish); err == nil {
		t.Error("the secret it was created with still works after a password was set")
	}
}

// Short passwords are refused, and refusing one leaves the old one alone.
func TestAShortPasswordIsRefusedAndChangesNothing(t *testing.T) {
	homeDir(t, "password-short")

	if err := Create(&Account{ID: "shortpw", Secret: "the first one", SecretSet: true}); err != nil {
		t.Fatal(err)
	}
	if err := SetSecret("shortpw", "abc"); err == nil {
		t.Fatal("a three-character password was accepted")
	}
	if err := CheckSecret("shortpw", "the first one"); err != nil {
		t.Error("a refused change replaced the password anyway")
	}
}

// Signing up chooses a password, so the account says it has one.
func TestChoosingAPasswordAtSignupIsRecorded(t *testing.T) {
	homeDir(t, "password-signup")

	if err := Create(&Account{ID: "chosepw", Secret: "chosen at signup", SecretSet: true}); err != nil {
		t.Fatal(err)
	}
	if !HasSecret("chosepw") {
		t.Error("an account that chose a password at signup reports having none")
	}
}

// A stand-in for the 24 random characters a Google signup gets.
const randomish = "Zk3xQ8vR1mT7pL0aB5nH2sYw"
