package mail

// The mail stack, end to end, over real sockets.
//
// Every test here is a thing that broke. Not hypothetically — each one shipped,
// and each was found by a person using a mail client rather than by anything in
// this package, which is the reason the file exists.
//
//	an open relay          the MTA took any recipient from localhost, guarded by
//	                       a comment promising an AUTH check that never ran
//	a mailbox nobody       IMAP compared the username against the display name,
//	could open             so everybody who signed in with Google was refused
//	a client that          AUTH was neither advertised nor possible, because the
//	could not send         Login method satisfied no interface go-smtp calls
//	an agent that          writing to agent@ from a client filed the mail and
//	never answered         woke nothing: twice over, first because agent@ was
//	                       looked up as an account and then because the rule
//	                       asked whether the From header could be trusted
//	mail with no trace     sending from a client recorded nothing anywhere
//	a blank sender         the envelope used a display name as an address
//
// Nothing here needs a network, an API key or a model. Outbound is captured by
// a relay sink on a port of its own: SMTP_RELAY_HOST is how an operator points
// this instance at a smarthost, so pointing it at a test is the same code path
// a real send takes, with the last hop landing somewhere we can read.
//
// A stack per test, on ephemeral ports, so they neither collide nor depend on
// the order they run in.

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/auth"

	smtpd "github.com/emersion/go-smtp"
)

// ── the harness ─────────────────────────────────────────────────

type stack struct {
	t          *testing.T
	acc        *auth.Account
	token      string
	domain     string
	submission string // host:port
	mta        string // host:port
	sink       *sink
}

// newStack brings up submission, the MTA and a relay sink, with one account.
func newStack(t *testing.T, id string) *stack {
	t.Helper()

	// A domain, because Reachable() asks for one and ReplyOut refuses to send
	// anywhere without it — "there is no mail domain here to send it from".
	// ConfiguredDomain falls back to "localhost" and Domain() does not, so a
	// test that set neither had an address to receive at and no domain to send
	// from.
	t.Setenv("MAIL_DOMAIN", "mu.test")
	domain := ConfiguredDomain()

	acc := &auth.Account{ID: id, Name: strings.ToUpper(id[:1]) + id[1:] + " Person",
		Admin: true, Created: time.Now()}
	if err := auth.Create(acc); err != nil {
		have, err := auth.GetAccount(id)
		if err != nil {
			t.Fatal(err)
		}
		acc = have
	}
	_, token, err := auth.CreateToken(acc.ID, "mail client", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	s := &stack{t: t, acc: acc, token: token, domain: domain, sink: newSink(t)}
	s.submission = serveOn(t, &submissionBackend{})
	s.mta = serveOn(t, &Backend{})

	// Outbound goes to the sink rather than to somebody's MX.
	t.Setenv("SMTP_RELAY_HOST", s.sink.addr)

	// The spam filter is not what these tests are about, and a bare message
	// from an unknown sender with no SPF or DKIM scores as spam — correctly —
	// which files it in Junk and makes every assertion about INBOX fail for a
	// reason that has nothing to do with what is being tested. Its own tests
	// cover it.
	spamMutex.Lock()
	savedFilter := *spamFilter
	spamFilter.Enabled = false
	spamMutex.Unlock()
	t.Cleanup(func() {
		spamMutex.Lock()
		*spamFilter = savedFilter
		spamMutex.Unlock()
	})

	// The mailbox starts empty, and is emptied again after. Tests in this
	// package share the package-level store.
	mutex.Lock()
	messages = nil
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = nil; mutex.Unlock() })

	return s
}

func serveOn(t *testing.T, be smtpd.Backend) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := smtpd.NewServer(be)
	srv.Domain = ConfiguredDomain()
	srv.AllowInsecureAuth = true
	srv.MaxRecipients = submissionMaxRecipients
	go srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func (s *stack) address() string { return EmailForUser(s.acc.ID, s.domain) }

