package mail

// Submission, over a real socket.
//
// Same reason imap_test.go uses one: what breaks in an SMTP server is the
// conversation — a code a client reads as fatal when it was meant to retry, an
// AUTH mechanism advertised and not implemented — and none of that is visible
// from inside the handlers.

import (
	"bufio"
	"encoding/base64"
	"net"
	netmail "net/mail"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"

	smtpd "github.com/emersion/go-smtp"
)

// submissionServer starts one on a random port and returns its address.
func submissionServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := smtpd.NewServer(&submissionBackend{})
	s.Domain = ConfiguredDomain()
	s.AllowInsecureAuth = true
	s.MaxRecipients = submissionMaxRecipients
	go s.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { s.Close() })
	return ln.Addr().String()
}

// smtpConn is a hand-driven client, so a test can send a command a real client
// would refuse to send.
type smtpConn struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

func dialSMTP(t *testing.T, addr string) *smtpConn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &smtpConn{t: t, conn: conn, r: bufio.NewReader(conn)}
	c.read() // greeting
	return c
}

// read collects one whole reply.
//
// SMTP continues a reply with "250-" and ends it with "250 ", and EHLO always
// spans several lines. Reading one bufferful gave back part of the greeting and
// left the rest to be mistaken for the answer to the next command, which is why
// this looked like the server rejecting valid credentials.
func (c *smtpConn) read() string {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	var out strings.Builder
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			c.t.Fatalf("read: %v (got %q)", err, out.String())
		}
		out.WriteString(line)
		if len(line) >= 4 && line[3] == ' ' {
			return out.String()
		}
	}
}

func (c *smtpConn) do(line string) string {
	c.t.Helper()
	if _, err := c.conn.Write([]byte(line + "\r\n")); err != nil {
		c.t.Fatalf("write: %v", err)
	}
	return c.read()
}

// account makes one with a token, reusing it across runs in this package.
func submissionAccount(t *testing.T, id, display string) (*auth.Account, string) {
	t.Helper()
	acc := &auth.Account{ID: id, Name: display, Created: time.Now()}
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
	return acc, token
}

func plainAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte("\x00" + user + "\x00" + pass))
}

// AUTH PLAIN takes a username and an access token, and nothing else.
//
// The display name is the case that matters. It is free text and not unique,
// so it must never sign anybody in — the same rule credentials.go exists to
// state once, after IMAP got it wrong in the other direction and refused
// everybody whose display name differed from their username.
func TestSubmissionSignsYouInByUsernameAndToken(t *testing.T) {
	acc, token := submissionAccount(t, "submituser", "Submit User")
	addr := submissionServer(t)

	for _, tc := range []struct {
		what, user, pass string
		want             bool
	}{
		{"the username and token", acc.ID, token, true},
		{"the full address", acc.ID + "@" + ConfiguredDomain(), token, true},
		{"the display name", acc.Name, token, false},
		{"a wrong token", acc.ID, "not-a-token", false},
		{"no token", acc.ID, "", false},
	} {
		c := dialSMTP(t, addr)
		c.do("EHLO test")
		got := c.do("AUTH PLAIN " + plainAuth(tc.user, tc.pass))
		ok := strings.Contains(got, "235")
		if ok != tc.want {
			t.Errorf("AUTH with %s: %s", tc.what, strings.TrimSpace(got))
		}
	}
}

// AUTH is advertised, because a client decides whether it can send from EHLO.
//
// The server this replaced had a Login method that satisfied no interface go-smtp
// has called in years, so AUTH was neither advertised nor possible and every
// mail client reported the account as unable to send.
func TestSubmissionOffersAuth(t *testing.T) {
	c := dialSMTP(t, submissionServer(t))
	greeting := c.do("EHLO test")
	if !strings.Contains(greeting, "AUTH") || !strings.Contains(strings.ToUpper(greeting), "PLAIN") {
		t.Errorf("EHLO does not offer AUTH PLAIN:\n%s", greeting)
	}
}

// Nothing happens before AUTH.
//
// An open submission server is an open relay, which is the one mistake in this
// area that ends with the domain on a blocklist.
func TestSubmissionRefusesEverythingBeforeAuth(t *testing.T) {
	c := dialSMTP(t, submissionServer(t))
	c.do("EHLO test")

	// MAIL FROM is where an unauthenticated sender is turned away, and 530 is
	// the specific code, because that is what makes a mail client ask for a
	// password rather than show an error.
	got := c.do("MAIL FROM:<anyone@" + ConfiguredDomain() + ">")
	if !strings.Contains(got, "530") {
		t.Errorf("MAIL FROM before AUTH answered %q, want 530", strings.TrimSpace(got))
	}

	// The rest of the sequence cannot be reached at all. go-smtp enforces the
	// ordering itself, so these are refused before the session sees them — the
	// property is that no envelope can be completed, not which layer says so.
	for _, cmd := range []string{"RCPT TO:<someone@example.com>", "DATA"} {
		got := c.do(cmd)
		if code := strings.TrimSpace(got); !strings.HasPrefix(code, "5") {
			t.Errorf("%q before AUTH answered %q, want a refusal", cmd, code)
		}
	}
}

