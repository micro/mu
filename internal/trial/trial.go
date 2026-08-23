// Package trial is the free exchanges somebody gets before they have an
// account.
//
// Somebody writes to agent@ from an address nobody here knows. They get an
// account they did not ask for and cannot sign in to (auth.Unclaimed), and this
// is what governs what it may do: a fixed number of exchanges, then a mail with
// a link that turns it into a real account with the conversation still in it.
//
// # Why it is not credits
//
// A credit is charged when an operation costs this instance something to run,
// and it is spent from a balance an account tops up. An unclaimed account has
// never been near a card, so every one of these runs would fail the quota check
// and answer "top up at /billing/topup and send it again" — to somebody who has
// not signed up, about an account they do not know exists. Wrong answer, and it
// would make the first experience of the product a bill.
//
// So the two paths are separate: an account that has been claimed spends
// credits, an unclaimed one spends turns. Nothing on this path touches the
// ledger.
//
// # Why it is not in the mail client
//
// It was, and it is a policy about accounts rather than about mail. Every word
// of it is "set up an account at /signup"; none of it is about SMTP, threading
// or who may wake an agent, which is what the rest of that package decides.
// Mail is only where it is reached from because mail is the only channel that
// reaches a stranger — and the moment a second one does, a policy sitting in
// one channel's package gets copied rather than shared.
//
// The state it reads is already underneath it, in internal/auth: TurnsLeft,
// SpendTurn, Invited. What is here is the decision made out of them, the
// instance's own daily ceiling, and what somebody is told.
package trial

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// dailyTotal is the instance-wide ceiling on free exchanges per day.
//
// Zero turns them off, which is the right setting for an instance somebody runs
// for themselves: there is nobody to demonstrate to.
func dailyTotal() int {
	v := strings.TrimSpace(settings.Get("TRIAL_DAILY_TOTAL"))
	if v == "" {
		return 500
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 500
	}
	return n
}

var (
	dayMu    sync.Mutex
	dayStamp string
	dayCount int
)

// dayTaken counts one against today's ceiling and reports whether there was room.
func dayTaken() bool {
	dayMu.Lock()
	defer dayMu.Unlock()
	today := time.Now().UTC().Format("2006-01-02")
	if dayStamp != today {
		dayStamp, dayCount = today, 0
	}
	if dayCount >= dailyTotal() {
		return false
	}
	dayCount++
	return true
}

// Allowed reports whether this is an unclaimed account's free exchange, and
// what to say instead when it is not.
//
// The empty string with ok=false means this is an ordinary account and the
// credit path applies — the caller carries on to quota as before.
func Allowed(ownerID string) (trial bool, ok bool, why string) {
	acc, err := auth.GetAccount(ownerID)
	if err != nil || acc == nil || !acc.Unclaimed {
		return false, false, ""
	}
	if auth.TurnsLeft(ownerID) <= 0 {
		return true, false, outOfTurns(acc)
	}
	if !dayTaken() {
		// The ceiling, not this person's allowance. Said as a delay rather than
		// a refusal, because it is: tomorrow it works again.
		return true, false, "I have answered as many questions as this instance gives away today. " +
			"Try again tomorrow, or set up an account at " + app.PublicURL() + "/signup and it will not " +
			"come up again."
	}
	return true, true, ""
}

// Spend records the exchange and invites them in if it was the last.
//
// Called after the answer has gone out. The invitation is a second mail rather
// than a footer on the first, because it is a different message with a link in
// it — appending it to every answer near the limit would put a sales line under
// somebody's weather forecast.
func Spend(ownerID string) {
	acc, err := auth.GetAccount(ownerID)
	if err != nil || acc == nil || !acc.Unclaimed {
		return
	}
	if last := auth.SpendTurn(ownerID); !last {
		return
	}
	invite(ownerID)
}

// invite mails the link that turns an unclaimed account into a real one.
//
// Once. auth.Invited is what stops every message after the limit sending
// another, which is the difference between an invitation and being harassed by
// a mail server.
func invite(ownerID string) {
	if auth.Invited(ownerID) {
		return
	}
	acc, err := auth.GetAccount(ownerID)
	if err != nil || acc == nil || acc.Email == "" {
		return
	}
	// The same invite the rest of the instance issues, so it is redeemed by the
	// same code and an invite-only instance behaves the same way. Created by the
	// account it belongs to rather than by an admin, because nobody approved
	// this one — the sender earned it by using the thing.
	code, err := auth.CreateInvite(acc.Email, acc.ID)
	if err != nil {
		app.Log("trial", "could not create an invite for %s: %v", acc.ID, err)
		return
	}
	auth.MarkInvited(ownerID)

	link := app.PublicURL() + "/signup?invite=" + code
	if app.EmailSender == nil {
		return
	}
	app.EmailSender(acc.Email, "Keep your agent",
		invitePlain(link), inviteHTML(link))
}

// outOfTurns is what somebody is told when they write again after the limit.
//
// It names no number. What a demonstration is worth is an operator's setting,
// and a message quoting "your ten free questions" fixes in writing something
// that is theirs to change — the same reasoning that took the count off the
// landing page.
func outOfTurns(acc *auth.Account) string {
	return fmt.Sprintf("That is the free exchanges used up — everything we have said is saved.\n\n"+
		"Set up an account and it is all still here: %s/signup\n\n"+
		"It takes a username and a password. This address is already verified, so there is "+
		"nothing to click in a second email.", app.PublicURL())
}

func invitePlain(link string) string {
	return "That is the free exchanges used up.\n\n" +
		"Everything we have said is saved. Set up an account and it is all still there — " +
		"the conversation, and the agent that had it.\n\n" +
		link + "\n\n" +
		"It takes a username and a password. Your address is already verified, so there is " +
		"nothing else to click."
}

func inviteHTML(link string) string {
	return `<p>That is the free exchanges used up.</p>` +
		`<p>Everything we have said is saved. Set up an account and it is all still there — ` +
		`the conversation, and the agent that had it.</p>` +
		`<p><a href="` + link + `">Keep your agent</a></p>` +
		`<p>It takes a username and a password. Your address is already verified, so there is ` +
		`nothing else to click.</p>`
}
