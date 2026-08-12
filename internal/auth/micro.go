package auth

// The instance's own agent, as an account.
//
// Micro already behaved like one — it writes the daily opinion, it posts to the
// blog under its own name, it surfaces breaking stories — but app.SystemUserID
// was a display constant and nothing more. There was no accounts["micro"], so
// nothing the agent did could be attributed, metered or rate limited. Its web
// searches are paid calls to Brave that appear in no usage record and are
// charged to nobody, because there was no identity to charge.
//
// So it gets an account. Not a special case bolted beside the account system —
// the same record every other caller has, which is what makes /usage,
// /admin/usage, quota, banning and the wallet work for it without any of them
// learning a new concept.
//
// Not an admin. It was, briefly, and that was the wrong instinct: what this
// account needs is not to be *charged*, which is a billing property, and admin
// was reached for because admins happen to be exempt. That trade granted
// /admin/env — every secret on the instance — /admin/console and the power to
// delete and ban, all to avoid a balance check. The exemption is now its own
// rule in internal/quota, expressed as itself.
//
// Agent, because a person and a program are not the same caller and a system
// that cannot tell them apart will eventually mail a password reset to a
// program. An admin may well be an agent one day; that is a different claim from
// not knowing which one you are talking to.

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// MicroID is the account id of the instance's own agent. It matches
// app.SystemUserID, which is what the blog has posted under all along.
const MicroID = "micro"

// EnsureMicro creates the instance's agent account if it is missing.
//
// After a human admin exists, never before. Account creation bootstraps the
// first account on an empty instance as admin, so creating this at boot on a
// fresh install would hand the instance to the agent and leave the person who
// runs it unable to reach /admin/env. On a new instance this simply does
// nothing until somebody signs up, which is the correct order: an instance with
// no people does not need an agent acting for them.
//
// Idempotent, and safe to call on every boot.
func EnsureMicro() {
	if _, err := GetAccount(MicroID); err == nil {
		return
	}
	if Operator() == "" {
		return // no human admin yet; the first signup is theirs, not ours
	}

	// A secret nobody holds. The agent is reached through the instance, never by
	// signing in, so this exists to keep the record the same shape as every
	// other account rather than to be used. Random rather than empty, because an
	// empty secret is a password of "" the day somebody adds a login path that
	// forgets this account is different.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return
	}

	acc := &Account{
		ID:       MicroID,
		Name:     "Micro",
		Secret:   hex.EncodeToString(secret),
		Created:  time.Now(),
		Agent:    true,
		Approved: true,
	}
	if err := Create(acc); err != nil {
		return
	}
}

// IsAgent reports whether an account is a program rather than a person.
func IsAgent(id string) bool {
	acc, err := GetAccount(id)
	return err == nil && acc.Agent
}
