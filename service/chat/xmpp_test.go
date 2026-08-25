package chat

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
)

// A real client can connect, authenticate, bind and send a message.
//
// Driven over a socket with the bytes a client actually sends, because an XMPP
// server that compiles is not an XMPP server. The handshake is where this either
// works with Conversations and Dino or does not.
func TestAClientCanConnectAndSpeak(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "xmppuser")

	client := dial(t)
	defer client.Close()

	// Stream header, and the features an unauthenticated client may use.
	client.write(`<?xml version='1.0'?><stream:stream to='example.test' ` +
		`xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' version='1.0'>`)
	got := client.until(t, "</stream:features>")
	if !strings.Contains(got, "<stream:stream") {
		t.Fatalf("no stream header back: %q", clip(got))
	}
	if !strings.Contains(got, "PLAIN") {
		t.Fatalf("SASL PLAIN was not offered, so no client can log in: %q", clip(got))
	}
	// An unauthenticated client must not be told what an authenticated one can
	// do.
	if strings.Contains(got, "xmpp-bind") {
		t.Error("bind is offered before authentication")
	}

	// SASL PLAIN: authzid\x00authcid\x00password, and the password is an
	// access token — the same credential IMAP and submission take.
	payload := base64.StdEncoding.EncodeToString(
		[]byte("\x00" + acc.ID + "\x00" + token))
	client.write(`<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>` +
		payload + `</auth>`)
	if got := client.read(t); !strings.Contains(got, "<success") {
		t.Fatalf("sign-in refused with a valid token: %q", clip(got))
	}

	// The RFC requires a new stream after SASL.
	client.write(`<?xml version='1.0'?><stream:stream to='example.test' ` +
		`xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' version='1.0'>`)
	got = client.until(t, "</stream:features>")
	if !strings.Contains(got, "xmpp-bind") {
		t.Fatalf("bind was not offered after authentication: %q", clip(got))
	}

	// Bind a resource, which is what makes this connection addressable.
	client.write(`<iq type='set' id='bind1'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'>` +
		`<resource>phone</resource></bind></iq>`)
	got = client.read(t)
	if !strings.Contains(got, "xmppuser@example.test/phone") {
		t.Fatalf("bind did not return the full JID: %q", clip(got))
	}

	// And the account is now online, which is the thing the websocket rooms
	// never had.
	if !Online("xmppuser") {
		t.Error("a bound session does not count as online")
	}

	// The roster leads with the agent, because that is the reason to connect.
	client.write(`<iq type='get' id='r1'><query xmlns='jabber:iq:roster'/></iq>`)
	got = client.read(t)
	if !strings.Contains(got, "agent@example.test") {
		t.Errorf("the agent is not in the roster, so a client shows nobody to "+
			"talk to: %q", clip(got))
	}
}

// A message to a stranger is refused rather than silently dropped.
//
// Federation is not built. A stanza error is what a client renders as "not
// delivered"; silence is what it renders as delivered, which is the worse
// failure of the two.
func TestAMessageOffTheInstanceIsRefused(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	acc, token := accountWithToken(t, "xmppsender")

	client := dial(t)
	defer client.Close()
	client.handshake(t, acc.ID, token)

	client.write(`<message type='chat' to='somebody@elsewhere.example'>` +
		`<body>hello</body></message>`)
	got := client.read(t)
	if !strings.Contains(got, "remote-server-not-found") {
		t.Errorf("a message to another server was accepted and went nowhere: %q", clip(got))
	}
}

// Two accounts here can talk to each other.
func TestTwoAccountsHereCanTalk(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "xmppalice")
	b, btok := accountWithToken(t, "xmppbob")

	alice := dial(t)
	defer alice.Close()
	alice.handshake(t, a.ID, atok)

	bob := dial(t)
	defer bob.Close()
	bob.handshake(t, b.ID, btok)

	alice.write(`<message type='chat' to='xmppbob@example.test'><body>are you there</body></message>`)
	got := bob.read(t)
	if !strings.Contains(got, "are you there") {
		t.Fatalf("the message did not arrive: %q", clip(got))
	}
	if !strings.Contains(got, "xmppalice@example.test") {
		t.Errorf("the message does not say who it is from: %q", clip(got))
	}
}