// submit sends one message through the submission server, as a mail client
// would: EHLO, AUTH PLAIN, MAIL FROM, RCPT TO, DATA.
func (s *stack) submit(from, to, raw string) error {
	s.t.Helper()
	c := dialSMTP(s.t, s.submission)
	c.do("EHLO client")
	if got := c.do("AUTH PLAIN " + plainAuth(s.acc.ID, s.token)); !strings.Contains(got, "235") {
		return fmt.Errorf("auth: %s", strings.TrimSpace(got))
	}
	if got := c.do("MAIL FROM:<" + from + ">"); !strings.Contains(got, "250") {
		return fmt.Errorf("mail from: %s", strings.TrimSpace(got))
	}
	if got := c.do("RCPT TO:<" + to + ">"); !strings.Contains(got, "250") {
		return fmt.Errorf("rcpt to: %s", strings.TrimSpace(got))
	}
	if got := c.do("DATA"); !strings.Contains(got, "354") {
		return fmt.Errorf("data: %s", strings.TrimSpace(got))
	}
	body := strings.ReplaceAll(raw, "\n", "\r\n")
	if got := c.do(body + "\r\n."); !strings.Contains(got, "250") {
		return fmt.Errorf("message refused: %s", strings.TrimSpace(got))
	}
	return nil
}

// arrive delivers a message into the MTA the way the internet does: no
// authentication, straight to a local recipient.
func (s *stack) arrive(from, to, raw string) error {
	s.t.Helper()
	c := dialSMTP(s.t, s.mta)
	c.do("EHLO somewhere.example.com")
	if got := c.do("MAIL FROM:<" + from + ">"); !strings.Contains(got, "250") {
		return fmt.Errorf("mail from: %s", strings.TrimSpace(got))
	}
	if got := c.do("RCPT TO:<" + to + ">"); !strings.Contains(got, "250") {
		return fmt.Errorf("rcpt to: %s", strings.TrimSpace(got))
	}
	if got := c.do("DATA"); !strings.Contains(got, "354") {
		return fmt.Errorf("data: %s", strings.TrimSpace(got))
	}
	body := strings.ReplaceAll(raw, "\n", "\r\n")
	if got := c.do(body + "\r\n."); !strings.Contains(got, "250") {
		return fmt.Errorf("message refused: %s", strings.TrimSpace(got))
	}
	return nil
}

// imap opens an authenticated IMAP session against this account.
func (s *stack) imap() *client {
	s.t.Helper()
	c := dial(s.t)
	c.ok("LOGIN " + s.acc.ID + " " + s.token)
	return c
}

// ── the relay sink ──────────────────────────────────────────────

// sink is an SMTP server that keeps what it is given, standing in for the
// recipient's mail server.
type sink struct {
	addr string
	mu   sync.Mutex
	got  []string
}

func newSink(t *testing.T) *sink {
	t.Helper()
	s := &sink{}
	s.addr = serveOn(t, &sinkBackend{s: s})
	return s
}

// waitFor returns the first captured message containing want, or fails.
//
// Polled rather than assumed: the relay happens on the connection's own
// goroutine and DATA returns as soon as this instance has accepted the message,
// not when the next hop has taken it.
func (s *sink) waitFor(t *testing.T, want string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, m := range s.got {
			if strings.Contains(m, want) {
				s.mu.Unlock()
				return m
			}
		}
		n := len(s.got)
		s.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		_ = n
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Fatalf("nothing relayed containing %q; %d message(s) arrived:\n%s",
		want, len(s.got), strings.Join(s.got, "\n---\n"))
	return ""
}

