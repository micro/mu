package mail

// IMAP: your mail, in whatever client you already use.
//
// Mu runs the SMTP server, so mail arrives here and is read here — on a page
// this instance draws. That is a fine place to read mail and it is the only
// place, which makes an address on Mu a smaller thing than an address anywhere
// else: nobody's phone, nobody's Mail.app, nobody's Thunderbird can open it.
// IMAP is the standard answer and has been since 1986. It costs one protocol and
// it turns "an address" into "an address you can actually use".
//
// It matters more than it looks, because the pitch is working with agents that
// have addresses, and an agent that answers your mail is worth something only
// if the mail is somewhere you were going to look anyway. With IMAP the agent's replies appear in the
// thread in the client you already have open, and the folders are the agents —
// see imapbox.go, where a plus-address tag is a folder.
//
// # Scope
//
// IMAP4rev1, the part clients actually use: LOGIN and AUTHENTICATE PLAIN, LIST,
// SELECT and EXAMINE, STATUS, FETCH and UID FETCH, SEARCH and UID SEARCH, STORE
// for \Seen and \Deleted, EXPUNGE, IDLE, and the housekeeping around them. No
// APPEND, no CREATE, RENAME or DELETE: folders here are aliases and spam, both
// derived from what has arrived, so there is nothing for those verbs to do that
// would still be true a minute later. A client is told so — the capability list
// does not claim them and the commands answer NO rather than pretending.
//
// # Passwords
//
// Mu has no password. Sign-in is a passkey or a link, and an IMAP client needs
// something it can put in a text field — so the password is an access token, the
// same one an agent calls the API with, minted at /token. That is the
// app-password pattern every provider ended up at, and it has the property that
// matters: a client is revoked on its own without touching how you sign in.
//
// # TLS
//
// None here, deliberately. Nothing in this repo terminates TLS — the web server
// runs behind a proxy that does — and an IMAP server with its own certificate
// handling would be the first. So this listens in the clear and the operator
// puts the same terminator in front of it on 993. See docs/INSTALL.md.

import (
	"bufio"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
)

// imapIdleTick is how often an idling connection looks for new mail.
//
// The protocol's own limit is 29 minutes before a client must renew IDLE. This
// is about how soon a new message shows up in a client that is sitting open, and
// twenty seconds is the difference between mail that arrives and mail you go and
// fetch.
const imapIdleTick = 20 * time.Second

// imapTimeout closes a connection nothing has said anything on. Generous,
// because an idling client says nothing for a long time on purpose.
const imapTimeout = 30 * time.Minute

// imapSession is one client connection.
type imapSession struct {
	conn net.Conn
	r    *bufio.Reader
	w    *bufio.Writer

	account string // empty until authenticated
	name    string // what they logged in as, for the log

	folder   string // the selected folder, empty for none
	readOnly bool
	msgs     []*Message // the folder as it was when selected, oldest first
	uids     []uint32
	deleted  map[string]bool // marked \Deleted this session
}

// StartIMAPServer serves IMAP on addr until it fails.
func StartIMAPServer(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	app.Log("mail", "Starting IMAP server on %s", addr)
	app.Log("mail", "  - Log in with your username and an access token as the password")
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					app.Log("mail", "an IMAP session panicked: %v", rec)
				}
			}()
			serveIMAP(conn)
		}()
	}
}

// StartIMAPServerIfEnabled starts IMAP unless it is turned off.
//
// On by default and on a high port by default, for the same reason the SMTP
// server defaults to 2525: an unprivileged process cannot bind 143, and a
// default that cannot start is a feature nobody finds. IMAP_PORT=off is the way
// to have none.
func StartIMAPServerIfEnabled() {
	addr, on := app.ListenAddr("IMAP_PORT", app.IMAPPort)
	if !on {
		return
	}
	go func() {
		if err := StartIMAPServer(addr); err != nil {
			app.Log("mail", "IMAP server error: %v", err)
		}
	}()
}

func serveIMAP(conn net.Conn) {
	s := &imapSession{
		conn:    conn,
		r:       bufio.NewReader(conn),
		w:       bufio.NewWriter(conn),
		deleted: map[string]bool{},
	}
	defer conn.Close()

	s.send("* OK [CAPABILITY " + imapCapability(false) + "] Mu IMAP ready")
	for {
		conn.SetReadDeadline(time.Now().Add(imapTimeout)) //nolint:errcheck
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		if !s.command(strings.TrimRight(line, "\r\n")) {
			return
		}
	}
}

