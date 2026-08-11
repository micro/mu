package phone

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalise(t *testing.T) {
	t.Setenv("SMS_DEFAULT_COUNTRY", "")
	cases := []struct{ in, want string }{
		{"+44 7700 900123", "+447700900123"},
		{"+1 (555) 339-0242", "+15553390242"},
		{"+447700900123", "+447700900123"},
		{"07700900123", ""},          // no country code and no default: ambiguous, so refused
		{"nonsense", ""},             //
		{"", ""},                     //
		{"+1", ""},                   // too short to be anybody
		{"+1234567890123456789", ""}, // too long to be anybody
	}
	for _, c := range cases {
		if got := Normalise(c.in); got != c.want {
			t.Errorf("Normalise(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormaliseUsesTheInstanceDefaultCountry(t *testing.T) {
	// Guessing a country is how you text a stranger in another one, so only an
	// operator's explicit default rescues a local number.
	t.Setenv("SMS_DEFAULT_COUNTRY", "44")
	if got := Normalise("07700900123"); got != "+447700900123" {
		t.Errorf("Normalise(\"07700900123\") = %q with SMS_DEFAULT_COUNTRY=44", got)
	}
}

// TestTheStorageNamespaceIsStillSMS guards live data.
//
// These records moved package but not storage. Somebody verified their phone
// number before internal/phone existed, and the proof is filed under the "sms"
// namespace. Renaming it to match the new package would read as tidying and
// would silently orphan every verified number on every running instance — the
// symptom being a message arriving from a number its owner had proved was
// theirs, and going to the operator instead.
func TestTheStorageNamespaceIsStillSMS(t *testing.T) {
	if ns != "sms" {
		t.Fatalf("storage namespace is %q — every number verified before this "+
			"package existed is filed under \"sms\" and would be orphaned", ns)
	}
	if numbers != "numbers" || claims != "claims" || instance != "instance" {
		t.Errorf("a collection was renamed: %q %q %q", numbers, claims, instance)
	}
}

func TestOwnershipRoundTrips(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".mu", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SMS_DEFAULT_COUNTRY", "")

	const me, mine = "someone", "+447700900999"

	if Verified(me, mine) {
		t.Fatal("a number was verified before anybody verified it")
	}
	if err := Verify(me, mine); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !Verified(me, mine) {
		t.Error("a number verified did not read back as verified")
	}
	// Spelling must not matter: the same number written differently is the same
	// number, which is the whole reason Normalise runs before every lookup.
	if !Verified(me, "+44 7700 900999") {
		t.Error("the same number spelled differently did not match")
	}
	if got := Owner(mine); got != me {
		t.Errorf("Owner = %q, want %q", got, me)
	}

	found := false
	for _, n := range Numbers(me) {
		if n == mine {
			found = true
		}
	}
	if !found {
		t.Errorf("Numbers(%q) = %v, missing the number just verified", me, Numbers(me))
	}

	// A phone is not yours forever.
	Forget(me, mine)
	if Verified(me, mine) {
		t.Error("a forgotten number is still verified")
	}
	if got := Owner(mine); got == me {
		t.Error("a forgotten number still has its old claim")
	}

	if err := Verify(me, "not a number"); err == nil {
		t.Error("something that is not a phone number was verified")
	}
}