// A token is not a licence to forge.
//
// It authenticates an account and says nothing about which address may go in
// MAIL FROM. Without this check anybody holding one could send as any address
// on the domain, including the ones that carry password resets and sign-in
// links — so the interesting case is not a stranger's address but another
// local user's.
func TestSubmissionOnlySendsAsYourself(t *testing.T) {
	acc, token := submissionAccount(t, "forgeme", "Forge Me")
	submissionAccount(t, "victim", "Victim")
	domain := ConfiguredDomain()
	addr := submissionServer(t)

	for _, tc := range []struct {
		what, from string
		want       bool
	}{
		{"your own address", acc.ID + "@" + domain, true},
		{"a plus alias of it", acc.ID + "+research@" + domain, true},
		{"another local account", "victim@" + domain, false},
		{"an address elsewhere", "someone@example.com", false},
		{"a reserved local address", "agent@" + domain, false},
	} {
		c := dialSMTP(t, addr)
		c.do("EHLO test")
		if got := c.do("AUTH PLAIN " + plainAuth(acc.ID, token)); !strings.Contains(got, "235") {
			t.Fatalf("could not authenticate: %s", got)
		}
		got := c.do("MAIL FROM:<" + tc.from + ">")
		if ok := strings.Contains(got, "250"); ok != tc.want {
			t.Errorf("MAIL FROM %s (%s): %s", tc.what, tc.from, strings.TrimSpace(got))
		}
	}
}

