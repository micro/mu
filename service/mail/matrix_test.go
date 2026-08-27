package mail

// Every shape of recipient, every consequence.
//
// The bugs this covers were all one bug seen from five places, and each was
// found by hand after somebody hit it: an address that would not route, a tag
// dropped so an agent never woke, a question filed as though it had arrived, an
// answer that started its own conversation. The table is here so the next one
// is found by the suite instead.
//
// Four questions are asked of every delivery, because each of them was wrong
// for some address at some point and none of them implies the others:
//
//	whose mailbox   — where it was filed
//	which tag       — which agent it names, or none
//	woken           — whether an agent was asked to answer
//	which folder    — INBOX or Sent, and never both
//
// TestAskingYourAgentIsOneConversation in deliver_test.go covers threading
// across a turn; this one covers a single delivery in every form it takes.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/event"
)

// sender writes to every kind of address there is.
func TestEveryShapeOfRecipient(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	me := account(t, "mxsender")
	account(t, "mxother")
	// A stranger here: no contact record either way, which is what makes
	// writing to their agent a request rather than a licence.
	account(t, "mxstranger")

	cases := []struct {
		name string
		to   string

		// filed is whose mailbox it lands in. Empty means the delivery is
		// expected to fail.
		filed string
		tag   string
		wakes bool
		// inSenderSent is whether it belongs in the sender's Sent folder. False
		// for a delivery that fails.
		inSenderSent bool
		// inRecipientInbox is whether it belongs in the recipient's INBOX. A
		// message you wrote is never in your own, whoever it was addressed to.
		inRecipientInbox bool
		// read is whether it is filed already read.
		read bool
		errs string
	}{
		{
			name: "a bare username",
			to:   "mxother",
			// A convenience the mail_send tool documents: {"to": "asim"}.
			filed: "mxother", tag: "", wakes: false,
			inSenderSent: true, inRecipientInbox: true, read: false,
		},
		{
			name:  "a full local address",
			to:    "mxother@example.test",
			filed: "mxother", tag: "", wakes: false,
			inSenderSent: true, inRecipientInbox: true, read: false,
		},
		{
			name:  "a local address in the wrong case",
			to:    "MxOther@EXAMPLE.TEST",
			filed: "mxother", tag: "", wakes: false,
			inSenderSent: true, inRecipientInbox: true, read: false,
		},
		{
			name: "somebody else's agent",
			to:   "mxstranger+research@example.test",
			// Filed and tagged, because it is their mail and the tag is part of
			// the address. Not woken: waking somebody's agent spends their
			// credits, and a stranger here is a stranger. See mayDispatch —
			// Owned is only true for your own.
			filed: "mxstranger", tag: "research", wakes: false,
			inSenderSent: true, inRecipientInbox: true, read: false,
		},
		{
			name:  "your own agent",
			to:    "mxsender+research@example.test",
			filed: "mxsender", tag: "research", wakes: true,
			// Your own message is never in your own INBOX. It is in Sent.
			inSenderSent: true, inRecipientInbox: false, read: true,
		},
		{
			name: "your own plain address",
			to:   "mxsender@example.test",
			// A note to yourself. No tag, so nothing is woken — untagged mail
			// to your own address is just mail.
			filed: "mxsender", tag: "", wakes: false,
			inSenderSent: true, inRecipientInbox: false, read: true,
		},
		{
			name: "the shared agent address",
			to:   "agent@example.test",
			// agent@ is not an account: it resolves to whoever wrote to it.
			filed: "mxsender", tag: "", wakes: true,
			inSenderSent: true, inRecipientInbox: false, read: true,
		},
		{
			name:  "the shared address naming an agent",
			to:    "agent+news@example.test",
			filed: "mxsender", tag: "news", wakes: true,
			inSenderSent: true, inRecipientInbox: false, read: true,
		},
		{
			name:  "the shared address in the wrong case",
			to:    "AGENT@example.test",
			filed: "mxsender", tag: "", wakes: true,
			inSenderSent: true, inRecipientInbox: false, read: true,
		},
		{
			name: "nobody here by that name",
			to:   "mxnosuch@example.test",
			errs: "mxnosuch",
		},
		{
			name: "a bare name nobody here holds",
			to:   "mxnosuch",
			errs: "mxnosuch",
		},
		{
			name: "no recipient at all",
			to:   "   ",
			errs: "no recipient",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := event.Subscribe(event.MailForAgent)
			defer sub.Close()

			id, err := Deliver(Outgoing{
				FromID: me, Display: "Sender", To: tc.to,
				Subject: "subject for " + tc.name, Body: "body",
			})

			if tc.errs != "" {
				if err == nil {
					t.Fatalf("Deliver to %q succeeded; it should have been refused", tc.to)
				}
				if !strings.Contains(err.Error(), tc.errs) {
					t.Errorf("error %q does not mention %q, so it does not say what "+
						"was wrong with the address", err, tc.errs)
				}
				return
			}
			if err != nil {
				t.Fatalf("Deliver to %q: %v", tc.to, err)
			}

			m := storedByMessageID(id)
			if m == nil {
				t.Fatal("nothing was filed")
			}
			if m.ToID != tc.filed {
				t.Errorf("filed for %q, want %q", m.ToID, tc.filed)
			}
			if m.Tag != tc.tag {
				t.Errorf("filed with tag %q, want %q — the tag is what names "+
					"which agent answers", m.Tag, tc.tag)
			}
			if m.Read != tc.read {
				t.Errorf("filed with Read=%v, want %v", m.Read, tc.read)
			}

			if woke := wokeAnAgent(sub); woke != tc.wakes {
				if tc.wakes {
					t.Error("no agent was woken, so the message sits in an inbox " +
						"and is never answered")
				} else {
					t.Error("an agent was woken for a message that should not " +
						"have started a run")
				}
			}

			// Folders. INBOX and Sent split on one predicate, so a message is
			// in exactly one of them for any given account.
			if got := inFolder(t, me, imapSent, id); got != tc.inSenderSent {
				t.Errorf("in the sender's Sent = %v, want %v", got, tc.inSenderSent)
			}
			if got := inFolder(t, me, imapInbox, id); got != false {
				t.Error("what I wrote is in my own INBOX, as though it had arrived")
			}
			if tc.filed != me {
				if got := inFolder(t, tc.filed, imapInbox, id); got != tc.inRecipientInbox {
					t.Errorf("in the recipient's INBOX = %v, want %v", got, tc.inRecipientInbox)
				}
				if inFolder(t, tc.filed, imapSent, id) {
					t.Error("mail I sent them is in their Sent folder")
				}
			}

			// The flat list the mail_send tool and the agent read back.
			if listed := holds(ListMessages(tc.filed, 50), id); listed == (tc.filed == me) {
				if tc.filed == me {
					t.Error("mail_inbox lists what I wrote as mail I received")
				} else {
					t.Error("mail_inbox does not list a message that was delivered")
				}
			}
		})
	}
}

