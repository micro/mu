package mail

// IMAP, end to end: a client connects, logs in, and reads its mail.
//
// Driven over a real socket rather than by calling the handlers, because the
// thing that breaks in an IMAP server is the wire format — a missing space, a
// literal whose count is wrong, a response sent before its continuation — and
// none of that is visible from inside.

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// client is a connection to a server started for this test.
type client struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
	n    int
}

func dial(t *testing.T) *client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveIMAP(conn)
		}
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &client{t: t, conn: conn, r: bufio.NewReader(conn)}
	if greeting := c.line(); !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("greeting was %q", greeting)
	}
	return c
}

func (c *client) line() string {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	line, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("nothing came back: %v", err)
	}
	return strings.TrimRight(line, "\r\n")
}

// do sends a command and reads until its tagged response. It returns everything
// including that line.
func (c *client) do(cmd string) []string {
	c.t.Helper()
	c.n++
	tag := fmt.Sprintf("a%03d", c.n)
	fmt.Fprintf(c.conn, "%s %s\r\n", tag, cmd)

	var out []string
	for {
		line := c.line()
		out = append(out, line)
		if strings.HasPrefix(line, tag+" ") {
			return out
		}
	}
}

// ok runs a command and fails unless it succeeded.
func (c *client) ok(cmd string) []string {
	c.t.Helper()
	out := c.do(cmd)
	if last := out[len(out)-1]; !strings.Contains(last, " OK") {
		c.t.Fatalf("%s: %s", cmd, last)
	}
	return out
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

// A client that has not logged in is told to, and is told nothing else.
func TestIMAPAsksForCredentialsFirst(t *testing.T) {
	c := dial(t)
	for _, cmd := range []string{"LIST \"\" *", "SELECT INBOX", "FETCH 1 (FLAGS)"} {
		out := c.do(cmd)
		if last := out[len(out)-1]; !strings.Contains(last, " NO") {
			t.Errorf("%s answered %q before login", cmd, last)
		}
	}
	// And CAPABILITY, which a client asks before it can log in, still works.
	if got := joined(c.ok("CAPABILITY")); !strings.Contains(got, "IMAP4rev1") {
		t.Errorf("no capability line: %s", got)
	}
}

// A wrong token is refused, and refused the same way a wrong username is —
// which of the two was wrong is information about somebody else's account.
func TestIMAPRefusesABadToken(t *testing.T) {
	c := dial(t)
	out := c.do(`LOGIN someone "not-a-token"`)
	last := out[len(out)-1]
	if !strings.Contains(last, " NO") {
		t.Fatalf("a bad token was accepted: %s", last)
	}
	if strings.Contains(strings.ToLower(last), "no such user") {
		t.Errorf("the refusal says whether the user exists: %s", last)
	}
}

// The wire format of a sequence set, which is where an IMAP server goes wrong
// quietly: a client asks for 2:4 and gets 1:3, or asks for * and gets nothing.
func TestASequenceSetMeansWhatTheProtocolSaysItMeans(t *testing.T) {
	s := &imapSession{msgs: make([]*Message, 5), uids: []uint32{10, 11, 12, 13, 14}}
	for i := range s.msgs {
		s.msgs[i] = &Message{ID: fmt.Sprintf("m%d", i)}
	}

	for _, tc := range []struct {
		set   string
		byUID bool
		want  []int
	}{
		{"1", false, []int{0}},
		{"2:4", false, []int{1, 2, 3}},
		{"1,3,5", false, []int{0, 2, 4}},
		{"4:*", false, []int{3, 4}},
		{"*", false, []int{4}},
		{"*:3", false, []int{2, 3, 4}}, // reversed ranges are the same range
		{"11", true, []int{1}},
		{"11:13", true, []int{1, 2, 3}},
		{"12:*", true, []int{2, 3, 4}},
		{"99", true, nil},
	} {
		got := s.pick(tc.set, tc.byUID)
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%q (uid=%v) picked %v, want %v", tc.set, tc.byUID, got, tc.want)
		}
	}
}

