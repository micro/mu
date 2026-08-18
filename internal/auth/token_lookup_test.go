package auth

// Validating a token was O(every token on the instance) of bcrypt, holding the
// lock everything else in this package needs.
//
// bcrypt at cost 10 is about 60ms by design, and the loop tried each token
// twice — unpadded then padded — so a hundred tokens was a double-digit number
// of seconds per call, with every page load, session lookup and account read on
// the instance queued behind it. It presented as "the admin pages are slow" and
// was actually "one agent calling /mcp stalls the site".

import (
	"testing"
	"time"
)

func TestValidatingATokenDoesNotScanEveryToken(t *testing.T) {
	owner := "token-speed"
	if err := Create(&Account{ID: owner, Name: owner, Secret: "x"}); err != nil {
		t.Skipf("cannot make an account here: %v", err)
	}

	// Enough to be obviously slow the old way and quick the new way. Each one
	// costs a bcrypt to issue, so this is deliberately not a hundred.
	var last string
	for i := 0; i < 12; i++ {
		_, raw, err := CreateToken(owner, "t", nil, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		last = raw
	}

	start := time.Now()
	for i := 0; i < 20; i++ {
		got, err := ValidatePAT(last)
		if err != nil {
			t.Fatalf("a token this instance issued did not validate: %v", err)
		}
		if got != owner {
			t.Fatalf("token resolved to %q, want %q", got, owner)
		}
	}
	each := time.Since(start) / 20

	// A map lookup and a sha256 are microseconds. One bcrypt is ~60ms, and the
	// old path did up to two per stored token. Ten milliseconds is far above
	// the former and far below a single comparison, so it separates them on any
	// machine without pinning a number to this one.
	if each > 10*time.Millisecond {
		t.Errorf("validating a token takes %v with 12 of them stored — it is still "+
			"comparing against each one, which is seconds per call on a real instance", each)
	}
}

// A token that does not exist is refused, quickly, and without a scan.
func TestAnUnknownTokenIsRefused(t *testing.T) {
	if _, err := ValidatePAT("not-a-token-anybody-issued"); err == nil {
		t.Error("an invented token validated")
	}
	if _, err := ValidatePAT(""); err == nil {
		t.Error("an empty token validated")
	}
}

// Padding is stripped on both sides, so a token copied with or without its
// base64 '=' still works — the behaviour the old double comparison existed for.
func TestPaddingDoesNotDecideWhetherATokenWorks(t *testing.T) {
	owner := "token-padding"
	if err := Create(&Account{ID: owner, Name: owner, Secret: "x"}); err != nil {
		t.Skipf("cannot make an account here: %v", err)
	}
	_, raw, err := CreateToken(owner, "t", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range []string{raw, raw + "=", raw + "=="} {
		if _, err := ValidatePAT(form); err != nil {
			t.Errorf("token with %d padding characters was refused: %v", len(form)-len(raw), err)
		}
	}
}