// Writing to somebody else's agent does not spend their credits.
//
// mayDispatch treats Owned as licence to skip asking whether the account has
// ever heard of the sender, and Owned means "signed in as *this account*" —
// the account the mail is for, not the one that sent it. It was passed as a
// constant, which was true in submission where the only reachable case was
// your own agent, and became a hole the moment the rule was shared with the
// doors where it is not: any account here could have woken any other's agent.
func TestWakingSomebodyElsesAgentIsNotYoursToDo(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	me := account(t, "wakerouter")
	victim := account(t, "wakevictim")

	sub := event.Subscribe(event.MailForAgent)
	defer sub.Close()

	if _, err := Deliver(Outgoing{FromID: me, Display: "Me",
		To: victim + "+research@example.test", Subject: "do my work", Body: "spend"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if wokeAnAgent(sub) {
		t.Error("writing to a stranger's agent woke it, so anybody here can " +
			"spend anybody else's credits by choosing an address")
	}

	// And your own still wakes, so the check has not simply turned the feature
	// off.
	if _, err := Deliver(Outgoing{FromID: me, Display: "Me",
		To: me + "+research@example.test", Subject: "my own", Body: "work"}); err != nil {
		t.Fatalf("Deliver to my own agent: %v", err)
	}
	if !wokeAnAgent(sub) {
		t.Error("writing to my own agent no longer wakes it")
	}
}

// A conversation stays one conversation however its parent is named.
//
// Three names for the same parent reach SendMessageTo, and only one was ever
// looked up. A caller that holds the header and not our id — which is every
// reply an agent makes, and everything arriving off the network — got no
// parent and started a conversation of its own.
func TestAThreadIsFoundByAnyNameForItsParent(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "threader")

	root, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "opening", Body: "first"})
	if err != nil {
		t.Fatalf("open the conversation: %v", err)
	}
	opening := storedByMessageID(root)
	if opening == nil {
		t.Fatal("the opening message was not filed")
	}

	for _, tc := range []struct {
		name  string
		reply Local
	}{
		{
			name:  "by the In-Reply-To header",
			reply: Local{InReplyTo: root},
		},
		{
			name:  "by our own id for the parent",
			reply: Local{ReplyTo: opening.ID},
		},
		{
			name: "by the References chain when In-Reply-To was dropped",
			// Some clients send References and no In-Reply-To. Threading onto
			// the conversation beats starting a second one beside it.
			reply: Local{References: "<older@elsewhere.test> " + root},
		},
		{
			name:  "by both, agreeing",
			reply: Local{ReplyTo: opening.ID, InReplyTo: root},
		},
		{
			name: "by the header when our id is stale",
			// A caller that has an id for a message we no longer hold still
			// has the header, and the header still names the conversation.
			reply: Local{ReplyTo: "no-such-message", InReplyTo: root},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answered := "<answer." + strings.ReplaceAll(tc.name, " ", "-") + "@example.test>"
			l := tc.reply
			l.FromID, l.Display, l.From, l.To = me, "Micro", SharedAgentAddress(), me
			l.Subject, l.Body, l.MessageID = "Re: opening", "answer", answered
			if err := DeliverHere(l); err != nil {
				t.Fatalf("the agent could not answer: %v", err)
			}
			answer := storedByMessageID(answered)
			if answer == nil {
				t.Fatal("the answer was not filed")
			}
			if answer.ThreadID != opening.ThreadID {
				t.Errorf("the answer is its own conversation (%s), not part of "+
					"the one it answers (%s)", answer.ThreadID, opening.ThreadID)
			}
		})
	}

	// And something unrelated is still its own conversation, so threading has
	// not become "everything is one thread".
	loose, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "unrelated", Body: "different question"})
	if err != nil {
		t.Fatalf("second conversation: %v", err)
	}
	if m := storedByMessageID(loose); m == nil || m.ThreadID == opening.ThreadID {
		t.Error("an unrelated message joined the previous conversation")
	}
}