// send writes one line and flushes it. IMAP is a conversation; buffering a
// response until the next one would deadlock against a client waiting for it.
func (s *imapSession) send(line string) {
	s.w.WriteString(line + "\r\n") //nolint:errcheck
	s.w.Flush()                    //nolint:errcheck
}

// imapCapability is what this server can do. Authenticated and not are
// different answers: LOGINDISABLED and AUTH= only mean anything before.
// imapCapability is what this server can do.
//
// SPECIAL-USE (RFC 6154) is advertised so a client asks for the attributes on
// LIST and files its sent mail in Sent rather than keeping a local copy the
// server never sees. Announcing the folder without announcing the capability
// leaves a well-behaved client ignoring it.
// LIST-EXTENDED is named because SPECIAL-USE is: RFC 6154 says a server
// advertising the one must answer the selection option, which is the other.
// They were not separable, and claiming the half that draws the nicer folder
// icons while breaking on the half that asks for them is how a client came to
// be told it had no mailboxes.
func imapCapability(authenticated bool) string {
	if authenticated {
		return "IMAP4rev1 IDLE UIDPLUS SPECIAL-USE LIST-EXTENDED ID NAMESPACE"
	}
	return "IMAP4rev1 IDLE UIDPLUS SPECIAL-USE LIST-EXTENDED ID NAMESPACE AUTH=PLAIN"
}

// command handles one line. It returns false when the connection is finished.
func (s *imapSession) command(line string) bool {
	tag, rest := imapCut(line)
	if tag == "" {
		return true
	}
	name, args := imapCut(rest)
	name = strings.ToUpper(name)

	// UID prefixes a command rather than being one.
	uid := false
	if name == "UID" {
		uid = true
		name, args = imapCut(args)
		name = strings.ToUpper(name)
	}

	switch name {
	case "CAPABILITY":
		s.send("* CAPABILITY " + imapCapability(s.account != ""))
		s.ok(tag, "CAPABILITY completed")
	case "NOOP", "CHECK":
		s.ok(tag, name+" completed")
	case "LOGOUT":
		s.send("* BYE Mu IMAP signing off")
		s.ok(tag, "LOGOUT completed")
		return false
	case "ID":
		// RFC 2971. Apple Mail sends this before it sends anything else and
		// Thunderbird sends it again after login, so a server that does not
		// answer it makes a BAD the first thing a client ever hears. Nothing is
		// kept from what the client says about itself; answering is the point.
		s.send(`* ID ("name" "Mu")`)
		s.ok(tag, "ID completed")
	case "NAMESPACE":
		// RFC 2342. One personal namespace and no others: an account here has
		// its own mail and no way to see anybody else's, which is what an empty
		// shared and other-users namespace says.
		s.send(`* NAMESPACE (("" "` + imapDelimiter + `")) NIL NIL`)
		s.ok(tag, "NAMESPACE completed")
	case "ENABLE":
		// RFC 5161. Nothing here needs enabling, and the answer to that is an
		// empty ENABLED line — a client told its command was malformed may
		// retry it, where one told nothing was enabled carries on.
		s.send("* ENABLED")
		s.ok(tag, "ENABLE completed")
	case "LOGIN":
		s.login(tag, args)
	case "AUTHENTICATE":
		s.authenticate(tag, args)
	case "LIST", "LSUB":
		s.list(tag, name, args)
	case "SELECT", "EXAMINE":
		s.selectFolder(tag, args, name == "EXAMINE")
	case "STATUS":
		s.status(tag, args)
	case "FETCH":
		s.fetch(tag, args, uid)
	case "SEARCH":
		s.search(tag, args, uid)
	case "STORE":
		s.store(tag, args, uid)
	case "EXPUNGE":
		s.expunge(tag)
	case "CLOSE":
		s.expungeQuietly()
		s.folder, s.msgs, s.uids = "", nil, nil
		s.ok(tag, "CLOSE completed")
	case "UNSELECT":
		s.folder, s.msgs, s.uids = "", nil, nil
		s.ok(tag, "UNSELECT completed")
	case "IDLE":
		return s.idle(tag)
	case "SUBSCRIBE", "UNSUBSCRIBE":
		// Every folder here exists because there is mail in it, so there is no
		// subscription to keep. Answered OK rather than NO: a client that is
		// told no on subscribe reports it as a failure to the person, and there
		// is nothing for them to do about it.
		s.ok(tag, name+" completed")
	case "CREATE", "DELETE", "RENAME", "APPEND":
		// Folders are aliases and spam, both derived from what has arrived. A
		// folder somebody created here would be empty and would vanish, which
		// is worse than saying so.
		s.no(tag, name+" is not supported: folders here are your addresses, and follow your mail")
	default:
		s.bad(tag, "unknown command "+name)
	}
	return true
}

