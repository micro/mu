// An IMAP client for a Mu instance, written against somebody else's library.
//
// This is the interop test as a program. Mu's IMAP server is hand-written — no
// dependency, see service/mail/imap.go — and a server that has only ever been
// spoken to by its own tests is a server that agrees with itself. This one
// talks to it through emersion/go-imap, which is what a large part of the Go
// mail world uses and which has no idea Mu exists.
//
// If this works, Thunderbird works. That is the whole point of it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

func main() {
	var (
		addr  = flag.String("addr", "localhost:1143", "IMAP address, host:port")
		user  = flag.String("user", "", "your account name on the instance")
		tls   = flag.Bool("tls", false, "connect with TLS (port 993 behind a terminator)")
		count = flag.Uint("n", 10, "how many of the newest messages to list")
		watch = flag.Bool("watch", false, "wait for new mail with IDLE and print it")
	)
	flag.Parse()

	// The password is an access token, minted at /token on the instance. Mu has
	// no password to give a mail client — sign-in is a passkey or a link — so a
	// token stands in, which is the app-password pattern and revocable on its
	// own.
	pass := os.Getenv("MU_TOKEN")
	if *user == "" || pass == "" {
		log.Fatal("need -user and MU_TOKEN (a personal access token from /token)")
	}

	var (
		c   *client.Client
		err error
	)
	if *tls {
		c, err = client.DialTLS(*addr, nil)
	} else {
		c, err = client.Dial(*addr)
	}
	if err != nil {
		log.Fatalf("could not reach %s: %v", *addr, err)
	}
	defer c.Logout() //nolint:errcheck

	if err := c.Login(*user, pass); err != nil {
		log.Fatalf("login refused: %v", err)
	}
	fmt.Printf("connected to %s as %s\n\n", *addr, *user)

	// Folders. On Mu these are plus-address tags — mail to you+research@ shows
	// up as INBOX/research — plus Junk, where the spam filter's decisions are
	// visible so you can disagree with them.
	folders := make(chan *imap.MailboxInfo, 32)
	go c.List("", "*", folders) //nolint:errcheck
	fmt.Println("folders:")
	for f := range folders {
		fmt.Printf("  %s\n", f.Name)
	}

	// STATUS before SELECT, which is what a real client does: it is the cheap
	// question — how much is in there and how much is new — asked of every
	// folder to decide which are worth opening.
	//
	// It is also the only place the unseen count comes from. SELECT answers
	// with [UNSEEN n], where n is the sequence number of the *first* unseen
	// message rather than how many there are, so reading box.Unseen after a
	// SELECT reports zero however much unread mail is sitting there.
	st, err := c.Status("INBOX", []imap.StatusItem{imap.StatusMessages, imap.StatusUnseen})
	if err != nil {
		log.Fatalf("STATUS failed: %v", err)
	}

	box, err := c.Select("INBOX", false)
	if err != nil {
		log.Fatalf("could not open INBOX: %v", err)
	}
	fmt.Printf("\nINBOX: %d messages, %d unseen, uidvalidity %d, uidnext %d\n\n",
		box.Messages, st.Unseen, box.UidValidity, box.UidNext)

	if box.Messages > 0 {
		from := uint32(1)
		if box.Messages > uint32(*count) {
			from = box.Messages - uint32(*count) + 1
		}
		seq := new(imap.SeqSet)
		seq.AddRange(from, box.Messages)

		msgs := make(chan *imap.Message, *count)
		done := make(chan error, 1)
		go func() {
			done <- c.Fetch(seq, []imap.FetchItem{imap.FetchEnvelope, imap.FetchFlags, imap.FetchUid}, msgs)
		}()
		for m := range msgs {
			fmt.Printf("  uid %-6d %s  %-28s %s\n",
				m.Uid, seen(m.Flags), sender(m), subject(m))
		}
		if err := <-done; err != nil {
			log.Fatalf("fetch failed: %v", err)
		}
	}

	if !*watch {
		return
	}

	// IDLE is what makes a mail client feel like a mail client: the server says
	// when something arrives rather than being asked every minute.
	fmt.Println("\nwaiting for new mail (ctrl-c to stop)…")
	updates := make(chan client.Update, 8)
	c.Updates = updates
	stop := make(chan struct{})
	idled := make(chan error, 1)
	go func() { idled <- c.Idle(stop, nil) }()
	for {
		select {
		case u := <-updates:
			if mu, ok := u.(*client.MailboxUpdate); ok {
				fmt.Printf("  now %d messages\n", mu.Mailbox.Messages)
			}
		case err := <-idled:
			if err != nil {
				log.Fatalf("idle stopped: %v", err)
			}
			return
		case <-time.After(30 * time.Minute):
			close(stop)
		}
	}
}

func seen(flags []string) string {
	for _, f := range flags {
		if f == imap.SeenFlag {
			return " "
		}
	}
	return "•"
}

func sender(m *imap.Message) string {
	if m.Envelope == nil || len(m.Envelope.From) == 0 {
		return ""
	}
	a := m.Envelope.From[0]
	return trim(a.Address(), 28)
}

func subject(m *imap.Message) string {
	if m.Envelope == nil {
		return ""
	}
	return trim(strings.TrimSpace(m.Envelope.Subject), 60)
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
