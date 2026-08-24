package mail

import (
	"fmt"
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

// A message you wrote yourself is not mail that arrived.
//
// Writing to agent@ files it in your own inbox, because that is where the
// conversation is. It was landing unread, so a mail client rang for a message
// the person had just sent from that same client — "I sent to agent@micro.mu
// and I got my own email back to asim@micro.mu" — and /inbox counted it.
func TestYourOwnMessageDoesNotArriveUnread(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	me := account(t, "selfsender")
	other := account(t, "othersender")

	for _, to := range []string{"agent@example.test", "selfsender+research@example.test"} {
		id, err := Deliver(Outgoing{FromID: me, Display: "Me", To: to,
			Subject: "mine", Body: "something"})
		if err != nil {
			t.Fatalf("Deliver to %s: %v", to, err)
		}
		m := storedByMessageID(id)
		if m == nil {
			t.Fatalf("nothing filed for %s", to)
		}
		if !m.Read {
			t.Errorf("what I sent to %s landed unread in my own inbox", to)
		}
	}

	// And mail from somebody else is still mail, whatever else changes.
	id, err := Deliver(Outgoing{FromID: other, Display: "Them", To: "selfsender@example.test",
		Subject: "theirs", Body: "hello"})
	if err != nil {
		t.Fatalf("Deliver from another account: %v", err)
	}
	if m := storedByMessageID(id); m == nil || m.Read {
		t.Error("mail from somebody else arrived already read")
	}
}

// Asking your agent and being answered is one conversation, in the two folders
// a mail client expects.
//
// The whole reported symptom, end to end: "my inbox now has two mails of
// similar kind in the inbox, one from myself and one from the agent", and "it's
// like it responded to its own response". Three separate faults produced it.
//
// The question landed in INBOX rather than Sent, so a client rang for a message
// the person had sent from that same client. The answer was filed with the
// In-Reply-To header in ReplyTo — which is this instance's own id for the
// parent, a different namespace — so the parent lookup found nothing and the
// answer opened a conversation of its own. And a purely local answer was filed
// with no Message-ID at all, because one was minted after the delivery loop
// rather than before, so nothing later in the conversation had anything to
// name.
func TestAskingYourAgentIsOneConversation(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "asker2")

	asked, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "what is the weather", Body: "in London"})
	if err != nil {
		t.Fatalf("write to the agent: %v", err)
	}
	question := storedByMessageID(asked)
	if question == nil {
		t.Fatal("the question was not filed")
	}

	// The agent answering, by the path agent/mail actually uses: it holds the
	// header it is replying to and no id of ours for it.
	answered := "<answer.1@example.test>"
	if err := DeliverHere(Local{
		FromID: me, Display: "Micro", From: SharedAgentAddress(), To: me,
		Subject: "Re: what is the weather", Body: "raining",
		InReplyTo: asked, MessageID: answered,
	}); err != nil {
		t.Fatalf("the agent could not answer: %v", err)
	}
	answer := storedByMessageID(answered)
	if answer == nil {
		t.Fatal("the answer was not filed")
	}

	if answer.ThreadID != question.ThreadID {
		t.Errorf("the answer is its own conversation (%s) rather than part of the "+
			"one it answers (%s)", answer.ThreadID, question.ThreadID)
	}

	inbox, ok := imapFolder(me, "INBOX")
	if !ok {
		t.Fatal("no INBOX")
	}
	sent, ok := imapFolder(me, "Sent")
	if !ok {
		t.Fatal("no Sent")
	}
	if !holds(inbox, answered) {
		t.Error("the agent's answer is not in INBOX")
	}
	if holds(inbox, asked) {
		t.Error("what I wrote to my agent is in my INBOX, as though it had arrived")
	}
	if !holds(sent, asked) {
		t.Error("what I wrote to my agent is not in Sent either, so it is nowhere")
	}

	// And the same for the flat list the mail_inbox tool renders, which is what
	// the agent reads back when somebody asks it to check their mail. IMAP has
	// folders and this has none, so the question would otherwise sit in it
	// beside its own answer with nothing to say which was which.
	listed := ListMessages(me, 20)
	if holds(listed, asked) {
		t.Error("mail_inbox lists what I wrote to my agent as mail I received")
	}
	if !holds(listed, answered) {
		t.Error("mail_inbox does not list the agent's answer")
	}

	// One conversation on /mail, which renders threads rather than messages —
	// so both belong to it, and it is one row rather than two.
	mutex.RLock()
	box := inboxes[me]
	mutex.RUnlock()
	if box == nil {
		t.Fatal("no inbox was built for the account")
	}
	if n := len(box.Threads[question.ThreadID].Messages); n != 2 {
		t.Errorf("the conversation holds %d messages, want the question and the answer", n)
	}
}

func holds(msgs []*Message, messageID string) bool {
	for _, m := range msgs {
		if m != nil && m.MessageID == messageID {
			return true
		}
	}
	return false
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

// A message bigger than the limit is refused with the size and the limit,
// wherever it came from. A truncated send is worse than a refused one.
// See issue 1465.
func TestDeliverRefusesAnOversizedMessage(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	sender := account(t, "sender")

	big := strings.Repeat("x", maxOutgoingBytes)
	_, err := Deliver(Outgoing{FromID: sender, Display: "A", To: "someone@example.test",
		Subject: "hi", Body: big})
	if err == nil {
		t.Fatal("delivered a message over the limit")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", maxOutgoingBytes)) {
		t.Errorf("error %q does not say the size or the limit", err)
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
