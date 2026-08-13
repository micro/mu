package email

// Checking that somebody can read an address.
//
// **Who calls this.** Not a person tidying their own account — an agent that is
// building something and needs the check every signup needs: is this address
// real, and does the person filling in the form actually read it. Mu sends the
// code, keeps it, and answers whether the one that came back matches. What that
// means is the caller's business; it belongs in their records, not ours.
//
// Getting that backwards is what the first version of this did. It read as "the
// caller is claiming an address for themselves", and every rule that follows
// from that reading is an obstruction to the real one: three codes a day, a
// refusal to re-check an address already proved, and — worst — one address to
// one account instance-wide, so the second product with a user at the same
// Gmail address simply could not verify them. A tool whose limits assume it is
// being used once is useless to somebody using it for a living.
//
// So the limits here are only the ones that survive the question *what abuse
// does this actually prevent*:
//
//   - **Codes to one address are throttled.** This is the only thing here that
//     puts mail in front of somebody who did not ask for it, and without a
//     ceiling per recipient it is a way to bomb a mailbox with a respectable
//     name on it. Per hour, per address, per caller.
//   - **Five wrong guesses ends a code.** Six digits is a million; without this
//     it is a few thousand requests.
//   - **Ten minutes.** A code that never expires is a password sent in clear.
//   - **It is an email**, so it costs a send and counts against the same daily
//     cap sending does. Nothing extra on top of that.
//
// Everything else — how many people you verify, how often, whether you have
// checked this address before — is the caller's own volume, and metering it
// twice is the same mistake in a different coat.
//
// **The message carries the caller's name, not ours.** An agent verifying its
// users sends mail on behalf of a product those people have heard of, and a
// code that arrives talking about micro.mu reads like a phish and gets deleted.
//
// **Mine is the other half**, and it is the small one. Verifying an address you
// can read yourself is worth something here specifically: service/mail routes on
// "addresses this account has proved", so mail from one reaches your agent
// rather than a spam folder. That was one address, proved by clicking a link,
// which an agent has no browser to do. Passing mine records it. It is off by
// default, because the ordinary caller is verifying somebody else's address and
// has no business adding a stranger's mailbox to their own account.

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
	codes     = "codes"         // live challenges
	codeLog   = "verifications" // what went where, for the per-address throttle
	codeLife  = 10 * time.Minute
	codeTries = 5

	// perAddress is how many codes one caller may send to one address in an
	// hour. High enough that a person who mistypes, waits, and asks again is
	// never stopped; low enough that this is not a way to fill somebody's inbox.
	perAddress = 5
	addressFor = time.Hour
)

// Verification is the state of one address's check, which is the whole of what
// a caller needs to drive a signup form.
type Verification struct {
	Address string
	// Status is pending, approved, incorrect, expired, exhausted, or none.
	//
	// A wrong code is a status rather than an error on purpose. It is the
	// ordinary outcome of asking — somebody mistyped — and an agent showing
	// "that code is not right" to its user should not have to read that out of
	// an exception.
	Status string
	// Left is how many more guesses this code has, while it is pending.
	Left int
	// Expires is when it stops being good for anything.
	Expires time.Time
}

// OK reports whether the code was right.
func (v Verification) OK() bool { return v.Status == "approved" }

// Says is the status as a sentence, for a page or an agent to pass on.
func (v Verification) Says() string {
	switch v.Status {
	case "pending":
		return fmt.Sprintf("Emailed a code to %s. It is good for ten minutes.", v.Address)
	case "approved":
		return fmt.Sprintf("%s is verified — whoever gave that code can read it.", v.Address)
	case "incorrect":
		return fmt.Sprintf("That code is not right. %d more %s.", v.Left,
			map[bool]string{true: "try", false: "tries"}[v.Left == 1])
	case "expired":
		return "That code has expired — ask for another."
	case "exhausted":
		return "Too many wrong codes — ask for another."
	}
	return fmt.Sprintf("No code is waiting for %s.", v.Address)
}

// StartVerify emails a code to an address and returns the check's state.
//
// app is the name to put in the message — the caller's product, not this
// instance. Empty falls back to the account's own name, which is right for
// somebody verifying an address of their own.
func StartVerify(owner, addr, app string) (Verification, error) {
	if owner == "" {
		return Verification{}, fmt.Errorf("sign in to verify an address")
	}
	if !Configured() {
		return Verification{}, fmt.Errorf("this instance has no sending domain configured")
	}
	address := auth.NormaliseAddress(addr)
	if address == "" {
		return Verification{}, fmt.Errorf("%q is not an email address", addr)
	}

	// The daily cap on sending, because this is a send. Not a second allowance:
	// a caller verifying a hundred signups is doing a hundred sends' worth of
	// work and should meet the cap they already know about.
	if over, why := quota.OverLimit(owner, quota.OpExternalEmail); over {
		return Verification{}, fmt.Errorf("%s", why)
	}
	// The one limit that is about the recipient rather than the caller.
	if n := sentTo(owner, address); n >= perAddress {
		return Verification{}, fmt.Errorf("that is %d codes to %s within the hour — "+
			"wait before sending another", n, address)
	}

	acc, err := auth.GetAccount(owner)
	if err != nil {
		return Verification{}, fmt.Errorf("account not found")
	}
	product := strings.TrimSpace(app)
	if product == "" {
		product = acc.Name
	}

	// Priced before it goes, refused before anything is spent. The endpoint
	// carries no Cost, so the gateway is not charging for this — the service
	// does, here, because only the first of the two steps sends anything and an
	// endpoint charge cannot tell them apart.
	if quota.Metered(quota.OpExternalEmail) {
		ok, _, per, err := quota.CheckQuota(owner, quota.OpExternalEmail)
		if err != nil {
			return Verification{}, err
		}
		if !ok || quota.BalanceOf(owner) < per {
			return Verification{}, fmt.Errorf("a verification email costs %d credits "+
				"and there are not enough on this account", per)
		}
	}

	code, err := digits(6)
	if err != nil {
		return Verification{}, err
	}

	// Counted before the message goes out. Counting after would let a caller who
	// keeps hitting a provider error retry past the ceiling.
	noteSend(owner, address)

	if _, err := deliver(owner, product, address,
		code+" is your "+product+" verification code",
		verifyBody(code, product)); err != nil {
		return Verification{}, err
	}
	if err := quota.ConsumeWith(owner, quota.OpExternalEmail, map[string]interface{}{
		"to": address, "verification": true,
	}); err != nil {
		// The message is gone; refusing now would only hide that.
		logf("charging %s for a verification email: %v", owner, err)
	}

	// One live code per address, so asking again replaces rather than adds — two
	// valid codes doubles the guessing surface for no benefit.
	clearCodes(owner, address)
	now := time.Now()
	if _, err := userdb.Create(ns, owner, codes, map[string]interface{}{
		"address": address,
		"code":    code,
		"tries":   0,
		"at":      now.Format(time.RFC3339),
	}, false); err != nil {
		return Verification{}, err
	}
	return Verification{Address: address, Status: "pending",
		Left: codeTries, Expires: now.Add(codeLife)}, nil
}