func (s *sink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

type sinkBackend struct{ s *sink }

func (b *sinkBackend) NewSession(*smtpd.Conn) (smtpd.Session, error) {
	return &sinkSession{s: b.s}, nil
}

type sinkSession struct{ s *sink }

func (*sinkSession) Mail(string, *smtpd.MailOptions) error { return nil }
func (*sinkSession) Rcpt(string, *smtpd.RcptOptions) error { return nil }
func (*sinkSession) Reset()                                {}
func (*sinkSession) Logout() error                         { return nil }
func (s *sinkSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.s.mu.Lock()
	s.s.got = append(s.s.got, string(b))
	s.s.mu.Unlock()
	return nil
}

// ── the tests ───────────────────────────────────────────────────

// Mail arrives, and a mail client can read it.
//
// The whole point of the thing: somebody writes to your address and it turns up
// in whatever client you already have open, with a sender you can see. The
// sender is asserted because it was blank — imapEnvelope used Message.From,
// which is a display name for anything delivered locally, and sent a mailbox
// with no host.
func TestMailArrivesAndIsReadableInAClient(t *testing.T) {
	s := newStack(t, "reader")

	if err := s.arrive("someone@example.com", s.address(),
		"From: Someone <someone@example.com>\nTo: "+s.address()+
			"\nSubject: An invoice\nMessage-ID: <inv-1@example.com>\n\nAttached.\n"); err != nil {
		t.Fatal(err)
	}

	c := s.imap()
	if got := joined(c.ok("SELECT INBOX")); !strings.Contains(got, "* 1 EXISTS") {
		t.Fatalf("the message is not in the mailbox:\n%s", got)
	}
	got := joined(c.ok("UID FETCH 1:* (ENVELOPE FLAGS)"))
	for _, want := range []string{"An invoice", `"someone"`, `"example.com"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the envelope is missing %s — a client draws this row:\n%s", want, got)
		}
	}
}

// A plus address is a folder, so one agent's mail can be subscribed to alone.
func TestATaggedAddressBecomesAFolder(t *testing.T) {
	s := newStack(t, "tagged")

	to := s.acc.ID + "+research@" + s.domain
	if err := s.arrive("someone@example.com", to,
		"From: someone@example.com\nTo: "+to+"\nSubject: Three papers\n\nOn retrieval.\n"); err != nil {
		t.Fatal(err)
	}

	c := s.imap()
	folders := joined(c.ok(`LIST "" *`))
	for _, want := range []string{`"INBOX"`, `"INBOX/research"`, `"Junk"`} {
		if !strings.Contains(folders, want) {
			t.Errorf("LIST is missing %s:\n%s", want, folders)
		}
	}
	if got := joined(c.ok("SELECT INBOX/research")); !strings.Contains(got, "* 1 EXISTS") {
		t.Errorf("the tagged folder is empty:\n%s", got)
	}
}

// A client can write to somebody else on this instance, and it lands.
//
// The half of sending that needs no network at all, so it can be asserted
// exactly: submission accepts it, it is filed for the recipient, and their mail
// client can read it with a sender that renders.
func TestAClientCanWriteToSomebodyOnThisInstance(t *testing.T) {
	s := newStack(t, "writer")
	other := newStack(t, "recipient")

	to := other.address()
	err := s.submit(s.address(), to,
		"From: "+s.address()+"\nTo: "+to+"\nSubject: A question\n\nWell?\n")
	if err != nil {
		t.Fatal(err)
	}

	c := other.imap()
	if got := joined(c.ok("SELECT INBOX")); !strings.Contains(got, "* 1 EXISTS") {
		t.Fatalf("the message never reached the recipient:\n%s", got)
	}
	got := joined(c.ok("UID FETCH 1:* (ENVELOPE)"))
	for _, want := range []string{"A question", `"` + s.acc.ID + `"`, `"` + s.domain + `"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the envelope is missing %s:\n%s", want, got)
		}
	}
}

// A reply carries the headers that make it a reply.
//
// Asserted on the message this instance builds rather than on one that reached
// somebody's server, because the last hop requires a relay with a certificate
// the system pool trusts — a property worth keeping and not worth weakening for
// a test. buildExternalTo is the seam that exists for exactly this: "separate
// from sending it so the wire format can be tested".
//
// Gmail threads on References and a client that sees only In-Reply-To files a
// long conversation as a run of unrelated messages, so both have to be there.
func TestAReplyIsAddressedToTheConversation(t *testing.T) {
	s := newStack(t, "threader")

	msg, _ := buildExternalTo("Threader", s.address(), "", "someone@example.com", nil,
		"Re: A question", "Quite.", "", "<q-1@example.com>",
		"<q-0@example.com> <q-1@example.com>")
	out := string(msg)

	for _, want := range []string{
		"In-Reply-To: <q-1@example.com>",
		"References: <q-0@example.com> <q-1@example.com>",
		"Subject: Re: A question",
		"To: someone@example.com",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the outgoing reply is missing %q:\n%s", want, firstLines(out, 20))
		}
	}
}

