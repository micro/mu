package mail

// The guards on waking an agent from inbound mail, and the address that needs
// nothing remembered.
//
// The hook itself needs the agent and the roster, so it is wired in main.go.
// What lives here is the decision to call it at all, and the cases where
// calling it would be wrong: spam, our own outbound, a plus address nobody
// claimed, a sender who is not who they say, and a stranger who is. Each of
// the first three is a loop or a bill — an agent replying to its own reply
// costs a model call per turn, forever. The last two are worse: an agent runs
// with its owner's scope and tools, so whoever can wake it can act as them.
//
// This file used to carry its own copy of the rule and check the copy. The
// rule now lives in inbound_agent.go and this tests that.

import (
	"strings"
	"testing"

	"mu/internal/auth"
)

// req builds a wake request against a world where the domain is micro.mu and
// wakeowner's verified address is asim@aslam.me.
func req(t *testing.T, r wakeRequest, known ...string) bool {
	t.Helper()
	withDomain(t, "micro.mu")

	prevKnown := KnownSender
	restore := withHandler(t)
	KnownSender = func(_, addr string) bool {
		for _, k := range known {
			if strings.EqualFold(k, addr) {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { restore(); KnownSender = prevKnown })

	if r.Owner == "" {
		r.Owner = "wakeowner"
	}
	return mayDispatch(r)
}

// tagged is mail to one agent's own address, authenticated, not spam.
func tagged(from string) wakeRequest {
	return wakeRequest{Owner: "wakeowner", Tag: "research", From: from,
		To: "asim+research@micro.mu", Authenticated: true}
}

// shared is mail to agent@micro.mu — no tag, the default agent answers.
func shared(from string) wakeRequest {
	return wakeRequest{Owner: "wakeowner", Shared: true, From: from,
		To: "agent@micro.mu", Authenticated: true}
}

func wakeOwner(t *testing.T) {
	t.Helper()
	if _, err := auth.GetAccount("wakeowner"); err == nil {
		return
	}
	err := auth.Create(&auth.Account{
		ID: "wakeowner", Name: "wakeowner", Secret: "s",
		Email: "asim@aslam.me", EmailVerified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWhenInboundMailWakesAnAgent(t *testing.T) {
	wakeOwner(t)

	// The owner, writing to their own agent. The case the whole feature exists
	// for, and it has to keep working.
	if !req(t, tagged("asim@aslam.me")) {
		t.Error("the account holder cannot wake their own agent from their own verified address")
	}
	// Somebody in the address book. Deliberate: you decided they could reach
	// you, and an agent answering your contacts is the point of giving it an
	// address at all.
	if !req(t, tagged("colleague@example.com"), "colleague@example.com") {
		t.Error("a contact cannot reach an agent, so the address is only useful to its owner")
	}

	// A stranger who knows the address. This is the hole: the tag was the only
	// thing standing between anyone and an agent holding the owner's tools.
	if req(t, tagged("stranger@example.com")) {
		t.Error("a stranger who guessed the tag can drive the agent and spend the owner's credits")
	}
	// A known address on unauthenticated mail. From headers are free to write,
	// so without SPF or DKIM "from your contact" means nothing.
	unauth := tagged("colleague@example.com")
	unauth.Authenticated = false
	if req(t, unauth, "colleague@example.com") {
		t.Error("an unauthenticated From header is enough to impersonate a contact")
	}
	forged := tagged("asim@aslam.me")
	forged.Authenticated = false
	if req(t, forged) {
		t.Error("the owner's address can be forged into a wake-up")
	}

	// The guards that were already here.
	spam := tagged("asim@aslam.me")
	spam.IsSpam = true
	if req(t, spam) {
		t.Error("spam wakes an agent")
	}
	if req(t, tagged("agent@micro.mu")) {
		t.Error("our own reply wakes an agent, which is an unbounded exchange with ourselves")
	}
	if req(t, tagged("Agent@Micro.MU")) {
		t.Error("the loop guard is case-sensitive, so a differently-cased reply loops")
	}
	// An agent now answers from its own address, so that address writing to
	// itself is the same loop wearing a different name.
	if req(t, tagged("asim+research@micro.mu")) {
		t.Error("an agent's own address can wake it, which is a loop per model call")
	}

	// Untagged mail to your own address is just mail. Every newsletter in the
	// inbox would otherwise start a run.
	plain := wakeRequest{From: "asim@aslam.me", To: "asim@micro.mu", Authenticated: true}
	if req(t, plain) {
		t.Error("untagged mail wakes an agent")
	}
}

// agent@<domain> takes nothing remembered: no plus convention, no recalling
// what you named the agent. It is answered by the same rules.
func TestTheSharedAddressAnswersTheAccountItResolvedFrom(t *testing.T) {
	wakeOwner(t)

	if !req(t, shared("asim@aslam.me")) {
		t.Error("the owner cannot reach their default agent at the address that needs no remembering")
	}
	if !req(t, shared("colleague@example.com"), "colleague@example.com") {
		t.Error("a contact cannot use the shared address")
	}
	if req(t, shared("stranger@example.com")) {
		t.Error("the shared address answers strangers, which is an open agent for anyone who writes in")
	}
	unauth := shared("asim@aslam.me")
	unauth.Authenticated = false
	if req(t, unauth) {
		t.Error("the shared address takes an unauthenticated From header")
	}
	// Its own replies must not re-enter, and this is the address they come
	// from when no named agent answered.
	if req(t, shared("agent@micro.mu")) {
		t.Error("the shared address answers itself")
	}
}

// With no address book wired, the owner is still the owner. An instance that
// forgets to set the hook must fail closed for strangers, not open.
func TestWithNoAddressBookOnlyTheOwnerGetsThrough(t *testing.T) {
	wakeOwner(t)
	withDomain(t, "micro.mu")

	prevKnown := KnownSender
	restore := withHandler(t)
	KnownSender = nil
	defer func() { restore(); KnownSender = prevKnown }()

	if !mayDispatch(tagged("asim@aslam.me")) {
		t.Error("the owner cannot reach their agent when no address book is wired")
	}
	if mayDispatch(tagged("stranger@example.com")) {
		t.Error("an unwired address book lets everybody in")
	}
}

func TestAnInstanceWithNothingListeningNeverWakesAnything(t *testing.T) {
	wakeOwner(t)
	withDomain(t, "micro.mu")

	inboundMu.Lock()
	prev := inboundHandlers
	inboundHandlers = map[string][]InboundHandler{}
	inboundMu.Unlock()
	defer func() {
		inboundMu.Lock()
		inboundHandlers = prev
		inboundMu.Unlock()
	}()

	if mayDispatch(tagged("asim@aslam.me")) {
		t.Error("an instance with nothing registered still decides to wake something")
	}
}

// withHandler registers a do-nothing handler so the guard has something to
// dispatch to, and hands back the undo.
func withHandler(t *testing.T) func() {
	t.Helper()
	inboundMu.Lock()
	prev := inboundHandlers
	inboundHandlers = map[string][]InboundHandler{
		Tagged:       {func(InboundMail) {}},
		AgentMailbox: {func(InboundMail) {}},
	}
	inboundMu.Unlock()
	return func() {
		inboundMu.Lock()
		inboundHandlers = prev
		inboundMu.Unlock()
	}
}

// The shared mailbox has no owner in its own name, so whose mail it is comes
// from who sent it. Only a proved address counts.
func TestTheSharedMailboxResolvesItsOwnerFromTheSender(t *testing.T) {
	wakeOwner(t)

	if acc := AccountForVerifiedEmail("asim@aslam.me"); acc == nil || acc.ID != "wakeowner" {
		t.Errorf("a verified address did not resolve to its account: %+v", acc)
	}
	if acc := AccountForVerifiedEmail("  ASIM@Aslam.me "); acc == nil {
		t.Error("case and whitespace from a mail header defeat the lookup")
	}
	for _, nobody := range []string{"stranger@example.com", ""} {
		if acc := AccountForVerifiedEmail(nobody); acc != nil {
			t.Errorf("%q resolved to account %s", nobody, acc.ID)
		}
	}
}

// The rule has to stay wired into the path, and the path has to keep storing
// the message first, so a failure to answer never loses mail.
func TestTheGuardIsStillInThePath(t *testing.T) {
	src := readSource(t, "smtp.go")
	for _, want := range []string{
		"}, wakeRequest{",
		"Authenticated: dkimPass || s.spfPass,",
		"Shared:        sharedAgentMail,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the inbound-agent guard lost %q", want)
		}
	}

	store := strings.Index(src, "if err := SendMessageTo(")
	woken := strings.Index(src, "deliverInbound(InboundMail{")
	if store < 0 || woken < 0 || woken < store {
		t.Error("the agent is woken before the message is stored, so a panic there loses the mail")
	}

	rule := readSource(t, "inbound_agent.go")
	for _, want := range []string{"Authenticated", "senderKnownTo", "SenderIsAccountOwner"} {
		if !strings.Contains(rule, want) {
			t.Errorf("the rule lost %q", want)
		}
	}

	// The shared mailbox resolves by sender, and drops rather than bounces
	// when nobody owns the address.
	if !strings.Contains(src, "AccountForVerifiedEmail(fromAddr.Address)") {
		t.Error("agent@<domain> no longer works out whose mail it is from who sent it")
	}
}

// A machine writing on its own account must not wake an agent.
//
// RFC 3834 exists because two automatic responders will talk to each other
// until somebody notices, and an agent is the most expensive possible
// participant: a model call per turn, forever.
//
// A DMARC report is the case that made this obvious. It is DKIM-signed by
// Google, so it passes every other check here, and there is nobody on the other
// end to answer.
func TestMailAMachineSentDoesNotWakeAnAgent(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")
	// mayDispatch declines to reason about who may wake what on an instance
	// where nothing is listening, so there has to be something listening.
	Inbound(AgentMailbox, func(InboundMail) {})

	base := wakeRequest{
		Owner: "alice", Tag: "news", From: "known@example.com",
		To: "alice+news@mu.example", Authenticated: true, Owned: true,
	}
	if !mayDispatch(base) {
		t.Fatal("the control case does not dispatch, so this test proves nothing")
	}

	machine := base
	machine.Machine = true
	if mayDispatch(machine) {
		t.Error("mail a machine sent woke the agent")
	}

	// Owned is the strongest evidence there is — a token was presented before
	// the message was accepted — and it still must not override this. A mail
	// client set to forward reports is signed in as you.
	if !machine.Owned {
		t.Fatal("this case is only interesting while Owned is set")
	}
}

// The headers that say so, and the ones that do not.
func TestMachineMailReadsTheStandardHeaders(t *testing.T) {
	for _, tt := range []struct {
		name   string
		header map[string]string
		want   bool
	}{
		{"a person", map[string]string{"From": "someone@example.com"}, false},
		{"RFC 3834", map[string]string{"Auto-Submitted": "auto-generated"}, true},
		{"a DMARC report", map[string]string{"Auto-Submitted": "auto-generated",
			"Content-Type": "multipart/report; report-type=disposition-notification"}, true},
		// "no" is the value RFC 3834 reserves for mail a person actually sent,
		// so a sender being explicit about it must not be punished for saying so.
		{"explicitly not", map[string]string{"Auto-Submitted": "no"}, false},
		{"bulk", map[string]string{"Precedence": "bulk"}, true},
		{"a mailing list", map[string]string{"List-Id": "<dev.example.com>"}, true},
		{"unsubscribable", map[string]string{"List-Unsubscribe": "<mailto:x@example.com>"}, true},
		{"an Exchange auto-reply", map[string]string{"X-Auto-Response-Suppress": "All"}, true},
		{"an empty header is not a header", map[string]string{"List-Id": "  "}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := machineMail(testHeader(tt.header)); got != tt.want {
				t.Errorf("machineMail(%v) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

// testHeader is the one method machineMail needs, so the test does not have to
// build a whole parsed message to ask about four headers.
type testHeader map[string]string

func (h testHeader) Get(k string) string {
	for name, v := range h {
		if strings.EqualFold(name, k) {
			return v
		}
	}
	return ""
}