// Check tests a code against the one that was sent.
//
// The error is for a caller's mistake — a malformed address. Everything a
// person filling in a form can do wrong comes back as a status, because it is
// an answer rather than a failure.
func Check(owner, addr, code string) (Verification, error) {
	address := auth.NormaliseAddress(addr)
	if address == "" {
		return Verification{}, fmt.Errorf("%q is not an email address", addr)
	}
	v := Verification{Address: address, Status: "none"}
	code = strings.TrimSpace(code)

	recs, err := userdb.List(ns, owner, codes, "mine",
		map[string]interface{}{"address": address}, "", "", 1)
	if err != nil || len(recs) == 0 {
		return v, nil
	}
	rec := recs[0]

	at, _ := rec.Data["at"].(string)
	when, _ := time.Parse(time.RFC3339, at)
	v.Expires = when.Add(codeLife)
	if time.Since(when) > codeLife {
		clearCodes(owner, address)
		v.Status = "expired"
		return v, nil
	}

	tries := 0
	if f, ok := rec.Data["tries"].(float64); ok {
		tries = int(f)
	}
	if tries >= codeTries {
		clearCodes(owner, address)
		v.Status = "exhausted"
		return v, nil
	}

	if want, _ := rec.Data["code"].(string); code != want {
		rec.Data["tries"] = tries + 1
		userdb.Update(ns, owner, codes, rec.ID, rec.Data, false) //nolint:errcheck
		v.Status, v.Left = "incorrect", codeTries-tries-1
		return v, nil
	}

	clearCodes(owner, address)
	v.Status = "approved"
	return v, nil
}

// Claim records an address on the caller's own account, having checked it.
//
// Separate from Check, and not something Check does on its own, because the
// ordinary caller is verifying somebody else's address: adding a stranger's
// mailbox to your account because you asked whether they could read it would be
// an answer to a question nobody asked.
//
// One address to one account here, unlike verification itself, and the
// difference is what this list is for: service/mail asks it whose mail arrived,
// and two accounts holding one address makes that a coin toss. Verification has
// no such constraint — two products may both have a user at the same address,
// and they do.
func Claim(owner, addr string) error { return auth.AddVerifiedAddress(owner, addr) }

// Addresses is every address this account has proved is its own.
func Addresses(owner string) []string { return auth.VerifiedAddresses(owner) }

// Verified reports whether this address is this account's own.
func Verified(owner, addr string) bool { return auth.OwnsAddress(owner, addr) }

// Forget drops an address this account had claimed.
func Forget(owner, addr string) error { return auth.RemoveVerifiedAddress(owner, addr) }

// verifyBody is the message. Plain, short, and about the caller's product.
//
// The last line is not politeness. Most people who get one of these did not ask
// for it, and telling them that ignoring it is enough is what stops a signup
// form being a way to worry strangers.
func verifyBody(code, product string) string {
	return fmt.Sprintf("Your %s verification code is %s.\n\n"+
		"It is good for ten minutes. If you did not ask for it, ignore this — "+
		"nothing has been changed and nobody has been given access to anything.",
		product, code)
}

// Pending is the address this owner has a live code for, if any.
//
// The page needs it to know which of the two steps to show. Without it the form
// is a box for an address, a box for a code, and a button that guesses which
// you meant.
func Pending(owner string) (string, bool) {
	recs, err := userdb.List(ns, owner, codes, "mine", nil, "", "", 5)
	if err != nil {
		return "", false
	}
	for _, r := range recs {
		at, _ := r.Data["at"].(string)
		when, err := time.Parse(time.RFC3339, at)
		if err != nil || time.Since(when) > codeLife {
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

// sentTo counts the codes this caller has sent to one address in the last hour.
//
// Counted from its own log rather than from the live codes, because a code is
// deleted when it is used and a ceiling you can clear by succeeding is not a
// ceiling.
func sentTo(owner, address string) int {
	since := time.Now().Add(-addressFor)
	recs, err := userdb.List(ns, owner, codeLog, "mine",
		map[string]interface{}{"address": address}, "", "", 50)
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

func noteSend(owner, address string) {
	userdb.Create(ns, owner, codeLog, map[string]interface{}{ //nolint:errcheck
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

func logf(format string, args ...interface{}) { app.Log("email", format, args...) }
