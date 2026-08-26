// Send one message from a Mu instance to a JID on somebody else's server.
//
// This exists because outbound federation has exactly one door. A stanza
// addressed off-domain arriving on the C2S port is the only thing that reaches
// SendRemote — chat_send posts to rooms, not JIDs — so there is no curl that
// exercises it and no test that can, short of standing up a second XMPP server.
//
// What it proves, in order, and it stops at the first one that fails:
//
//	the C2S port is reachable and terminates TLS
//	the token authenticates over SASL PLAIN
//	the instance resolves the remote domain and completes dialback with it
//	the remote server accepts the message
//
// Only the third is federation. The first two fail loudly and locally; the
// third fails as <message type='error'> with remote-server-not-found, which is
// what this prints and the reason it waits around after sending rather than
// exiting on a successful write.
//
// Deliberately no XMPP library. The thing under test is a handshake between two
// servers, and a client that speaks the four stanzas it needs by hand has
// nothing in it that could be doing the work instead. examples/imap-client
// makes the opposite choice for the opposite reason: there the client *is* the
// test.
package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		addr   = flag.String("addr", "micro.mu:5223", "the C2S port, host:port, direct TLS")
		domain = flag.String("domain", "", "the instance's XMPP domain (default: the host in -addr)")
		user   = flag.String("user", "", "your account name on the instance")
		to     = flag.String("to", "", "a JID on another server, e.g. someone@jabber.org")
		text   = flag.String("text", "Hello from the other side.", "what to say")
		wait   = flag.Duration("wait", 30*time.Second, "how long to listen for an error after sending")
	)
	flag.Parse()

	// A personal access token, the same credential IMAP and submission take.
	pass := os.Getenv("MU_TOKEN")
	if *user == "" || *to == "" || pass == "" {
		die("need -user and -to, and MU_TOKEN (a personal access token from /token)")
	}
	host := strings.Split(*addr, ":")[0]
	if *domain == "" {
		*domain = host
	}

	conn, err := tls.Dial("tcp", *addr, &tls.Config{ServerName: host})
	if err != nil {
		die("connecting to %s: %v\n\nA timeout here is the firewall, not the server. "+
			"Connection refused is the server.", *addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Minute))
	fmt.Printf("connected to %s\n", *addr)

	dec := openStream(conn, *domain)
	skipFeatures(dec)

	auth := base64.StdEncoding.EncodeToString([]byte("\x00" + *user + "\x00" + pass))
	send(conn, `<auth xmlns='urn:ietf:params:xml:ns:xmpp-sasl' mechanism='PLAIN'>%s</auth>`, auth)
	st, err := next(dec)
	if err != nil {
		die("reading the SASL reply: %v", err)
	}
	if st.Name.Local != "success" {
		die("the instance refused the token: <%s>", st.Name.Local)
	}
	fmt.Printf("authenticated as %s@%s\n", *user, *domain)

	// The stream restarts after SASL, per RFC 6120 §6.4.6.
	dec = openStream(conn, *domain)
	skipFeatures(dec)

	send(conn, `<iq type='set' id='bind'><bind xmlns='urn:ietf:params:xml:ns:xmpp-bind'>`+
		`<resource>federate</resource></bind></iq>`)
	if _, err := next(dec); err != nil {
		die("binding a resource: %v", err)
	}
	_ = dec.Skip()

	fmt.Printf("sending to %s\n", *to)
	send(conn, `<message to='%s' type='chat'><body>%s</body></message>`,
		escape(*to), escape(*text))

	// Silence is success, which is why this waits. The instance answers the
	// stanza only when it could not deliver it, and dialback against a domain
	// that is slow to answer can take most of the ten-second dial timeout.
	_ = conn.SetDeadline(time.Now().Add(*wait + 5*time.Second))
	deadline := time.Now().Add(*wait)
	quiet := true
	for time.Now().Before(deadline) {
		st, err := next(dec)
		if err != nil {
			break
		}
		var stanza struct {
			Type  string `xml:"type,attr"`
			From  string `xml:"from,attr"`
			Inner string `xml:",innerxml"`
		}
		if err := dec.DecodeElement(&stanza, &st); err != nil {
			break
		}
		quiet = false
		fmt.Printf("\n<- <%s type=%q from=%q>\n   %s\n", st.Name.Local, stanza.Type, stanza.From, stanza.Inner)
		if st.Name.Local == "message" && stanza.Type == "error" {
			fmt.Println("\nremote-server-not-found means the dialback handshake did not complete.")
			fmt.Println("The instance's log under `chat` says which half: an outbound dial that")
			fmt.Println("failed, or a verification call that came back invalid.")
			os.Exit(1)
		}
	}
	if quiet {
		fmt.Println("\nno error came back: dialback completed and the remote server took the message.")
		fmt.Println("Whether it reached a person is between them and their user — check the other end.")
	}
}

// openStream sends a stream header and reads theirs.
func openStream(conn io.ReadWriter, domain string) *xml.Decoder {
	send(conn, `<?xml version='1.0'?><stream:stream xmlns='jabber:client' `+
		`xmlns:stream='http://etherx.jabber.org/streams' to='%s' version='1.0'>`, escape(domain))
	dec := xml.NewDecoder(conn)
	st, err := next(dec)
	if err != nil {
		die("opening a stream: %v", err)
	}
	if st.Name.Local != "stream" {
		die("expected a stream header, got <%s>", st.Name.Local)
	}
	return dec
}

// skipFeatures reads past <stream:features>, whatever is in it.
func skipFeatures(dec *xml.Decoder) {
	st, err := next(dec)
	if err != nil {
		die("reading stream features: %v", err)
	}
	if st.Name.Local == "features" {
		_ = dec.Skip()
	}
}

func send(w io.Writer, format string, a ...interface{}) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		die("writing: %v", err)
	}
}

// next is the next start element, ignoring character data and end elements.
func next(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		t, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if st, ok := t.(xml.StartElement); ok {
			return st, nil
		}
	}
}

// escape makes a string safe inside an attribute or an element.
func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func die(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