// Three turns, each answering the last, is still one conversation.
//
// A single reply threading correctly is not the same as a conversation holding
// together: the second question answers the agent's answer, and it can only
// find the thread if the answer was filed with a Message-ID of its own. Purely
// local answers had none — one was minted after the delivery loop rather than
// before it — so a conversation came apart on the third turn rather than the
// second, which is why it survived the first fix.
func TestAConversationSurvivesMoreThanOneTurn(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "multiturn")

	first, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "turn one", Body: "question one"})
	if err != nil {
		t.Fatalf("turn one: %v", err)
	}
	want := storedByMessageID(first).ThreadID

	answerOne := "<a1@example.test>"
	if err := DeliverHere(Local{
		FromID: me, Display: "Micro", From: SharedAgentAddress(), To: me,
		Subject: "Re: turn one", Body: "answer one",
		InReplyTo: first, MessageID: answerOne,
	}); err != nil {
		t.Fatalf("answer one: %v", err)
	}

	// Turn two answers the answer, which is what a person replying in their
	// mail client actually sends.
	second, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "Re: turn one", Body: "question two",
		InReplyTo: answerOne, References: first + " " + answerOne})
	if err != nil {
		t.Fatalf("turn two: %v", err)
	}

	answerTwo := "<a2@example.test>"
	if err := DeliverHere(Local{
		FromID: me, Display: "Micro", From: SharedAgentAddress(), To: me,
		Subject: "Re: turn one", Body: "answer two",
		InReplyTo: second, References: first + " " + answerOne + " " + second,
		MessageID: answerTwo,
	}); err != nil {
		t.Fatalf("answer two: %v", err)
	}

	for _, id := range []string{first, answerOne, second, answerTwo} {
		m := storedByMessageID(id)
		if m == nil {
			t.Fatalf("%s was not filed", id)
		}
		if m.ThreadID != want {
			t.Errorf("%s is in conversation %s, want %s — four turns, one "+
				"conversation", id, m.ThreadID, want)
		}
	}

	mutex.RLock()
	box := inboxes[me]
	mutex.RUnlock()
	if box == nil || box.Threads[want] == nil {
		t.Fatal("the conversation was not built into the inbox")
	}
	if n := len(box.Threads[want].Messages); n != 4 {
		t.Errorf("the conversation holds %d messages, want 4", n)
	}
}

