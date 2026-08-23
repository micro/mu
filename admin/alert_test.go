package admin

// The two properties that decide whether anybody keeps this switched on.
//
// Everything else here is plumbing: read a number, compare it, send a message.
// What makes an alerting system usable is that it does not repeat itself and
// does not announce history as news, and both of those are easy to get wrong in
// a way that only shows up in production, once, when somebody turns it off.

import (
	"strings"
	"testing"
	"time"
)

// An alert that has just fired does not fire again.
//
// This is what makes a threshold safe to set low. Without it, "disk is 85%
// full" is a message every five minutes until somebody clears the disk — and a
// channel that does that once teaches its reader to ignore it, which costs the
// next alert as well as this one.
func TestAnAlertDoesNotRepeatWhileItIsStillTrue(t *testing.T) {
	resetFired()

	if !fired("disk") {
		t.Fatal("the first one did not go out")
	}
	for i := 0; i < 5; i++ {
		if fired("disk") {
			t.Fatalf("it fired again immediately (attempt %d)", i+2)
		}
	}
	// A different thing is a different alert, or one noisy check silences the
	// rest.
	if !fired("rate:instance") {
		t.Error("an unrelated alert was suppressed by the cooldown on another")
	}
}

// And it fires again once the cooldown has passed.
func TestAnAlertComesBackAfterTheCooldown(t *testing.T) {
	resetFired()
	if !fired("disk") {
		t.Fatal("the first one did not go out")
	}

	// Wound back past any plausible cooldown rather than waiting one out.
	firedMu.Lock()
	lastFired["disk"] = time.Now().Add(-30 * 24 * time.Hour)
	firedMu.Unlock()

	if !fired("disk") {
		t.Error("it never came back, so a problem that persists is reported once and forgotten")
	}
}

func resetFired() {
	firedMu.Lock()
	lastFired = map[string]time.Time{}
	firedMu.Unlock()
}

// Switching this on does not announce everything that already happened.
//
// somethingWrong watches a count that is not zero on a running instance, so the
// first read after a restart would otherwise report every held entry as new.
// The same shape as seeding the callers file, and the same failure: a burst of
// messages about things that happened weeks ago, which is exactly the noise
// that gets a channel muted.
func TestTheFirstLookEstablishesABaselineRatherThanAnnouncingIt(t *testing.T) {
	resetFired()
	wrongMu.Lock()
	lastWrong, wrongSeen = 0, false
	wrongMu.Unlock()

	somethingWrong()
	if !fired("wrong") {
		t.Error("the first sweep raised an alert about entries that were already there")
	}
}

// A threshold of zero turns a check off, and is not mistaken for "unset".
//
// Zero and empty are different answers and the same falsy value, which is the
// bug this repo has hit three times in other places — an empty string meaning
// both "no restriction" and "the default one".
func TestZeroTurnsACheckOffAndUnsetDoesNot(t *testing.T) {
	if got := number("", 85); got != 85 {
		t.Errorf("unset gave %d rather than the default", got)
	}
	if got := number("0", 85); got != 0 {
		t.Errorf("an explicit 0 gave %d — an operator cannot turn the check off", got)
	}
	if got := number("nonsense", 85); got != 85 {
		t.Errorf("a value that is not a number gave %d rather than the default", got)
	}
	if got := number("40", 85); got != 40 {
		t.Errorf("an operator's own number was ignored: %d", got)
	}
}

// Every alert says what the number was and what it is compared against.
//
// An operator reading "usage is high" at 3am has to reconstruct the threshold,
// the value and what to do about it — and the moment that is work, the next one
// is ignored. This is a shape check rather than a wording check: the message
// has to name the setting, so it can be changed by whoever is reading it.
func TestAnAlertSaysWhatWouldChangeIt(t *testing.T) {
	for _, a := range []alert{
		{Key: "k", What: "Busy", Why: "The threshold is 5,000 an hour (ALERT_CALLS_PER_HOUR)."},
		{Key: "k", What: "Disk", Why: "the threshold is 85% (ALERT_DISK_PERCENT)."},
	} {
		if !strings.Contains(a.Why, "ALERT_") {
			t.Errorf("%q does not name the setting that would change it: %q", a.What, a.Why)
		}
	}
}
