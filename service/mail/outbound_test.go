package mail

// Who may send mail out of this instance as themselves.
//
// Mail leaving here goes out as you@<mail domain>, which on a public instance
// is the domain carrying password resets and sign-in links. Anyone can sign up.
// So the rules below are not decoration: they are what stops a free account
// spending the deliverability of the mail that has to arrive, and no balance
// makes that whole.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// mailAccount makes one for the test and takes it away afterwards.
func mailAccount(t *testing.T, id string) *auth.Account {
	t.Helper()
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id, Secret: "s"}); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}
	acc, err := auth.GetAccount(id)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	return acc
}

// A fresh account with nothing behind it cannot cold-mail a stranger.
func TestAnUnaccountableAccountCannotColdMailOut(t *testing.T) {
	acc := mailAccount(t, "mail-gate-fresh")
	acc.Admin, acc.Approved, acc.EmailVerified = false, false, false
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}

	ok, why := MaySendOut(acc.ID, "stranger@example.com")
	if ok {
		t.Fatal("a fresh account can send mail out under this instance's domain")
	}
	// The refusal has to name both ways out. One that does not is
	// indistinguishable from the feature being broken.
	//
	// Named as places rather than as routes: a path written into a sentence
	// reads as a link and is not one, wherever the sentence ends up.
	for _, want := range []string{"your Account", "email_send"} {
		if !strings.Contains(why, want) {
			t.Errorf("the refusal does not mention %s: %s", want, why)
		}
	}
	if strings.Contains(why, "/account") || strings.Contains(why, "/wallet") {
		t.Errorf("the refusal names a route, which reads as a link and is not one: %s", why)
	}
}

// Verifying an address opens it, because that is what accountability means
// here — and it is the same word posting already uses.
func TestProvingWhoYouAreOpensIt(t *testing.T) {
	acc := mailAccount(t, "mail-gate-verified")

	acc.EmailVerified = false
	auth.UpdateAccount(acc) //nolint:errcheck
	if ok, _ := MaySendOut(acc.ID, "stranger@example.com"); ok {
		t.Fatal("unverified and yet allowed")
	}

	acc.Email, acc.EmailVerified = "someone@example.org", true
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}
	if ok, why := MaySendOut(acc.ID, "stranger@example.com"); !ok {
		t.Errorf("a verified account is still refused: %s", why)
	}
}

// An admin is trusted, which is what keeps a self-hosted instance out of this
// entirely: the first account on one is the admin.
func TestASelfHostersOwnAccountIsNotCaughtByThis(t *testing.T) {
	acc := mailAccount(t, "mail-gate-admin")
	acc.Admin = true
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}
	if ok, why := MaySendOut(acc.ID, "anyone@example.com"); !ok {
		t.Errorf("the operator of their own instance cannot send mail: %s", why)
	}
}

// Answering somebody who wrote to you is never gated.
//
// An agent with an address that cannot reply to its own mail is the feature not
// working. It is also the right line on risk rather than a hole in it:
// complaints come from mail nobody asked for, and an answer to somebody who
// made contact is the best signal a sending domain has.
func TestAnsweringSomebodyWhoWroteToYouIsNeverGated(t *testing.T) {
	acc := mailAccount(t, "mail-gate-reply")
	acc.Admin, acc.Approved, acc.EmailVerified = false, false, false
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}

	const them = "wroteto@example.com"
	if ok, _ := MaySendOut(acc.ID, them); ok {
		t.Fatal("allowed before they had written to anybody")
	}

	arrived(t, acc.ID, them, false)
	if ok, why := MaySendOut(acc.ID, them); !ok {
		t.Errorf("cannot answer somebody who wrote first: %s", why)
	}
	// And it opens nothing else. A reply is a reply to one person.
	if ok, _ := MaySendOut(acc.ID, "someone-else@example.com"); ok {
		t.Error("one inbound message opened cold mail to everybody")
	}
}

// Spam does not count as having been written to, or anyone could open the gate
// by sending one message nobody wanted.
func TestAMessageFiledAsSpamIsNotARelationship(t *testing.T) {
	acc := mailAccount(t, "mail-gate-spam")
	acc.Admin, acc.Approved, acc.EmailVerified = false, false, false
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}

	const them = "spammer@example.com"
	arrived(t, acc.ID, them, true)
	if ok, _ := MaySendOut(acc.ID, them); ok {
		t.Error("a spam message was enough to open cold mail back to its sender")
	}
}

// Mail cannot leave an instance with no mail domain, and the refusal says where
// to go instead rather than failing somewhere in the transport.
func TestWithNoMailDomainNothingLeavesAndItSaysWhy(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "")
	acc := mailAccount(t, "mail-gate-nodomain")
	acc.Admin = true // trusted, so the gate is not what is being tested
	if err := auth.UpdateAccount(acc); err != nil {
		t.Fatal(err)
	}

	_, err := SendOut(acc.ID, acc.Name, "someone@example.com", "Hi", "there", "", "")
	if err == nil {
		t.Fatal("mail left an instance with no domain to send it from")
	}
	if !strings.Contains(err.Error(), "email_send") {
		t.Errorf("the refusal does not say where to go instead: %v", err)
	}
}

// A local recipient is not mail leaving the instance, and this path must not
// take one — the caller's routing decision would be silently wrong.
func TestSendOutRefusesALocalRecipient(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")
	acc := mailAccount(t, "mail-gate-local")

	if _, err := SendOut(acc.ID, acc.Name, "asim", "Hi", "there", "", ""); err == nil {
		t.Error("a bare username was accepted as mail leaving the instance")
	}
}

// arrived files an inbound message from an outside address, the way delivery
// does: the sender's address is the from id.
func arrived(t *testing.T, owner, from string, spam bool) {
	t.Helper()
	mutex.Lock()
	messages = append(messages, &Message{
		ID: from + "-" + owner, From: from, FromID: from,
		To: owner, ToID: owner, Subject: "Hello", Body: "hi",
		Spam: spam, CreatedAt: time.Now(),
	})
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		kept := messages[:0]
		for _, m := range messages {
			if m.ID != from+"-"+owner {
				kept = append(kept, m)
			}
		}
		messages = kept
		mutex.Unlock()
	})
}