func (s *imapSession) ok(tag, msg string)  { s.send(tag + " OK " + msg) }
func (s *imapSession) no(tag, msg string)  { s.send(tag + " NO " + msg) }
func (s *imapSession) bad(tag, msg string) { s.send(tag + " BAD " + msg) }

// needAuth answers the command itself when nobody has logged in.
func (s *imapSession) needAuth(tag string) bool {
	if s.account != "" {
		return false
	}
	s.no(tag, "log in first")
	return true
}

// needFolder answers when no folder is selected.
func (s *imapSession) needFolder(tag string) bool {
	if s.needAuth(tag) {
		return true
	}
	if s.folder == "" {
		s.no(tag, "select a mailbox first")
		return true
	}
	return false
}

// login is LOGIN <user> <token>.
func (s *imapSession) login(tag, args string) {
	user, rest := imapCut(args)
	pass, _ := imapCut(rest)
	s.signIn(tag, imapUnquote(user), imapUnquote(pass))
}

// authenticate is AUTHENTICATE PLAIN, which every client offers and some prefer.
func (s *imapSession) authenticate(tag, args string) {
	mech, _ := imapCut(args)
	if !strings.EqualFold(imapUnquote(mech), "PLAIN") {
		s.no(tag, "only PLAIN is supported")
		return
	}
	s.send("+")
	line, err := s.r.ReadString('\n')
	if err != nil {
		return
	}
	user, pass, ok := imapPlain(strings.TrimRight(line, "\r\n"))
	if !ok {
		s.bad(tag, "could not read the credentials")
		return
	}
	s.signIn(tag, user, pass)
}

// signIn is the one place a session becomes somebody's.
//
// The username may be a bare username or the full address, because both are
// what a person has in front of them when filling in a mail client. The password
// is an access token — see the package comment.
func (s *imapSession) signIn(tag, user, pass string) {
	if user == "" || pass == "" {
		s.no(tag, "a username and an access token are needed")
		return
	}
	// Shared with submission, so the two protocols cannot answer the same
	// question differently — see credentials.go, which is where they did.
	acc, err := accountForToken(user, pass)
	if err != nil {
		app.Log("mail", "IMAP sign-in refused for %q", user)
		s.no(tag, err.Error())
		return
	}

	s.account, s.name = acc.ID, acc.ID
	app.Log("mail", "IMAP: %s signed in", acc.ID)
	s.send(tag + " OK [CAPABILITY " + imapCapability(true) + "] signed in")
}

// list is LIST and LSUB, in every form a client sends them. Both answer the
// same thing here — every folder exists because there is mail in it, so there
// is nothing to be subscribed to separately.
//
// The plain form is `LIST "" "*"`. The extended ones (RFC 5258) put selection
// options in parentheses before the reference and return options after the
// pattern, and a client starts sending those the moment it sees SPECIAL-USE in
// the capability list — which this server advertises, and RFC 6154 says
// advertising it obliges answering them.
//
// This read the arguments as "the first word is the reference and everything
// after it is the pattern", so `LIST (SPECIAL-USE) "" "*"` asked for the
// pattern `" "*` and `LIST "" "*" RETURN (SPECIAL-USE)` asked for
// `"*" RETURN (SPECIAL-USE)`. Neither matches a folder, and a folder list is
// not a page that comes back short: a client told it has no mailboxes has no
// INBOX to open, so the account draws empty — no error, no inbox, nothing.
func (s *imapSession) list(tag, name, args string) {
	if s.needAuth(tag) {
		return
	}
	sel, _, patterns := imapListArgs(args)

	// The one query that is not about folders: a client asks for the delimiter
	// before it asks for anything else.
	if len(patterns) == 1 && patterns[0] == "" {
		s.send(`* ` + name + ` (\Noselect) "` + imapDelimiter + `" ""`)
		s.ok(tag, name+" completed")
		return
	}

	// SPECIAL-USE as a selection option means "only the folders that have one".
	// Every other option a client may send — SUBSCRIBED, REMOTE — is either
	// true of everything here or true of nothing, so it changes no answer.
	onlySpecial := imapHas(sel, "SPECIAL-USE")

	folders := imapFolders(s.account)

	// INBOX has children only when a tag folder sits under it. The test was
	// "more than two folders", which counted Junk — and once Sent existed as
	// well every account claimed children it did not have, so a client drew an
	// expander that opened onto nothing.
	tagged := false
	for _, folder := range folders {
		if strings.HasPrefix(folder, imapInbox+imapDelimiter) {
			tagged = true
			break
		}
	}

	for _, folder := range folders {
		if !imapMatchAny(patterns, folder) {
			continue
		}
		// SPECIAL-USE (RFC 6154), which is what makes a client file its sent
		// mail here and treat Junk as junk rather than drawing two more plain
		// folders and putting its own copies somewhere local.
		special := ""
		switch folder {
		case imapSent:
			special = ` \Sent`
		case imapJunk:
			special = ` \Junk`
		}
		if onlySpecial && special == "" {
			continue
		}
		attrs := `\HasNoChildren`
		if folder == imapInbox && tagged {
			attrs = `\HasChildren`
		}
		s.send(`* ` + name + ` (` + attrs + special + `) "` + imapDelimiter + `" ` + imapQuoted(folder))
	}
	s.ok(tag, name+" completed")
}