// LIST's wildcards. % must not cross a folder boundary — that is the whole
// difference between it and *, and a client uses % to draw one level of a tree.
func TestListWildcards(t *testing.T) {
	for _, tc := range []struct {
		pattern, name string
		want          bool
	}{
		{"*", "INBOX", true},
		{"*", "INBOX/research", true},
		{"%", "INBOX", true},
		{"%", "INBOX/research", false},
		{"INBOX/%", "INBOX/research", true},
		{"INBOX/*", "INBOX/research", true},
		{"inbox", "INBOX", true},
		{"Junk", "INBOX", false},
	} {
		if got := imapMatch(tc.pattern, tc.name); got != tc.want {
			t.Errorf("%q against %q: %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// A rendered message parses. The original headers were the obvious thing to
// send and are the wrong thing: they describe a multipart whose boundary the
// parser threw away, so a client sees a truncated message and shows nothing.
func TestARenderedMessageIsWellFormed(t *testing.T) {
	m := &Message{
		ID: "1", From: "a@example.com", To: "you@micro.mu",
		Subject: "Invoice 4021", Body: "Attached is this month's invoice.",
		MessageID: "<inv@example.com>", CreatedAt: time.Now(),
		RawHeaders: "Content-Type: multipart/mixed; boundary=\"gone\"\r\n",
	}
	out := string(imapRender(m))

	head, body, found := strings.Cut(out, "\r\n\r\n")
	if !found {
		t.Fatalf("no header/body break:\n%s", out)
	}
	for _, want := range []string{"From: a@example.com", "To: you@micro.mu",
		"Subject: Invoice 4021", "Message-ID: <inv@example.com>", "Content-Type: text/plain"} {
		if !strings.Contains(head, want) {
			t.Errorf("the headers are missing %q:\n%s", want, head)
		}
	}
	if !strings.Contains(body, "this month's invoice") {
		t.Errorf("the body is not in it: %q", body)
	}
	if strings.Contains(out, "boundary=\"gone\"") {
		t.Error("the original Content-Type was replayed; its boundary no longer exists")
	}
}

// A subject with an accent in it is encoded, because a raw UTF-8 byte in a
// header is not a header.
func TestANonASCIISubjectIsEncoded(t *testing.T) {
	m := &Message{ID: "1", From: "a@example.com", Subject: "Déjeuner", CreatedAt: time.Now()}
	out := string(imapRender(m))
	if strings.Contains(out, "Déjeuner") {
		t.Error("the subject went out as raw UTF-8")
	}
	if !strings.Contains(out, "=?utf-8?") {
		t.Errorf("the subject was not encoded:\n%s", out)
	}
}

// Folders are the account's aliases: INBOX holds everything, a tag is a folder
// of its own, and spam is where it can be seen and disagreed with.
func TestFoldersAreTheAccountsAliases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "imap-folders"

	mutex.Lock()
	messages = []*Message{
		{ID: "1", ToID: who, Subject: "plain", CreatedAt: time.Now().Add(-3 * time.Hour)},
		{ID: "2", ToID: who, Tag: "research", Subject: "tagged", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "3", ToID: who, Spam: true, Subject: "junk", CreatedAt: time.Now().Add(-time.Hour)},
		{ID: "4", ToID: "somebody-else", Subject: "not yours", CreatedAt: time.Now()},
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = nil; mutex.Unlock() })

	folders := imapFolders(who)
	if fmt.Sprint(folders) != "[INBOX INBOX/research Junk]" {
		t.Errorf("folders are %v", folders)
	}

	inbox, ok := imapFolder(who, "INBOX")
	if !ok || len(inbox) != 2 {
		t.Fatalf("INBOX holds %d messages, want the plain one and the tagged one", len(inbox))
	}
	// Oldest first: the order UIDs are assigned in. Every page in the product
	// lists the reverse, and taking that order would hand a client a mailbox
	// whose numbers go down.
	if inbox[0].ID != "1" {
		t.Errorf("INBOX is in the wrong order: %s first", inbox[0].ID)
	}

	tagged, _ := imapFolder(who, "INBOX/research")
	if len(tagged) != 1 || tagged[0].ID != "2" {
		t.Errorf("the research folder holds %d messages", len(tagged))
	}
	junk, _ := imapFolder(who, "Junk")
	if len(junk) != 1 || junk[0].ID != "3" {
		t.Errorf("Junk holds %d messages", len(junk))
	}
	if _, ok := imapFolder(who, "Nonsense"); ok {
		t.Error("a folder that does not exist was accepted")
	}
}

// A UID, once given, is that message in that folder forever. Break it and
// clients do not fail visibly — they re-download the mailbox and duplicate it.
func TestAUIDIsStable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "imap-uids"

	first := []*Message{{ID: "a"}, {ID: "b"}}
	uids, next := imapAssign(who, "INBOX", first)
	if fmt.Sprint(uids) != "[1 2]" || next != 3 {
		t.Fatalf("first assignment gave %v, next %d", uids, next)
	}

	// A new message arrives at the front of nothing and the end of the folder.
	again := []*Message{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	uids, next = imapAssign(who, "INBOX", again)
	if fmt.Sprint(uids) != "[1 2 3]" || next != 4 {
		t.Errorf("the numbering moved: %v, next %d", uids, next)
	}

	// One message deleted does not renumber the rest.
	uids, _ = imapAssign(who, "INBOX", []*Message{{ID: "a"}, {ID: "c"}})
	if fmt.Sprint(uids) != "[1 3]" {
		t.Errorf("a deletion renumbered the folder: %v", uids)
	}

	// A different folder is a different numbering.
	other, _ := imapAssign(who, "INBOX/research", []*Message{{ID: "c"}})
	if fmt.Sprint(other) != "[1]" {
		t.Errorf("folders share a numbering: %v", other)
	}

	// And it goes when the account does.
	imapForget(who)
	uids, _ = imapAssign(who, "INBOX", []*Message{{ID: "a"}})
	if fmt.Sprint(uids) != "[1]" {
		t.Errorf("deleting the account left the numbering behind: %v", uids)
	}
}

// BODY[] marks a message read and BODY.PEEK[] does not. That is the only
// difference between them, and getting it wrong marks every message a client
// previews as read behind you.
func TestPeekDoesNotMarkRead(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "imap-peek"

	m := &Message{ID: "1", ToID: who, Subject: "hello", Body: "there", CreatedAt: time.Now()}
	mutex.Lock()
	messages = []*Message{m}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = nil; mutex.Unlock() })

	s := &imapSession{account: who, folder: "INBOX", msgs: []*Message{m},
		uids: []uint32{1}, deleted: map[string]bool{}}

	s.fetchItem(m, 1, "BODY.PEEK[]")
	if m.Read {
		t.Error("a peek marked the message read")
	}
	s.fetchItem(m, 1, "BODY[]")
	if !m.Read {
		t.Error("fetching the body did not mark the message read")
	}
}