// Somebody who is not connected still gets the message.
//
// Being offline is the ordinary case for chat, and it used to come back to the
// sender as remote-server-not-found — a permanent failure, for a temporary
// condition that is not a failure. What makes accepting it honest is that it
// lands in the recipient's record, so it is at /inbox when they look.
func TestAMessageToSomebodyOfflineIsKeptRatherThanRefused(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "xmppwriter")
	accountWithToken(t, "xmppsleeper")

	alice := dial(t)
	defer alice.Close()
	alice.handshake(t, a.ID, atok)

	alice.write(`<message type='chat' to='xmppsleeper@example.test'>` +
		`<body>call me when you wake up</body></message>`)
	if got := alice.silence(t); got != "" {
		t.Fatalf("writing to somebody offline came back with %q", clip(got))
	}

	// Both ends, because the record is account-scoped: the sender's copy is
	// what they sent, the recipient's is what arrived, and neither can read the
	// other's.
	for _, who := range []string{"xmppwriter", "xmppsleeper"} {
		th := thread.Find(who, thread.ChatClient, xmppRoom(
			"xmppwriter@example.test", "xmppsleeper@example.test"))
		if th == nil {
			t.Fatalf("%s has no record of the conversation, so an offline "+
				"message is simply lost", who)
		}
		msgs := thread.Messages(who, th.ID, 10)
		if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "call me") {
			t.Errorf("%s's record holds %d messages, want the one that was sent: %+v",
				who, len(msgs), msgs)
		}
	}
}

// Delivering to several resources records the message once.
//
// A phone and a laptop are two places to deliver to, not two things that
// happened.
func TestOneMessageIsRecordedOnce(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "xmppsingle")
	b, btok := accountWithToken(t, "xmppmulti")

	alice := dial(t)
	defer alice.Close()
	alice.handshake(t, a.ID, atok)

	for _, res := range []string{"phone", "laptop"} {
		c := dial(t)
		defer c.Close()
		c.handshakeAs(t, b.ID, btok, res)
	}

	alice.write(`<message type='chat' to='xmppmulti@example.test'><body>once</body></message>`)
	time.Sleep(200 * time.Millisecond)

	th := thread.Find("xmppmulti", thread.ChatClient,
		xmppRoom("xmppsingle@example.test", "xmppmulti@example.test"))
	if th == nil {
		t.Fatal("nothing was recorded")
	}
	if n := len(thread.Messages("xmppmulti", th.ID, 10)); n != 1 {
		t.Errorf("one message reached %d resources and was recorded %d times", 2, n)
	}
}

// An address here that is nobody says so, and does not claim the server is gone.
//
// A typo and an unreachable domain are different problems with different fixes,
// and a client shows the sender whichever one it is told.
func TestAnAddressHereThatIsNobodySaysSo(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "xmpptypist")

	alice := dial(t)
	defer alice.Close()
	alice.handshake(t, a.ID, atok)

	alice.write(`<message type='chat' to='nosuchperson@example.test'><body>hello</body></message>`)
	got := alice.read(t)
	if !strings.Contains(got, "item-not-found") {
		t.Errorf("writing to a name that is nobody here reported %q, which does "+
			"not tell the sender the address is wrong", clip(got))
	}
}

// A display name with a quote in it does not end the attribute it is in.
//
// The same class of bug as the mail header injection this repository fixed, in
// a different syntax: this server writes stanzas as strings rather than
// marshalling them, so every value going into one has to be escaped.
func TestAValueCannotEndTheStanzaItIsIn(t *testing.T) {
	for _, in := range []string{
		`Bob' from='attacker@evil.test`,
		`<body>injected</body>`,
		"line\nbreak",
	} {
		if got := xmlAttr(in); strings.ContainsAny(got, "'<>") {
			t.Errorf("xmlAttr(%q) = %q — still holds markup", in, got)
		}
		if got := xmlText(in); strings.Contains(got, "<") {
			t.Errorf("xmlText(%q) = %q — still holds an element", in, got)
		}
	}
}

// An agent address is recognised in both of the shapes mail uses.
func TestAnAgentIsAddressableTheWayMailAddressesIt(t *testing.T) {
	for _, local := range []string{"agent", "asim+research", "asim+news"} {
		if !agentAddressed(local) {
			t.Errorf("%s@ is not recognised as an agent, so a message to it is "+
				"delivered as ordinary chat and nothing answers", local)
		}
	}
	for _, local := range []string{"asim", "bob", ""} {
		if agentAddressed(local) {
			t.Errorf("%s@ was treated as an agent, so writing to a person wakes "+
				"one", local)
		}
	}
}

// One conversation whichever way the message went.
func TestAConversationIsTheSameFromBothEnds(t *testing.T) {
	a := xmppRoom("asim@example.test", "agent@example.test")
	b := xmppRoom("agent@example.test", "asim@example.test")
	if a != b {
		t.Errorf("%q and %q are different conversations, so a reply starts a new one", a, b)
	}
	// And it cannot collide with a websocket room id.
	if !strings.HasPrefix(a, "xmpp_") {
		t.Errorf("%q shares a namespace with the websocket rooms", a)
	}
}

// ── harness ────────────────────────────────────────────────────────────

