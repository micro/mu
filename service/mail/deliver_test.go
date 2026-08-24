package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/event"
)

// The addresses on this instance actually reach somebody.
//
// Source checks below say the branch is in one place; this says the branch is
// right. Every case here came back as an error before: "that is not mail
// leaving it" from the compose form, or a filed message that woke nothing.
func TestDeliverRoutesEveryLocalShapeOfAddress(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	sender := account(t, "router")
	recipient := account(t, "routee")

	for _, tc := range []struct {
		name  string
		to    string
		owner string // whose inbox it lands in
		tag   string // which agent it names, if any
	}{
		{"a bare username", "routee", recipient, ""},
		{"a full local address", "routee@example.test", recipient, ""},
		{"somebody else's agent", "routee+research@example.test", recipient, "research"},
		{"your own agent", "router+research@example.test", sender, "research"},
		// agent@ is not an account: it resolves to whoever wrote to it, so the
		// conversation lands in the sender's own inbox. This is the address the
		// report came in about — the compose form refused it outright.
		{"the shared agent address", "agent@example.test", sender, ""},
		{"the shared address naming an agent", "agent+research@example.test", sender, "research"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := Deliver(Outgoing{
				FromID: sender, Display: "Router", To: tc.to,
				Subject: "hello", Body: "is anyone there",
			})
			if err != nil {
				t.Fatalf("Deliver to %s: %v", tc.to, err)
			}
			m := storedByMessageID(id)
			if m == nil {
				t.Fatalf("nothing was filed for %s", tc.to)
			}
			if m.ToID != tc.owner {
				t.Errorf("filed for %q, want %q", m.ToID, tc.owner)
			}
			// The tag is what names which agent answers. Three of the five
			// doors resolved the address to an account and dropped it, so mail
			// to an agent here was filed and nothing ever ran.
			if m.Tag != tc.tag {
				t.Errorf("filed with tag %q, want %q", m.Tag, tc.tag)
			}
		})
	}
}

// Writing to an agent here wakes it, and writing to a person does not.
//
// The wake is the half that filing quietly leaves out, and the half nobody
// notices is missing: the message is in the inbox either way, so a door that
// files and does not publish looks like it worked.
func TestDeliverWakesAnAgentAndOnlyAnAgent(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	sender := account(t, "waker")
	account(t, "wakee")

	sub := event.Subscribe(event.MailForAgent)
	defer sub.Close()

	drain := func() int {
		// The publish goes through the broker, so the subscriber's callback
		// runs on its own goroutine and the channel fills a moment later.
		time.Sleep(100 * time.Millisecond)
		n := 0
		for {
			select {
			case <-sub.Chan:
				n++
			default:
				return n
			}
		}
	}
	drain()

	if _, err := Deliver(Outgoing{FromID: sender, Display: "W", To: "wakee@example.test",
		Subject: "hi", Body: "just mail"}); err != nil {
		t.Fatalf("plain local mail: %v", err)
	}
	if n := drain(); n != 0 {
		t.Errorf("untagged mail to a person woke %d agents; untagged mail is just mail", n)
	}

	if _, err := Deliver(Outgoing{FromID: sender, Display: "W", To: "waker+research@example.test",
		Subject: "hi", Body: "do something"}); err != nil {
		t.Fatalf("mail to an agent: %v", err)
	}
	if n := drain(); n != 1 {
		t.Errorf("mail to an agent here woke %d agents, want 1", n)
	}
}

// Nobody here by that name is refused by name, rather than by route.
func TestDeliverSaysWhoItCouldNotFind(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	sender := account(t, "asker")

	_, err := Deliver(Outgoing{FromID: sender, Display: "A", To: "nobody@example.test",
		Subject: "hi", Body: "hello"})
	if err == nil {
		t.Fatal("delivered to an address nobody here holds")
	}
	if !strings.Contains(err.Error(), "nobody") {
		t.Errorf("error %q does not name the address that failed", err)
	}
}

func account(t *testing.T, id string) string {
	t.Helper()
	if have, err := auth.GetAccount(id); err == nil && have != nil {
		return have.ID
	}
	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	return id
}

func storedByMessageID(id string) *Message {
	mutex.RLock()
	defer mutex.RUnlock()
	for _, m := range messages {
		if m != nil && m.MessageID == id {
			return m
		}
	}
	return nil
}

// Every door sends through Deliver, so where the recipient is stops being
// something each caller decides for itself.
//
// It was decided four times and three of them were wrong. The compose form at
// /inbox called ReplyOut, which is the half of it for mail leaving, so writing
// to somebody on your own instance came back "that is on this instance — that
// is not mail leaving it". The mail_send tool and the JSON handler both had
// their own local branch, and both resolved the address to an account and threw
// the +tag away, so mail to asim+research@ was filed and woke nothing. Only
// submission had the whole rule.
func TestNoDoorKeepsItsOwnCopyOfTheLocalBranch(t *testing.T) {
	// DeliverHere files a message and does not wake anything, so a door
	// calling it directly is a door that has decided the routing itself.
	// deliver.go is where that call belongs; the two exceptions are the
	// instance's own notices, which have no sender and reach nobody's agent.
	allowed := map[string]bool{
		"deliver.go": true,
		// Inbound off the network: smtp.go has already routed, and client.go
		// is the agent's own reply landing back in the account it came from.
		"smtp.go":   true,
		"client.go": true,
	}
	for _, name := range goFilesHere(t) {
		if allowed[name] || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if strings.Contains(readSource(t, name), "DeliverHere(Local{") {
			t.Errorf("%s files local mail itself instead of going through Deliver, "+
				"so it decides the route — and every copy of that decision so far "+
				"has dropped the tag that names the agent", name)
		}
	}
}

// The page a person types into can write to this instance.
//
// The narrowest statement of the reported bug: /inbox refused every local
// address, including agent@ and every asim+agent@, because it only ever called
// the function for mail leaving.
func TestTheComposeFormRoutesLikeEverythingElse(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "inbox", "new.go"))
	if err != nil {
		t.Fatalf("read inbox/new.go: %v", err)
	}
	s := string(src)
	if strings.Contains(s, "mail.ReplyOut(") || strings.Contains(s, "mail.SendOut(") {
		t.Error("/inbox still sends through the outbound-only path, so it refuses " +
			"every address on this instance")
	}
	if !strings.Contains(s, "mail.Deliver(") {
		t.Error("/inbox does not send through mail.Deliver")
	}
}

// A bare username is a local recipient. The tool has always taken one — "to":
// "asim" is the example in its own doc comment — and lifting the branch out of
// submission, which requires an @, must not quietly drop that.
func TestABareUsernameIsStillALocalAddress(t *testing.T) {
	src := readSource(t, "deliver.go")
	if !strings.Contains(src, "strings.LastIndex(to, \"@\")") {
		t.Fatal("deliver.go no longer looks for an @ at all")
	}
	if strings.Contains(src, "at <= 0 {\n\t\treturn") {
		t.Error("Deliver refuses an address with no @, so the bare username the " +
			"mail_send tool documents no longer reaches anybody")
	}
}

func goFilesHere(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			names = append(names, e.Name())
		}
	}
	return names
}