// ownsAddress is the check underneath, and case is not identity.
func TestOwnsAddress(t *testing.T) {
	acc := &auth.Account{ID: "asim", Name: "Someone Else"}
	domain := ConfiguredDomain()
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"asim@" + domain, true},
		{"ASIM@" + strings.ToUpper(domain), true},
		{"asim+research@" + domain, true},
		{"<asim@" + domain + ">", true},
		{"asimov@" + domain, false},
		{"someone else@" + domain, false},
		{"asim@evil.example.com", false},
		{"asim", false},
		{"", false},
	} {
		if got := ownsAddress(acc, tc.addr); got != tc.want {
			t.Errorf("ownsAddress(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
	if ownsAddress(nil, "asim@"+domain) {
		t.Error("a nil account owns an address")
	}
}

// The text and the HTML both survive.
//
// A mail client sends multipart/alternative, and taking only the text part
// would quietly drop every link the person wrote. multipart/mixed wrapping
// multipart/alternative is what happens the moment there is an attachment, so
// the body is two levels down in an ordinary message.
func TestSubmissionReadsWhatAClientActuallySends(t *testing.T) {
	raw := "Subject: Hello\r\n" +
		"Content-Type: multipart/mixed; boundary=OUTER\r\n" +
		"\r\n" +
		"--OUTER\r\n" +
		"Content-Type: multipart/alternative; boundary=INNER\r\n" +
		"\r\n" +
		"--INNER\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"it=E2=80=99s here\r\n" +
		"--INNER\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>it&rsquo;s <b>here</b></p>\r\n" +
		"--INNER--\r\n" +
		"--OUTER--\r\n"

	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	plain, html := submissionBody(msg)

	// quoted-printable decoded, or an apostrophe arrives as =E2=80=99.
	if !strings.Contains(plain, "it’s here") {
		t.Errorf("the text part did not survive: %q", plain)
	}
	if !strings.Contains(html, "<b>here</b>") {
		t.Errorf("the HTML part did not survive: %q", html)
	}
}

// A plain message with no MIME at all is still a message.
func TestSubmissionReadsAPlainMessage(t *testing.T) {
	msg, err := netmail.ReadMessage(strings.NewReader("Subject: Hello\r\n\r\njust text\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	plain, html := submissionBody(msg)
	if strings.TrimSpace(plain) != "just text" {
		t.Errorf("plain body is %q", plain)
	}
	if html != "" {
		t.Errorf("invented an HTML part: %q", html)
	}
}

// Every mail this instance sends leaves through outbound.SendOut/ReplyOut,
// which is where the gate and the charge are. A submission server that called
// the transport itself would be a second exit with neither.
//
// Checked by reading the source, because the alternative is a test that mocks
// the network and proves nothing about which function was called.
func TestSubmissionSendsThroughTheOneDoor(t *testing.T) {
	src := readSource(t, "submission.go")
	if !strings.Contains(src, "ReplyOut(") {
		t.Error("submission does not send through ReplyOut, so it has neither " +
			"MaySendOut's gate nor the charge — see outbound.go")
	}
	for _, banned := range []string{"smtp.SendMail", "SendExternalReply(", "relayViaSubmission("} {
		if strings.Contains(src, banned) {
			t.Errorf("submission calls %s directly, which is a second way out of "+
				"this instance and skips the gate in outbound.go", banned)
		}
	}
}

// A token that has been revoked cannot send.
func TestSubmissionRefusesARevokedToken(t *testing.T) {
	acc, token := submissionAccount(t, "revoked", "Revoked")
	id, err := auth.ValidatePATToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteToken(id, acc.ID); err != nil {
		t.Skipf("cannot revoke in this build: %v", err)
	}

	c := dialSMTP(t, submissionServer(t))
	c.do("EHLO test")
	if got := c.do("AUTH PLAIN " + plainAuth(acc.ID, token)); strings.Contains(got, "235") {
		t.Errorf("a revoked token still signs in: %s", strings.TrimSpace(got))
	}
}

// A locally delivered message has a sender a mail client can draw.
//
// The agent's replies are filed as From "Mu", FromID "agent@…" — a display
// name in one field and the address in the other — and imapEnvelope took the
// first as the address. So it sent a mailbox of "Mu" with no host, and every
// client drew a blank sender on exactly the mail this product is about.
func TestTheEnvelopeNamesASenderAClientCanDraw(t *testing.T) {
	domain := ConfiguredDomain()
	for _, tc := range []struct {
		what string
		m    *Message
		want []string
	}{
		{
			"the agent, delivered locally",
			&Message{From: "Mu", FromID: "agent@" + domain, To: "asim@" + domain},
			[]string{`"Mu"`, `"agent"`, `"` + domain + `"`},
		},
		{
			"a local account id with no domain on it",
			&Message{From: "IMAP Test", FromID: "imaptest", To: "asim@" + domain},
			[]string{`"IMAP Test"`, `"imaptest"`, `"` + domain + `"`},
		},
		{
			"a stranger over SMTP, no display name",
			&Message{From: "sender@example.com", FromID: "sender@example.com"},
			[]string{`"sender"`, `"example.com"`},
		},
	} {
		got := imapEnvelope(tc.m)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s: envelope is missing %s:\n%s", tc.what, want, got)
			}
		}
		if strings.Contains(got, `"Mu" NIL "Mu"`) {
			t.Errorf("%s: the display name is being used as the address:\n%s", tc.what, got)
		}
	}
}

// Neither listener relays for a stranger.
//
// The property, stated where it cannot rot. There was an open relay here: the
// MTA allowed any recipient from localhost, on the reasoning that a trusted
// internal client was the only thing that could be connecting, and the comment
// beside it said "but still require SMTP AUTH to prevent abuse" — which never
// happened, because AUTH on that listener has never worked. So anything that
// could open a socket on the host could send mail anywhere, and putting any
// proxy in front would have made that the whole internet, since every
// connection then arrives from 127.0.0.1.
//
// The rule now has no exception in it: port 25 accepts mail for local users
// and relays for nobody, and sending is submission's job, where the sender
// authenticates first.
func TestNeitherListenerRelaysForAStranger(t *testing.T) {
	src := readSource(t, "smtp.go")

	// The exemption is gone, and the localhost test that granted it with it.
	for _, banned := range []string{
		"if s.isLocalhost {\n\t\ts.to = append(s.to, to)",
		"isExternal && s.isLocalhost",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("the MTA still relays for localhost:\n\t%s\nAnything on the host "+
				"can send mail anywhere, and a proxy in front makes that everybody.", banned)
		}
	}

	// And the rule it was standing on top of is still there.
	if !strings.Contains(src, "Relay access denied") {
		t.Error("the MTA no longer refuses external recipients at all")
	}
}

// Sending from a mail client leaves a record.
//
// ReplyOut sends and does not record — every other caller files its own copy
// afterwards. This one did not, so mail sent from Thunderbird existed nowhere
// on the instance: not in /inbox, not in the Sent view, and not in what the
// agent can see, so it had no idea what you had already said. IMAP has no
// APPEND either, so the client cannot file the copy itself the way it would
// against any other server, which makes this the only place it can come from.
func TestSubmissionRecordsWhatItSent(t *testing.T) {
	src := readSource(t, "submission.go")
	i := strings.Index(src, "ReplyOut(")
	if i < 0 {
		t.Fatal("submission no longer sends through ReplyOut")
	}
	if !strings.Contains(src[i:], "SendMessage(") {
		t.Error("nothing files a copy after ReplyOut, so mail sent from a client " +
			"leaves no trace on the instance")
	}
}

// Writing to agent@ from a mail client reaches the agent.
//
// agent@ is not an account — it is a reserved name that resolves to whoever
// wrote to it — so looking it up as one refuses it. smtp.go's Rcpt already
// carries a comment about that exact mistake: "the account lookup below
// refuses them... which is how agent@ was unreachable while the code answering
// it sat there working." This reproduced it, so the mail filed and nothing
// woke.
func TestSubmissionWakesTheAgent(t *testing.T) {
	src := readSource(t, "submission.go")
	for _, want := range []string{"AgentMailbox", "deliverInbound("} {
		if !strings.Contains(src, want) {
			t.Errorf("submission does not mention %s, so mail sent to the agent from "+
				"a mail client is filed and never answered", want)
		}
	}
}