// Writing to agent@ from a mail client wakes the agent.
//
// It did not, twice over. deliverLocally looked agent@ up as an account, which
// it is not — it resolves to whoever wrote to it — and then mayDispatch refused
// it anyway, because senderKnownTo asks whether the From header really belongs
// to the account, and over submission From is necessarily the instance address
// while a verified address is somebody's external one.
//
// Asserted through the registry rather than by running a model: what broke was
// the dispatch, and a test that needed an API key would not run.
func TestWritingToTheAgentFromAClientWakesIt(t *testing.T) {
	s := newStack(t, "asker")

	woke := make(chan InboundMail, 1)
	restore := onlyHandler(func(m InboundMail) { woke <- m })
	t.Cleanup(restore)

	agentAddr := AgentMailbox + "@" + s.domain
	err := s.submit(s.address(), agentAddr,
		"From: "+s.address()+"\nTo: "+agentAddr+"\nSubject: ping\n\nAre you there?\n")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case m := <-woke:
		if !m.Shared {
			t.Error("the agent was woken but not told this was the shared address, " +
				"so it answers as the wrong agent")
		}
		if m.Owner != s.acc.ID {
			t.Errorf("the run is attributed to %q, want %q — agent@ belongs to "+
				"whoever wrote to it", m.Owner, s.acc.ID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("writing to agent@ from a mail client woke nothing, so the mail was " +
			"filed and never answered")
	}
}

// Mail to your own untagged address is just mail, and must not start a run.
//
// The other half of the rule above. Every newsletter would otherwise be a model
// call, which is the reason mayDispatch has that clause at all — so a test that
// only proved agent@ wakes something would happily pass a change that woke on
// everything.
func TestOrdinaryMailFromAClientWakesNothing(t *testing.T) {
	s := newStack(t, "quiet")

	woke := make(chan InboundMail, 1)
	restore := onlyHandler(func(m InboundMail) { woke <- m })
	t.Cleanup(restore)

	if err := s.submit(s.address(), s.address(),
		"From: "+s.address()+"\nTo: "+s.address()+"\nSubject: a note\n\nTo self.\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case m := <-woke:
		t.Errorf("ordinary mail woke an agent (tag %q, shared %v) — every newsletter "+
			"is now a model call", m.Tag, m.Shared)
	case <-time.After(2 * time.Second):
	}
}

// Neither listener relays for anybody, and the MTA still takes local mail.
//
// The live version of the rule. There was an open relay here: the MTA allowed
// any recipient from localhost, on the reasoning that only a trusted internal
// client could be connecting, with a comment beside it promising an AUTH check
// that never ran. Any process on the host could send mail anywhere, and putting
// a proxy in front would have made that everybody, since every connection then
// arrives from 127.0.0.1 — which is exactly what these tests connect from.
func TestNeitherListenerRelaysForAnybody(t *testing.T) {
	s := newStack(t, "norelay")
	outside := "victim@elsewhere.example.net"

	if err := s.arrive("stranger@evil.example.com", outside, "Subject: x\n\nx\n"); err == nil {
		t.Error("the MTA relayed to an outside address for an unauthenticated sender")
	}
	if s.sink.count() != 0 {
		t.Errorf("%d message(s) reached the relay from an unauthenticated sender", s.sink.count())
	}

	// Unauthenticated submission cannot even start.
	c := dialSMTP(t, s.submission)
	c.do("EHLO client")
	if got := c.do("MAIL FROM:<" + s.address() + ">"); !strings.Contains(got, "530") {
		t.Errorf("submission took MAIL FROM without AUTH: %s", strings.TrimSpace(got))
	}

	// What must still work: mail for a local user, from anywhere.
	if err := s.arrive("someone@example.com", s.address(),
		"From: someone@example.com\nTo: "+s.address()+"\nSubject: hello\n\nhi\n"); err != nil {
		t.Errorf("the MTA refused ordinary inbound mail for a local user: %v", err)
	}
}

// A token authenticates an account, not an address.
//
// Live counterpart to TestSubmissionOnlySendsAsYourself: nothing forged reaches
// the wire, rather than merely being refused at MAIL FROM.
func TestAStolenTokenCannotSendAsAnybodyElse(t *testing.T) {
	s := newStack(t, "honest")

	err := s.submit("agent@"+s.domain, "someone@example.com",
		"From: agent@"+s.domain+"\nTo: someone@example.com\nSubject: trust me\n\nx\n")
	if err == nil {
		t.Fatal("a token was accepted as licence to send from agent@")
	}
	if s.sink.count() != 0 {
		t.Errorf("%d forged message(s) reached the relay", s.sink.count())
	}
}

// ── helpers ─────────────────────────────────────────────────────

// onlyHandler replaces the registered inbound handlers with one, and returns
// the function that puts the real ones back.
//
// Registered under both keys — the shared address and the tagged one — because
// what is being tested is whether anything is woken at all, not which of the
// two lists the dispatcher picked.
func onlyHandler(h InboundHandler) func() {
	inboundMu.Lock()
	saved := inboundHandlers
	inboundHandlers = map[string][]InboundHandler{
		AgentMailbox: {h},
		Tagged:       {h},
	}
	inboundMu.Unlock()
	return func() {
		inboundMu.Lock()
		inboundHandlers = saved
		inboundMu.Unlock()
	}
}

// firstLines is the head of a message, for a readable failure.
func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func allMessages() []*Message {
	mutex.RLock()
	defer mutex.RUnlock()
	out := make([]*Message, len(messages))
	copy(out, messages)
	return out
}

// A quoted reply does not arrive as a screen of nothing.
//
// Every block element becomes a newline, which is right for text and wrong for
// how a mail client builds a message: an empty paragraph is <div><br></div>, so
// one blank line a person left becomes two, and the wrappers Gmail puts round a
// quoted reply turn three into nine. A two-word note read as a screenful of
// white space with a "Hi" at the bottom of it.
//
// The rule is one blank line, never a run: the gap between two paragraphs is
// something the writer meant, and everything past it is markup.
func TestAQuotedReplyIsNotMostlyBlankLines(t *testing.T) {
	// What Gmail sends when you reply: the answer, then the attribution line,
	// then the quote — with empty divs for the blank lines that were there.
	gmail := `<div dir="auto">All good</div><br>` +
		`<div class="gmail_quote gmail_quote_container">` +
		`<div dir="ltr" class="gmail_attr">On Fri, 21 Aug 2026, 07:12 Asim, ` +
		`&lt;asim@micro.mu&gt; wrote:<br></div>` +
		`<blockquote class="gmail_quote">` +
		`<div><div><div><br></div><div><br></div><div><br></div></div></div>` +
		`<div dir="ltr">Hi</div></blockquote></div>`

	out := stripHTMLTags(gmail)

	// Nothing is lost.
	for _, want := range []string{"All good", "wrote:", "Hi"} {
		if !strings.Contains(out, want) {
			t.Errorf("stripping the markup lost %q:\n%q", want, out)
		}
	}

	// And no run of empty lines survives.
	run, worst := 0, 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			if run++; run > worst {
				worst = run
			}
			continue
		}
		run = 0
	}
	if worst > 1 {
		t.Errorf("a quoted reply carries a run of %d blank lines, want at most 1:\n%q",
			worst, out)
	}
}

// A paragraph break is not markup, and survives.
func TestAParagraphBreakSurvives(t *testing.T) {
	out := stripHTMLTags("<p>First.</p><p>Second.</p>")
	if !strings.Contains(out, "First.") || !strings.Contains(out, "Second.") {
		t.Fatalf("lost a paragraph: %q", out)
	}
	if strings.Contains(out, "First.\nSecond.") {
		t.Errorf("two paragraphs were run together, so the writing reads as one "+
			"block: %q", out)
	}
}
