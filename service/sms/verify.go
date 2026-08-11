package sms

// Claiming a number as your own, and what that is worth.
//
// It is what decides who an inbound message belongs to. This instance has one
// number for everybody, so a text arriving on it has to be given to an account,
// and "I proved this number is mine" is the only claim strong enough to decide
// that. Everything else is a guess — the fallback is whoever last texted that
// number from here, which is right often enough to be worth having and wrong
// whenever two accounts know the same person.
//
// This is the one place a message goes to a number the sender does not already
// know, because it is what makes knowing one possible. That makes it the
// weakest point in everything above: an unbounded "send a code to any number"
// is exactly the spam cannon the rest of the service exists to prevent, wearing
// a respectable name.
//
// So a verification text is charged like any other text, held to the same
// country allowlist and the same opt-out list, and capped harder than sending
// is — three a day. Somebody claiming their own phone does it once.

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"mu/internal/quota"
	"mu/internal/userdb"
)

const (
	codes      = "codes"
	codeMaxAge = 10 * time.Minute
	codeTries  = 5
	verifyCap  = 3 // starts per account per day
)

// StartVerify texts a code to a number the caller says is theirs.
func StartVerify(owner, number string) error {
	if !Configured() {
		return fmt.Errorf("this instance has no number to send from")
	}
	if Verified(owner, number) {
		return fmt.Errorf("%s is already yours", number)
	}
	if OptedOut(number) {
		return fmt.Errorf("%s has asked not to receive messages from this instance", number)
	}
	if !countryAllowed(number) {
		return fmt.Errorf("this instance does not send to %s", number)
	}
	if DailyLimit() == 0 {
		return fmt.Errorf("this instance is not sending texts at the moment")
	}
	if startsToday(owner) >= verifyCap {
		return fmt.Errorf("that is %d verification texts today, which is the limit", verifyCap)
	}

	code, err := digits(6)
	if err != nil {
		return err
	}

	cost := 0
	if quota.Metered(quota.OpSMSSend) {
		ok, _, per, err := quota.CheckQuota(owner, quota.OpSMSSend)
		if err != nil {
			return err
		}
		cost = per
		if !ok || quota.BalanceOf(owner) < cost {
			return fmt.Errorf("a verification text costs %d credits and there are not enough on this account", cost)
		}
	}

	// Counted before the message goes out. Counting after would let a caller
	// who keeps hitting a provider error retry without limit.
	noteStart(owner, number)

	if _, err := send(number, "Your code is "+code+". It is good for ten minutes."); err != nil {
		return err
	}
	if err := quota.ConsumeWith(owner, quota.OpSMSSend, map[string]interface{}{
		"to": number, "verification": true,
	}); err != nil {
		logCharge(owner, err)
	}

	// One live code per number, so asking again replaces rather than adds — two
	// valid codes doubles the guessing surface for no benefit.
	clearCodes(owner, number)
	_, err = userdb.Create(ns, owner, codes, map[string]interface{}{
		"number": number,
		"code":   code,
		"tries":  0,
		"at":     time.Now().Format(time.RFC3339),
	}, false)
	return err
}

// Confirm checks a code and, if it is right, records the number as the owner's.
func Confirm(owner, number, code string) error {
	code = strings.TrimSpace(code)
	recs, err := userdb.List(ns, owner, codes, "mine",
		map[string]interface{}{"number": number}, "", "", 1)
	if err != nil || len(recs) == 0 {
		return fmt.Errorf("ask for a code first")
	}
	rec := recs[0]

	at, _ := rec.Data["at"].(string)
	when, _ := time.Parse(time.RFC3339, at)
	if time.Since(when) > codeMaxAge {
		clearCodes(owner, number)
		return fmt.Errorf("that code has expired — ask for another")
	}

	tries := 0
	if f, ok := rec.Data["tries"].(float64); ok {
		tries = int(f)
	}
	if tries >= codeTries {
		// Six digits is a million guesses; five tries is what keeps it that.
		clearCodes(owner, number)
		return fmt.Errorf("too many wrong codes — ask for another")
	}

	want, _ := rec.Data["code"].(string)
	if code != want {
		rec.Data["tries"] = tries + 1
		userdb.Update(ns, owner, codes, rec.ID, rec.Data, false) //nolint:errcheck
		return fmt.Errorf("that code is not right")
	}

	clearCodes(owner, number)
	return Verify(owner, number)
}

// Pending returns the number this owner has a live code for, if any.
//
// The page needs it to know which of the two steps to show. Without it the
// form had to be one box for a number, one for a code and a single button that
// guessed which you meant from whether the second was empty — so a browser
// autofilling a field called "code" sent you to "ask for a code first", which
// is a sentence about a step you were trying to take.
func Pending(owner string) (string, bool) {
	recs, err := userdb.List(ns, owner, codes, "mine", nil, "", "", 5)
	if err != nil {
		return "", false
	}
	for _, r := range recs {
		at, _ := r.Data["at"].(string)
		when, err := time.Parse(time.RFC3339, at)
		if err != nil || time.Since(when) > codeMaxAge {
			continue
		}
		if n, _ := r.Data["number"].(string); n != "" {
			return n, true
		}
	}
	return "", false
}

func clearCodes(owner, number string) {
	recs, err := userdb.List(ns, owner, codes, "mine",
		map[string]interface{}{"number": number}, "", "", 10)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, owner, codes, r.ID) //nolint:errcheck
	}
}

// startsToday counts verification texts this account has asked for.
//
// Counted from the messages it charged for rather than the codes, because codes
// are deleted when they are used and a limit you can clear by succeeding is not
// a limit.
func startsToday(owner string) int {
	since := time.Now().Add(-24 * time.Hour)
	recs, err := userdb.List(ns, owner, verifyLog, "mine", nil, "", "", 100)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range recs {
		at, _ := r.Data["at"].(string)
		if t, err := time.Parse(time.RFC3339, at); err == nil && t.After(since) {
			n++
		}
	}
	return n
}

const verifyLog = "verifications"

func noteStart(owner, number string) {
	userdb.Create(ns, owner, verifyLog, map[string]interface{}{ //nolint:errcheck
		"number": number, "at": time.Now().Format(time.RFC3339),
	}, false)
}

// digits returns a random numeric code. crypto/rand, because a predictable code
// is the same as no code.
func digits(n int) (string, error) {
	var b strings.Builder
	for i := 0; i < n; i++ {
		d, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("could not make a code")
		}
		b.WriteString(d.String())
	}
	return b.String(), nil
}