// imapListArgs reads the arguments to LIST or LSUB: the selection options that
// may come before the reference, the reference, and one or more patterns.
//
// Return options after the pattern are read and dropped. Everything a client
// can ask for with them — the child attributes, the special-use ones — is on
// every line this server sends already, so honouring them is sending what it
// was going to send.
func imapListArgs(args string) (sel []string, ref string, patterns []string) {
	// imapSplit rather than imapCut: a parenthesised group is one argument, and
	// imapCut stops at the first space inside it.
	words := imapSplit(strings.TrimSpace(args))

	i := 0
	if i < len(words) && strings.HasPrefix(words[i], "(") {
		sel = imapGroup(words[i])
		i++
	}
	if i < len(words) {
		ref = imapUnquote(words[i])
		i++
	}
	if i < len(words) {
		if strings.HasPrefix(words[i], "(") {
			for _, p := range imapGroup(words[i]) {
				patterns = append(patterns, imapUnquote(p))
			}
		} else {
			patterns = []string{imapUnquote(words[i])}
		}
	}
	return sel, ref, patterns
}

// imapGroup reads a parenthesised list into its words.
func imapGroup(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	return imapSplit(strings.TrimSpace(s))
}

// imapMatchAny reports whether a folder matches any of the patterns a client
// asked for. LIST-EXTENDED lets it send several in one command.
func imapMatchAny(patterns []string, folder string) bool {
	for _, p := range patterns {
		if imapMatch(p, folder) {
			return true
		}
	}
	return false
}

// selectFolder is SELECT and EXAMINE — the same thing, one of them read-only.
func (s *imapSession) selectFolder(tag, args string, readOnly bool) {
	if s.needAuth(tag) {
		return
	}
	name := imapUnquote(strings.TrimSpace(args))
	msgs, ok := imapFolder(s.account, name)
	if !ok {
		s.no(tag, "no such mailbox")
		return
	}
	uids, next := imapAssign(s.account, imapName(s.account, name), msgs)

	s.folder, s.msgs, s.uids, s.readOnly = imapName(s.account, name), msgs, uids, readOnly
	s.deleted = map[string]bool{}

	unseen := 0
	first := 0
	for i, m := range msgs {
		if !m.Read {
			unseen++
			if first == 0 {
				first = i + 1
			}
		}
	}

	s.send(fmt.Sprintf("* %d EXISTS", len(msgs)))
	s.send("* 0 RECENT")
	s.send(`* FLAGS (\Seen \Deleted)`)
	s.send(`* OK [PERMANENTFLAGS (\Seen \Deleted)] limits`)
	s.send(fmt.Sprintf("* OK [UIDVALIDITY %d] validity", imapValidity(s.account, s.folder)))
	s.send(fmt.Sprintf("* OK [UIDNEXT %d] next", next))
	if first > 0 {
		s.send(fmt.Sprintf("* OK [UNSEEN %d] first unseen", first))
	}
	state := "READ-WRITE"
	if readOnly {
		state = "READ-ONLY"
	}
	s.send(tag + " OK [" + state + "] " + strings.ToUpper(map[bool]string{true: "EXAMINE", false: "SELECT"}[readOnly]) + " completed")
}