// The agent's own reply path threads, and the conversation survives the turn
// after it.
//
// The tests above build the answer with DeliverHere and an id of their own,
// which is the shape but not the path — so removing the Message-ID that
// SendReplyAll mints for a purely local answer broke nothing they assert. That
// is the gap that let the bug exist: an answer filed with no id of its own is
// fine until something tries to reply to it, so the conversation comes apart on
// the turn after the one anybody checks.
func TestTheAgentsOwnReplyCanBeRepliedTo(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "replypath")

	asked, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "first", Body: "question"})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	want := storedByMessageID(asked).ThreadID

	// The agent answering, through the function agent/mail actually calls.
	answerID, err := SendReplyAll(me, "Micro", SharedAgentAddress(), me, nil,
		"Re: first", "answer", "<p>answer</p>", asked, asked)
	if err != nil {
		t.Fatalf("the agent could not answer: %v", err)
	}
	if strings.TrimSpace(answerID) == "" {
		t.Fatal("the answer was filed with no Message-ID, so nothing can reply to it")
	}
	if answerID == asked {
		t.Fatal("the answer reused the id of the message it was answering, so a " +
			"reply to it threads onto the wrong message")
	}
	answer := byMessageID(answerID)
	if answer == nil {
		t.Fatal("the answer was not filed")
	}
	if answer.ThreadID != want {
		t.Errorf("the answer is its own conversation (%s), not part of the one it "+
			"answers (%s)", answer.ThreadID, want)
	}

	// And the turn after: replying to the answer stays in the conversation,
	// which is only possible because the answer has an id of its own.
	followUp, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "agent@example.test",
		Subject: "Re: first", Body: "and another thing",
		InReplyTo: answerID, References: asked + " " + answerID})
	if err != nil {
		t.Fatalf("follow up: %v", err)
	}
	if m := storedByMessageID(followUp); m == nil || m.ThreadID != want {
		t.Errorf("the follow-up left the conversation, so replying to the agent's "+
			"own answer starts a new thread (%v)", m)
	}
}

func byMessageID(headerID string) *Message {
	mutex.RLock()
	defer mutex.RUnlock()
	return byMessageIDUnlocked(headerID)
}

// An address off this instance still relays rather than being filed here.
//
// The routing branch has one job and it cuts both ways: the local half was the
// half that was broken, and a fix that quietly filed external mail locally
// would be worse than what it replaced.
func TestAnAddressElsewhereStillLeaves(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")
	me := account(t, "relayer")

	before := countStored()
	_, err := Deliver(Outgoing{FromID: me, Display: "Me", To: "somebody@elsewhere.test",
		Subject: "outbound", Body: "hello"})
	// No relay is configured in a test binary, so this fails — the point is
	// which way it failed. A message filed in a local mailbox would mean the
	// address was treated as local.
	if err == nil {
		t.Skip("this instance can relay, so there is nothing to assert here")
	}
	if strings.Contains(err.Error(), "no account here called") {
		t.Fatalf("an external address was looked up as a local account: %v", err)
	}
	if countStored() != before {
		t.Error("an external address was filed in a mailbox on this instance")
	}
}

// wokeAnAgent reports whether a wake was published, allowing for the broker
// delivering on its own goroutine.
func wokeAnAgent(sub *event.Subscription) bool {
	select {
	case <-sub.Chan:
		return true
	case <-time.After(300 * time.Millisecond):
		return false
	}
}

func inFolder(t *testing.T, accountID, folder, messageID string) bool {
	t.Helper()
	msgs, ok := imapFolder(accountID, folder)
	if !ok {
		t.Fatalf("no folder called %q", folder)
	}
	return holds(msgs, messageID)
}

func countStored() int {
	mutex.RLock()
	defer mutex.RUnlock()
	return len(messages)
}
