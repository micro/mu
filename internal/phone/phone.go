// Package phone is who a phone number belongs to.
//
// It exists because two services needed the same answer and one of them took it
// from the other: whatsapp imported sms for number formatting and ownership,
// which made the pair one unit. A phone number is a phone number whether it is
// carrying a text or a WhatsApp message, and proving one is yours should not
// have to be done twice because the proof happened to be stored by whichever
// service was written first.
//
// What is here is ownership: normalising a number to one spelling, recording
// that an account proved a number is theirs, and answering who that was. What is
// not here is anything about sending — who you may text, what it costs, opt-out,
// the rules — because that is a service's business and differs per channel.
//
// The storage namespace is deliberately still "sms". These records are live:
// somebody verified their number before this package existed, and renaming the
// namespace would lose them for no benefit anybody can see. The name is where
// the data has always been, not a claim about what owns it.
package phone

import (
	"fmt"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/settings"
	"mu/internal/userdb"
)

const (
	// ns is where these records live. See the package comment: it stays "sms"
	// because that is where they already are.
	ns = "sms"

	numbers  = "numbers"  // numbers this account has verified as its own
	claims   = "claims"   // number → the account that proved the number is theirs
	instance = "instance" // what the instance owns rather than any account
)

// Normalise reduces a number to E.164, or to empty if it cannot be read as one.
//
// Two spellings of one number are two numbers to every lookup there is, so this
// runs before anything is stored or compared. A number with no country code is
// ambiguous, and guessing one is how you text a stranger in another country —
// only the instance's own default rescues it.
func Normalise(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
			b.WriteRune(r)
		}
	}
	n := b.String()
	if n == "" || n == "+" {
		return ""
	}
	if !strings.HasPrefix(n, "+") {
		if cc := strings.TrimSpace(settings.Get("SMS_DEFAULT_COUNTRY")); cc != "" {
			n = "+" + strings.TrimPrefix(cc, "+") + strings.TrimPrefix(n, "0")
		} else {
			return ""
		}
	}
	if len(n) < 8 || len(n) > 16 {
		return ""
	}
	return n
}

// Verified reports whether this number belongs to this owner.
func Verified(owner, number string) bool {
	recs, err := userdb.List(ns, owner, numbers, "mine",
		map[string]interface{}{"number": Normalise(number)}, "", "", 1)
	return err == nil && len(recs) > 0
}

// Verify records that a number is this owner's own.
func Verify(owner, number string) error {
	number = Normalise(number)
	if number == "" {
		return fmt.Errorf("that does not look like a phone number in international format, e.g. +447700900123")
	}
	if Verified(owner, number) {
		return nil
	}
	if _, err := userdb.Create(ns, owner, numbers,
		map[string]interface{}{"number": number, "at": time.Now().Format(time.RFC3339)}, false); err != nil {
		return err
	}
	claim(owner, number)
	return nil
}

// Forget drops a number this owner had verified.
//
// Verifying is reversible, because a number is not yours forever: phones change
// hands, and a person who gave one up should be able to say so without an
// argument.
func Forget(owner, number string) {
	number = Normalise(number)
	recs, err := userdb.List(ns, owner, numbers, "mine",
		map[string]interface{}{"number": number}, "", "", 10)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, owner, numbers, r.ID) //nolint:errcheck
	}
	unclaim(owner, number)
}

// Numbers lists what this owner has verified as theirs.
func Numbers(owner string) []string {
	recs, err := userdb.List(ns, owner, numbers, "mine", nil, "", "", 50)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range recs {
		if n, _ := r.Data["number"].(string); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Owner is the account that proved this number is theirs, if any.
//
// Falls back to looking, and writes what it finds. Numbers verified before
// there was a claim to write have none, and the symptom of that is a message
// arriving from a number its owner had proved was theirs and going nowhere — so
// rather than a migration that has to be remembered, the miss repairs itself the
// first time it matters.
func Owner(number string) string {
	number = Normalise(number)
	if number == "" {
		return ""
	}
	recs, err := userdb.List(ns, instance, claims, "mine",
		map[string]interface{}{"number": number}, "", "", 1)
	if err == nil && len(recs) > 0 {
		if owner, _ := recs[0].Data["owner"].(string); owner != "" {
			return owner
		}
	}
	for _, acc := range auth.AllAccounts() {
		if acc != nil && Verified(acc.ID, number) {
			claim(acc.ID, number)
			return acc.ID
		}
	}
	return ""
}

// claim records, instance-wide, that this number belongs to this account.
//
// This is what verifying is for. Without it "verified" meant a label in an
// autocomplete: an inbound message was given to whoever last texted that number
// from here, so texting the instance from your own phone reached nobody unless
// you had texted yourself first. Proving a number is yours is the one claim
// strong enough to route on, and it beats the last-conversation guess, because
// somebody who texted your phone from here has not shown it is theirs.
//
// Instance-owned, like the opt-out list, because one account cannot read
// another's records and the router has to be able to ask.
func claim(owner, number string) {
	recs, err := userdb.List(ns, instance, claims, "mine",
		map[string]interface{}{"number": number}, "", "", 1)
	data := map[string]interface{}{
		"number": number, "owner": owner, "at": time.Now().Format(time.RFC3339),
	}
	if err == nil && len(recs) == 1 {
		// A phone changes hands, so the most recent proof wins.
		userdb.Update(ns, instance, claims, recs[0].ID, data, false) //nolint:errcheck
		return
	}
	userdb.Create(ns, instance, claims, data, false) //nolint:errcheck
}

// unclaim drops the claim, but only if it is this owner's to drop.
func unclaim(owner, number string) {
	recs, err := userdb.List(ns, instance, claims, "mine",
		map[string]interface{}{"number": number, "owner": owner}, "", "", 5)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, instance, claims, r.ID) //nolint:errcheck
	}
}
