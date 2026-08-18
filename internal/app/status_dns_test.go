package app

import (
	"testing"
	"time"
)

// The status page asks DNS two questions. It must not wait for the answers.
//
// net.LookupTXT has no deadline, and /status is public — a resolver that has
// stopped answering turned every hit on it into a request held open for as long
// as the stub resolver kept retrying. /admin/server, which embeds the same
// block, was taking twenty-five seconds.
//
// The name here does not resolve, which is the case that used to be slow: a
// negative answer is the one that waits for every retry.
func TestTheStatusPageDoesNotWaitOnDNS(t *testing.T) {
	const question = "there-is-no-such-name.invalid"

	start := time.Now()
	ok, known := dnsAnswer(question, "v=spf1")
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Fatalf("the first ask blocked for %s", took)
	}
	if known {
		t.Fatal("an answer that has not come back yet is reported as known")
	}
	if ok {
		t.Fatal("an unasked question answered yes")
	}

	// Asking again while the lookup is in flight is just as cheap.
	start = time.Now()
	dnsAnswer(question, "v=spf1")
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Fatalf("a second ask blocked for %s", took)
	}
}

// A row for an answer nobody has yet says so, rather than reporting the record
// as missing. "Not checked" and "not published" are different things.
func TestAnUncheckedRecordSaysItIsBeingChecked(t *testing.T) {
	c := dnsStatus("SPF DNS Record", "another-name-that-does-not-exist.invalid", "v=spf1")
	if c.Status {
		t.Fatal("an unchecked record is reported as present")
	}
	if c.Details == "" {
		t.Fatal("an unchecked record is indistinguishable from a missing one")
	}
}