// status answers about a folder without selecting it, which is how a client
// checks for new mail in the folders it is not looking at.
func (s *imapSession) status(tag, args string) {
	if s.needAuth(tag) {
		return
	}
	name, rest := imapCut(args)
	name = imapUnquote(name)
	msgs, ok := imapFolder(s.account, name)
	if !ok {
		s.no(tag, "no such mailbox")
		return
	}
	folder := imapName(s.account, name)
	_, next := imapAssign(s.account, folder, msgs)

	unseen := 0
	for _, m := range msgs {
		if !m.Read {
			unseen++
		}
	}

	var out []string
	for _, item := range strings.Fields(strings.Trim(strings.TrimSpace(rest), "()")) {
		switch strings.ToUpper(item) {
		case "MESSAGES":
			out = append(out, "MESSAGES "+strconv.Itoa(len(msgs)))
		case "RECENT":
			out = append(out, "RECENT 0")
		case "UNSEEN":
			out = append(out, "UNSEEN "+strconv.Itoa(unseen))
		case "UIDNEXT":
			out = append(out, "UIDNEXT "+strconv.FormatUint(uint64(next), 10))
		case "UIDVALIDITY":
			out = append(out, "UIDVALIDITY "+strconv.FormatUint(uint64(imapValidity(s.account, folder)), 10))
		}
	}
	s.send("* STATUS " + imapQuoted(folder) + " (" + strings.Join(out, " ") + ")")
	s.ok(tag, "STATUS completed")
}

// refresh re-reads the selected folder, so a message that arrived since SELECT
// is visible. Returns how many there are now.
func (s *imapSession) refresh() int {
	if s.folder == "" {
		return 0
	}
	msgs, ok := imapFolder(s.account, s.folder)
	if !ok {
		return len(s.msgs)
	}
	uids, _ := imapAssign(s.account, s.folder, msgs)
	s.msgs, s.uids = msgs, uids
	return len(msgs)
}

// idle holds the connection open and tells the client when mail arrives.
//
// Without it a client polls, which on a phone is the difference between mail
// that turns up and mail that turns up when you next open the app. The client
// ends it by sending DONE.
func (s *imapSession) idle(tag string) bool {
	if s.needAuth(tag) {
		return true
	}
	s.send("+ idling")

	done := make(chan error, 1)
	go func() {
		for {
			s.conn.SetReadDeadline(time.Time{}) //nolint:errcheck
			line, err := s.r.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			if strings.EqualFold(strings.TrimSpace(line), "DONE") {
				done <- nil
				return
			}
		}
	}()

	was := len(s.msgs)
	ticker := time.NewTicker(imapIdleTick)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				return false
			}
			s.ok(tag, "IDLE terminated")
			return true
		case <-ticker.C:
			if n := s.refresh(); n != was {
				was = n
				s.send(fmt.Sprintf("* %d EXISTS", n))
			}
		}
	}
}

// pick resolves a sequence set to messages, by sequence number or by UID.
//
// "1:*", "2,4:7", "*" — the whole grammar clients use. A number naming nothing
// is skipped rather than refused, which is what the protocol requires: a client
// may name a message that has since gone.
func (s *imapSession) pick(set string, byUID bool) []int {
	if len(s.msgs) == 0 {
		return nil
	}
	var out []int
	seen := map[int]bool{}

	add := func(i int) {
		if i >= 0 && i < len(s.msgs) && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}

	high := uint64(len(s.msgs))
	if byUID && len(s.uids) > 0 {
		high = uint64(s.uids[len(s.uids)-1])
	}

	for _, part := range strings.Split(set, ",") {
		lo, hi, ok := imapRange(part, high)
		if !ok {
			continue
		}
		if byUID {
			for i, uid := range s.uids {
				if uint64(uid) >= lo && uint64(uid) <= hi {
					add(i)
				}
			}
			continue
		}
		for n := lo; n <= hi && n <= uint64(len(s.msgs)); n++ {
			add(int(n) - 1)
		}
	}
	sort.Ints(out)
	return out
}

// imapRange reads one part of a sequence set. "*" is the highest there is.
func imapRange(part string, high uint64) (lo, hi uint64, ok bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return 0, 0, false
	}
	num := func(s string) (uint64, bool) {
		if s == "*" {
			return high, true
		}
		n, err := strconv.ParseUint(s, 10, 32)
		return n, err == nil
	}
	a, b, ranged := strings.Cut(part, ":")
	lo, ok = num(a)
	if !ok {
		return 0, 0, false
	}
	hi = lo
	if ranged {
		if hi, ok = num(b); !ok {
			return 0, 0, false
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, true
}

// flagsOf is a message's flags as the protocol spells them.
func (s *imapSession) flagsOf(m *Message) string {
	var f []string
	if m.Read {
		f = append(f, `\Seen`)
	}
	if s.deleted[m.ID] {
		f = append(f, `\Deleted`)
	}
	return "(" + strings.Join(f, " ") + ")"
}