// A read-only mailbox is read-only. EXAMINE exists so a client can look without
// changing anything, and a fetch that marked mail read would defeat it.
func TestExamineChangesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "imap-examine"

	m := &Message{ID: "1", ToID: who, Body: "there", CreatedAt: time.Now()}
	mutex.Lock()
	messages = []*Message{m}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = nil; mutex.Unlock() })

	s := &imapSession{account: who, folder: "INBOX", readOnly: true,
		msgs: []*Message{m}, uids: []uint32{1}, deleted: map[string]bool{}}
	s.fetchItem(m, 1, "BODY[]")
	if m.Read {
		t.Error("a read-only mailbox marked a message read")
	}
}

// A search a client actually sends.
func TestSearchTerms(t *testing.T) {
	read := &Message{ID: "1", Read: true, Subject: "Invoice", From: "a@example.com"}
	unread := &Message{ID: "2", Subject: "Lunch", From: "b@example.com", Body: "thursday"}
	s := &imapSession{msgs: []*Message{read, unread}, deleted: map[string]bool{}}

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"ALL", []string{"1", "2"}},
		{"UNSEEN", []string{"2"}},
		{"SEEN", []string{"1"}},
		{`SUBJECT "invoice"`, []string{"1"}},
		{`FROM "b@example.com"`, []string{"2"}},
		{`BODY "thursday"`, []string{"2"}},
		{`UNSEEN SUBJECT "lunch"`, []string{"2"}},
		{`UNSEEN SUBJECT "invoice"`, nil},
	} {
		terms := imapTerms(tc.query)
		var got []string
		for _, m := range s.msgs {
			if s.matches(m, terms) {
				got = append(got, m.ID)
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("SEARCH %s matched %v, want %v", tc.query, got, tc.want)
		}
	}
}

