package sms

// The verification text is the only message this instance is guaranteed to
// send somebody, and it used to spend itself on six digits.
//
// "Your code is 481920. It is good for ten minutes." — from a number nobody
// recognises, with nothing to say who sent it. Nobody saves that contact, and
// nothing has told them a reply would reach anything.

import (
	"strings"
	"testing"
)

func TestTheCodeStillTravels(t *testing.T) {
	got := intro("481920")
	if !strings.Contains(got, "481920") {
		t.Fatalf("the code is not in the message: %s", got)
	}
	if !strings.Contains(got, "ten minutes") {
		t.Errorf("how long the code lasts is no longer said: %s", got)
	}
}

// It says who it is from, and that a reply reaches something.
func TestItSaysWhoItIsAndThatAReplyWorks(t *testing.T) {
	got := intro("123456")
	if !strings.Contains(got, "Micro") {
		t.Errorf("the message does not say who sent it, so nobody saves the "+
			"number: %s", got)
	}
	if !strings.Contains(strings.ToLower(got), "reply") {
		t.Errorf("nothing says a reply reaches the agent, which is the whole "+
			"product: %s", got)
	}
}

// One segment, which means plain ASCII.
//
// A text is charged per segment: 160 characters in GSM-7, and 70 the moment one
// character falls outside that alphabet. An em dash or a curly apostrophe would
// double the price of every verification on the instance in exchange for
// typography nobody asked for.
func TestTheMessageIsOneSegment(t *testing.T) {
	got := intro("481920")

	for _, bad := range []string{"—", "–", "’", "‘", "“", "”", "…"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q is not in GSM-7, so this message costs two segments "+
				"instead of one: %s", bad, got)
		}
	}
	for i, r := range got {
		if r > 127 {
			t.Errorf("non-ASCII rune %q at %d forces UCS-2 and halves the "+
				"segment: %s", r, i, got)
			break
		}
	}
	if len(got) > 160 {
		t.Errorf("the message is %d characters, so it is charged as two: %s",
			len(got), got)
	}
}

// With no domain configured it does not trail off.
//
// origin.Self is empty until an operator sets one, and "your agent at ." is
// worse than not saying where.
func TestWithNoDomainItDoesNotTrailOff(t *testing.T) {
	t.Setenv("MU_DOMAIN", "")
	t.Setenv("PUBLIC_URL", "")
	t.Setenv("APP_URL", "")

	got := intro("111111")
	if strings.Contains(got, "at .") || strings.Contains(got, "at  ") {
		t.Errorf("the domain is empty and the sentence still has room for it: %s", got)
	}
	if !strings.Contains(got, "your agent") {
		t.Errorf("it no longer says what it is: %s", got)
	}
}

// The host is dialled by a person, so it carries no scheme.
func TestTheHostHasNoScheme(t *testing.T) {
	t.Setenv("MU_DOMAIN", "micro.mu")

	if h := selfHost(); strings.Contains(h, "://") {
		t.Errorf("selfHost = %q; https:// is four words of noise in a text message", h)
	}
	if h := selfHost(); strings.HasSuffix(h, "/") {
		t.Errorf("selfHost = %q, with a trailing slash", h)
	}
}