type conn struct {
	net.Conn
	buf []byte
}

func dial(t *testing.T) *conn {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		serveXMPP(c)
	}()
	t.Cleanup(func() { ln.Close() })

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return &conn{Conn: c, buf: make([]byte, 8192)}
}

func (c *conn) write(s string) { fmt.Fprint(c.Conn, s) }

// silence reads whatever comes back in a moment, and is content with nothing.
//
// read fails a test that gets no reply, which is right everywhere a reply is
// the point and wrong for asserting a message was accepted: acceptance in XMPP
// is the absence of an error stanza.
func (c *conn) silence(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for {
		n, err := c.Read(c.buf)
		b.Write(c.buf[:n])
		if err != nil {
			return strings.TrimSpace(b.String())
		}
	}
}

func (c *conn) read(t *testing.T) string {
	t.Helper()
	return c.until(t, "")
}

// until reads until the reply contains want, or a moment passes.
//
// A stanza is not a packet. The server writes a stream header and its features
// as two writes, and a client that assumed one read per reply would see half a
// handshake — which is what a real client gets right by parsing the stream and
// what a test has to do deliberately.
func (c *conn) until(t *testing.T, want string) string {
	t.Helper()
	var got strings.Builder
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, err := c.Read(c.buf)
		if n > 0 {
			got.Write(c.buf[:n])
			if want == "" || strings.Contains(got.String(), want) {
				return got.String()
			}
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() && got.Len() > 0 {
				return got.String()
			}
			if got.Len() > 0 {
				return got.String()
			}
			t.Fatalf("read: %v", err)
		}
	}
	return got.String()
}

// handshake runs stream, SASL, restart and bind, for tests about what happens
// afterwards.
func (c *conn) handshake(t *testing.T, id, token string) {
	c.handshakeAs(t, id, token, "")
}

// handshakeAs is the same, binding a named resource — a phone and a laptop are
// two connections for one account.
func (c *conn) handshakeAs(t *testing.T, id, token, resource string) {
	t.Helper()
	open := `<?xml version='1.0'?><stream:stream to='example.test' ` +
		`xmlns='jabber:client' xmlns:stream='http://etherx.jabber.org/streams' version='1.0'>`
	c.write(open)
	c.until(t, "</stream:features>")
	c.write(`<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>` +
		base64.StdEncoding.EncodeToString([]byte("\x00"+id+"\x00"+token)) + `</auth>`)
	c.read(t)
	c.write(open)
	c.until(t, "</stream:features>")
	bind := `<iq type='set' id='b'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'/></iq>`
	if resource != "" {
		bind = `<iq type='set' id='b'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'>` +
			`<resource>` + resource + `</resource></bind></iq>`
	}
	c.write(bind)
	c.read(t)
}

func accountWithToken(t *testing.T, id string) (*auth.Account, string) {
	t.Helper()
	acc, err := auth.GetAccount(id)
	if err != nil || acc == nil {
		acc = &auth.Account{ID: id, Name: id, Created: time.Now()}
		if err := auth.Create(acc); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	// The record outlives a test run — it is on disk — so a test that counts
	// messages counts the last run's too. Cleared at both ends: after, so a run
	// leaves nothing behind, and before, because a run that failed halfway
	// never got to its cleanup.
	thread.Forget(acc.ID)
	t.Cleanup(func() { thread.Forget(acc.ID) })
	_, token, err := auth.CreateToken(acc.ID, "xmpp test", nil, time.Time{})
	if err != nil {
		t.Fatalf("token for %s: %v", id, err)
	}
	return acc, token
}

func clip(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// A note to self is recorded once, not twice.
//
// Sending to your own address is ordinary — it is how somebody moves a link
// between their phone and their laptop, and it is what a client does when it
// syncs. Both ends of the exchange are the same account, so a record that
// writes "both sides" without noticing files it twice in one conversation and
// /inbox shows it duplicated with nothing wrong upstream.
func TestANoteToSelfIsRecordedOnce(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	a, atok := accountWithToken(t, "xmppself")

	me := dial(t)
	defer me.Close()
	me.handshake(t, a.ID, atok)

	me.write(`<message type='chat' to='xmppself@example.test'><body>note to self</body></message>`)
	if got := me.read(t); !strings.Contains(got, "note to self") {
		t.Fatalf("a message to my own address did not come back to me: %q", clip(got))
	}

	th := thread.Find("xmppself", thread.ChatClient,
		xmppRoom("xmppself@example.test", "xmppself@example.test"))
	if th == nil {
		t.Fatal("a note to self was not recorded at all")
	}
	if n := len(thread.Messages("xmppself", th.ID, 10)); n != 1 {
		t.Errorf("one note to self was recorded %d times", n)
	}
}