// The tokeniser keeps a quoted string and a bracketed section whole. Splitting
// on spaces is what makes BODY[HEADER.FIELDS (FROM TO)] into four items and a
// client into one that shows blank rows.
func TestTheTokeniserKeepsGroupsWhole(t *testing.T) {
	got := imapItems("(UID FLAGS BODY.PEEK[HEADER.FIELDS (DATE FROM SUBJECT)] RFC822.SIZE)")
	want := []string{"UID", "FLAGS", "BODY.PEEK[HEADER.FIELDS (DATE FROM SUBJECT)]", "RFC822.SIZE"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("split into %q", got)
	}
	if got := imapItems("FAST"); len(got) != 3 {
		t.Errorf("the FAST macro expanded to %v", got)
	}
}

// AUTHENTICATE PLAIN's blob, which several clients prefer to LOGIN.
func TestPlainCredentials(t *testing.T) {
	user, pass, ok := imapPlain("AGFzaW0AdG9rZW4=") // \0asim\0token
	if !ok || user != "asim" || pass != "token" {
		t.Errorf("read %q/%q ok=%v", user, pass, ok)
	}
	if _, _, ok := imapPlain("not base64 at all"); ok {
		t.Error("nonsense was accepted as credentials")
	}
}

// The whole conversation a mail client has, against a real account and a real
// token: log in, list the folders, open one, fetch a header list, read a
// message, mark it read, search for what is left.
//
// End to end because that is the only way to know the wire format is right. Each
// piece is tested above; this is the one that would have caught a response sent
// in the wrong order.
func TestAClientCanReadItsMail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	acc := &auth.Account{ID: "imap-live", Name: "reader", Created: time.Now()}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	_, token, err := auth.CreateToken(acc.ID, "mail client", nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	mutex.Lock()
	messages = []*Message{
		{ID: "1", ToID: acc.ID, From: "a@example.com", To: "reader@micro.mu",
			Subject: "Invoice 4021", Body: "Attached is this month's invoice.",
			MessageID: "<inv@example.com>", CreatedAt: time.Now().Add(-2 * time.Hour)},
		{ID: "2", ToID: acc.ID, From: "b@example.com", To: "reader+research@micro.mu",
			Tag: "research", Subject: "Three papers", Body: "All on retrieval.",
			CreatedAt: time.Now().Add(-time.Hour)},
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = nil; mutex.Unlock() })

	c := dial(t)
	c.ok("LOGIN reader " + token)

	folders := joined(c.ok(`LIST "" *`))
	for _, want := range []string{`"INBOX"`, `"INBOX/research"`, `"Junk"`} {
		if !strings.Contains(folders, want) {
			t.Errorf("LIST is missing %s:\n%s", want, folders)
		}
	}

	selected := joined(c.ok("SELECT INBOX"))
	for _, want := range []string{"* 2 EXISTS", "UIDVALIDITY", "UIDNEXT", "[READ-WRITE]"} {
		if !strings.Contains(selected, want) {
			t.Errorf("SELECT is missing %s:\n%s", want, selected)
		}
	}

	// What a client asks for to draw a message list.
	list := joined(c.ok("UID FETCH 1:* (UID FLAGS RFC822.SIZE BODY.PEEK[HEADER.FIELDS (FROM SUBJECT DATE)])"))
	for _, want := range []string{"UID 1", "UID 2", "Subject: Invoice 4021", "Subject: Three papers"} {
		if !strings.Contains(list, want) {
			t.Errorf("the message list is missing %q:\n%s", want, list)
		}
	}
	// A peek does not mark anything read.
	if got := joined(c.ok("SEARCH UNSEEN")); !strings.Contains(got, "* SEARCH 1 2") {
		t.Errorf("a peek marked mail read: %s", got)
	}

	// And reading one does.
	body := joined(c.ok("UID FETCH 1 (BODY[])"))
	if !strings.Contains(body, "this month's invoice") {
		t.Errorf("the message did not come back:\n%s", body)
	}
	if got := joined(c.ok("SEARCH UNSEEN")); !strings.Contains(got, "* SEARCH 2") ||
		strings.Contains(got, "SEARCH 1") {
		t.Errorf("after reading one, UNSEEN is %q", got)
	}

	// The agent's own folder holds its own mail and nothing else.
	c.ok("SELECT INBOX/research")
	one := joined(c.ok("FETCH 1:* (ENVELOPE)"))
	if !strings.Contains(one, "Three papers") || strings.Contains(one, "Invoice 4021") {
		t.Errorf("the research folder holds the wrong mail:\n%s", one)
	}

	c.ok("LOGOUT")
}
