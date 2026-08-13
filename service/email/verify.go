package email

// Claiming an address as your own, and what that is worth.
//
// The proof is the same one the account page has always taken — you can read
// what arrives at that mailbox — asked as a code rather than a link, because an
// agent has no browser to click one in. Where the proof is *kept* is
// internal/auth, next to the address the account signs in with, so "is this
// address yours" has one answer rather than two.
//
// What it buys is being recognised. service/mail routes on exactly this claim:
// mail from an address you have proved is never treated as a stranger's, it
// reaches your agent instead of a folder, and at the shared agent mailbox it is
// the only thing that says whose mail it was. That worked for one address, the
// one you signed up with. It now works for the address you actually write from.
//
// It does not change where a reply to *our* mail goes. Twilio will not carry a
// Reply-To, so a message sent through it is answered at its From, which on the
// sending domain has no inbox — see the package comment. What email_sender can
// now say is where the person should write instead, which is the honest half of
// that until the sending domain can receive.
//
// This is the one place this service sends to an address the caller has not
// shown any right to reach, and that is exactly the spam cannon the rest of it
// exists to prevent, wearing a respectable name. So a verification email is
// charged like any other, counts against the same daily cap, and is capped
// harder on top — three a day. Somebody claiming their own mailbox does it once.

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/userdb"
)

const (
	codes      = "codes"          // live challenges
	verifyLog  = "verifications"  // what was asked for, for the daily cap
	codeMaxAge = 10 * time.Minute // how long one is good for
	codeTries  = 5                // wrong guesses before it is torn up
	verifyCap  = 3                // starts per account per day
)

// Addresses is every address this account has proved is its own.
func Addresses(owner string) []string { return auth.VerifiedAddresses(owner) }

// Verified reports whether this address is this account's.
func Verified(owner, addr string) bool { return auth.OwnsAddress(owner, addr) }

// Forget drops an address this account had proved.
func Forget(owner, addr string) error { return auth.RemoveVerifiedAddress(owner, addr) }

// StartVerify emails a code to an address the caller says is theirs.
func StartVerify(owner, addr string) error {
	if owner == "" {
		return fmt.Errorf("sign in to verify an address")
	}
	if !Configured() {
		return fmt.Errorf("this instance has no sending domain configured")
	}
	address := auth.NormaliseAddress(addr)
	if address == "" {
		return fmt.Errorf("%q is not an email address", addr)
	}
	if Verified(owner, address) {
		return fmt.Errorf("%s is already yours", address)
	}
	if other := auth.AccountForAddress(address); other != nil {
		return fmt.Errorf("another account here has already proved that address is theirs")
	}
	if over, why := quota.OverLimit(owner, quota.OpExternalEmail); over {
		return fmt.Errorf("%s", why)
	}
	if startsToday(owner) >= verifyCap {
		return fmt.Errorf("that is %d verification emails today, which is the limit", verifyCap)
	}

	acc, err := auth.GetAccount(owner)
	if err != nil {
		return fmt.Errorf("account not found")
	}

	// Priced before it goes, refused before anything is spent. The endpoint
	// carries no Cost, so the gateway is not charging for this — the service
	// does, here, because only the first of the two steps sends anything and an
	// endpoint charge cannot tell them apart.
	cost := 0
	if quota.Metered(quota.OpExternalEmail) {
		ok, _, per, err := quota.CheckQuota(owner, quota.OpExternalEmail)
		if err != nil {
			return err
		}
		cost = per
		if !ok || quota.BalanceOf(owner) < cost {
			return fmt.Errorf("a verification email costs %d credits and there are not enough on this account", cost)
		}
	}

	code, err := digits(6)
	if err != nil {
		return err
	}

	// Counted before the message goes out. Counting after would let a caller
	// who keeps hitting a provider error retry without limit.
	noteStart(owner, address)

	body := fmt.Sprintf("Your code is %s. It is good for ten minutes.\n\n"+
		"Somebody at %s asked to add %s to their account. If that was not you, "+
		"ignore this — nothing has changed and the address has not been added.",
		code, Domain(), address)
	if _, err := deliver(owner, acc.Name, address, "Your code is "+code, body); err != nil {
		return err
	}
	if err := quota.ConsumeWith(owner, quota.OpExternalEmail, map[string]interface{}{
		"to": address, "verification": true,
	}); err != nil {
		// The message is gone; refusing now would only hide that.
		app.Log("email", "charging %s for a verification email: %v", owner, err)
	}

	// One live code per address, so asking again replaces rather than adds —
	// two valid codes doubles the guessing surface for no benefit.
	clearCodes(owner, address)
	_, err = userdb.Create(ns, owner, codes, map[string]interface{}{
		"address": address,
		"code":    code,
		"tries":   0,
		"at":      time.Now().Format(time.RFC3339),
	}, false)
	return err
}

// Confirm checks a code and, if it is right, records the address as the owner's.
func Confirm(owner, addr, code string) error {
	address := auth.NormaliseAddress(addr)
	if address == "" {
		return fmt.Errorf("%q is not an email address", addr)
	}
	code = strings.TrimSpace(code)

	recs, err := userdb.List(ns, owner, codes, "mine",
		map[string]interface{}{"address": address}, "", "", 1)
	if err != nil || len(recs) == 0 {
		return fmt.Errorf("ask for a code first")
	}
	rec := recs[0]

	at, _ := rec.Data["at"].(string)
	when, _ := time.Parse(time.RFC3339, at)
	if time.Since(when) > codeMaxAge {
		clearCodes(owner, address)
		return fmt.Errorf("that code has expired — ask for another")
	}

	tries := 0
	if f, ok := rec.Data["tries"].(float64); ok {
		tries = int(f)
	}
	if tries >= codeTries {
		// Six digits is a million guesses; five tries is what keeps it that.
		clearCodes(owner, address)
		return fmt.Errorf("too many wrong codes — ask for another")
	}

	want, _ := rec.Data["code"].(string)
	if code != want {
		rec.Data["tries"] = tries + 1
		userdb.Update(ns, owner, codes, rec.ID, rec.Data, false) //nolint:errcheck
		return fmt.Errorf("that code is not right")
	}

	clearCodes(owner, address)
	return auth.AddVerifiedAddress(owner, address)
}

// Pending is the address this owner has a live code for, if any.
//
// The page needs it to know which of the two steps to show — the same reason
// sms has one. Without it the form is a box for an address, a box for a code
// and a button that guesses which you meant.
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
		if a, _ := r.Data["address"].(string); a != "" {
			return a, true
		}
	}
	return "", false
}

func clearCodes(owner, address string) {
	recs, err := userdb.List(ns, owner, codes, "mine",
		map[string]interface{}{"address": address}, "", "", 10)
	if err != nil {
		return
	}
	for _, r := range recs {
		userdb.Delete(ns, owner, codes, r.ID) //nolint:errcheck
	}
}

// startsToday counts the verification emails this account has asked for.
//
// Counted from its own log rather than from the codes, because codes are
// deleted when they are used and a limit you can clear by succeeding is not a
// limit.
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

func noteStart(owner, address string) {
	userdb.Create(ns, owner, verifyLog, map[string]interface{}{ //nolint:errcheck
		"address": address, "at": time.Now().Format(time.RFC3339),
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
