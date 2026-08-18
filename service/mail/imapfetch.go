package mail

// The four commands that do something to messages: FETCH, SEARCH, STORE and
// EXPUNGE — plus the small amount of parsing IMAP's wire format needs.
//
// Kept apart from imap.go, which is the session and the shape of the
// conversation, because this is where the protocol's detail lives and it is the
// part worth reading on its own.

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// fetch is FETCH and UID FETCH.
func (s *imapSession) fetch(tag, args string, byUID bool) {
	if s.needFolder(tag) {
		return
	}
	set, rest := imapCut(args)
	items := imapItems(strings.TrimSpace(rest))

	// A UID FETCH must report the UID whether or not it was asked for —
	// otherwise the client cannot tell which message it got.
	if byUID && !imapHas(items, "UID") {
		items = append([]string{"UID"}, items...)
	}

	for _, i := range s.pick(set, byUID) {
		m := s.msgs[i]
		var out []string
		for _, item := range items {
			if v := s.fetchItem(m, s.uids[i], item); v != "" {
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			continue
		}
		s.send(fmt.Sprintf("* %d FETCH (%s)", i+1, strings.Join(out, " ")))
	}
	s.ok(tag, "FETCH completed")
}

// fetchItem is one requested item and its value, ready to go on the wire.
func (s *imapSession) fetchItem(m *Message, uid uint32, item string) string {
	upper := strings.ToUpper(item)
	switch {
	case upper == "UID":
		return "UID " + strconv.FormatUint(uint64(uid), 10)
	case upper == "FLAGS":
		return "FLAGS " + s.flagsOf(m)
	case upper == "INTERNALDATE":
		return "INTERNALDATE " + imapQuoted(m.CreatedAt.Format("02-Jan-2006 15:04:05 -0700"))
	case upper == "RFC822.SIZE":
		return "RFC822.SIZE " + strconv.Itoa(len(imapRender(m)))
	case upper == "ENVELOPE":
		return "ENVELOPE " + imapEnvelope(m)
	case upper == "BODYSTRUCTURE" || upper == "BODY":
		return strings.ToUpper(item) + " " + imapBodyStructure(m)
	case upper == "RFC822":
		return s.literal(m, "RFC822", imapRender(m))
	case upper == "RFC822.HEADER":
		head, _ := imapSplitMessage(imapRender(m))
		return s.literal(m, "RFC822.HEADER", head)
	case upper == "RFC822.TEXT":
		_, text := imapSplitMessage(imapRender(m))
		return s.literal(m, "RFC822.TEXT", text)
	case strings.HasPrefix(upper, "BODY.PEEK["), strings.HasPrefix(upper, "BODY["):
		return s.fetchBody(m, item)
	}
	return ""
}

// fetchBody answers BODY[...] and BODY.PEEK[...].
//
// The only difference between them is that BODY[] marks the message read and
// PEEK does not — which is the whole reason a client has both, and getting it
// wrong means every message a client previews is marked read behind you.
func (s *imapSession) fetchBody(m *Message, item string) string {
	peek := strings.HasPrefix(strings.ToUpper(item), "BODY.PEEK[")
	open := strings.Index(item, "[")
	close := strings.LastIndex(item, "]")
	if open < 0 || close < open {
		return ""
	}
	section := item[open+1 : close]
	partial := item[close+1:]

	full := imapRender(m)
	head, text := imapSplitMessage(full)

	var body []byte
	switch up := strings.ToUpper(strings.TrimSpace(section)); {
	case up == "":
		body = full
	case up == "HEADER":
		body = head
	case up == "TEXT", up == "1":
		body = text
	case strings.HasPrefix(up, "HEADER.FIELDS.NOT"):
		body = imapHeaderFields(head, imapNames(section), true)
	case strings.HasPrefix(up, "HEADER.FIELDS"):
		body = imapHeaderFields(head, imapNames(section), false)
	default:
		// A part this message does not have. Empty rather than an error: the
		// client asked about a structure it guessed at.
		body = nil
	}

	label := "BODY[" + section + "]"
	if from, n, ok := imapPartial(partial); ok {
		if from > len(body) {
			from = len(body)
		}
		to := from + n
		if to > len(body) {
			to = len(body)
		}
		body = body[from:to]
		label += fmt.Sprintf("<%d>", from)
	}

	if !peek {
		s.markRead(m)
	}
	return fmt.Sprintf("%s {%d}\r\n%s", label, len(body), string(body))
}

// literal is a value sent as a counted string, which is how anything with a
// newline in it has to go.
func (s *imapSession) literal(m *Message, label string, body []byte) string {
	s.markRead(m)
	return fmt.Sprintf("%s {%d}\r\n%s", label, len(body), string(body))
}

// markRead marks a message read, once, and only when the folder is writable.
func (s *imapSession) markRead(m *Message) {
	if s.readOnly || m.Read {
		return
	}
	MarkAsRead(m.ID, s.account) //nolint:errcheck
}

// imapSplitMessage divides a rendered message into its headers and its body.
func imapSplitMessage(full []byte) (head, text []byte) {
	if i := strings.Index(string(full), "\r\n\r\n"); i >= 0 {
		return full[:i+4], full[i+4:]
	}
	return full, nil
}

// imapHeaderFields keeps or drops the named headers, which is how a client
// fetches a message list without downloading the mail.
func imapHeaderFields(head []byte, want []string, invert bool) []byte {
	keep := map[string]bool{}
	for _, w := range want {
		keep[strings.ToLower(w)] = true
	}
	var out strings.Builder
	for _, line := range strings.Split(string(head), "\r\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if keep[strings.ToLower(strings.TrimSpace(name))] != invert {
			out.WriteString(line + "\r\n")
		}
	}
	out.WriteString("\r\n")
	return []byte(out.String())
}

// imapNames pulls the header names out of HEADER.FIELDS (FROM TO SUBJECT).
func imapNames(section string) []string {
	open := strings.Index(section, "(")
	close := strings.LastIndex(section, ")")
	if open < 0 || close < open {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(section[open+1:close], `"`, ""))
}

// imapPartial reads the <from.count> suffix a client uses to fetch part of a
// large message.
func imapPartial(s string) (from, count int, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "<") || !strings.HasSuffix(s, ">") {
		return 0, 0, false
	}
	a, b, found := strings.Cut(s[1:len(s)-1], ".")
	if !found {
		return 0, 0, false
	}
	from, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	count, err = strconv.Atoi(b)
	if err != nil {
		return 0, 0, false
	}
	return from, count, true
}

// search is SEARCH and UID SEARCH.
//
// A subset, and an honest one: what clients actually send is ALL, UNSEEN, and a
// text term somebody typed. Anything not understood is ignored rather than
// refused, so a criterion this does not know narrows nothing instead of
// answering nothing — the failure a person can see and correct.
func (s *imapSession) search(tag, args string, byUID bool) {
	if s.needFolder(tag) {
		return
	}
	terms := imapTerms(args)

	var hits []string
	for i, m := range s.msgs {
		if !s.matches(m, terms) {
			continue
		}
		n := uint64(i + 1)
		if byUID {
			n = uint64(s.uids[i])
		}
		hits = append(hits, strconv.FormatUint(n, 10))
	}
	line := "* SEARCH"
	if len(hits) > 0 {
		line += " " + strings.Join(hits, " ")
	}
	s.send(line)
	s.ok(tag, "SEARCH completed")
}

// matches is one message against every term, all of which must hold.
func (s *imapSession) matches(m *Message, terms []imapTerm) bool {
	for _, t := range terms {
		if !s.matchesOne(m, t) {
			return false
		}
	}
	return true
}

func (s *imapSession) matchesOne(m *Message, t imapTerm) bool {
	has := func(fields ...string) bool {
		want := strings.ToLower(t.value)
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f), want) {
				return true
			}
		}
		return false
	}
	switch t.name {
	case "ALL":
		return true
	case "SEEN":
		return m.Read
	case "UNSEEN", "NEW", "RECENT":
		return !m.Read
	case "DELETED":
		return s.deleted[m.ID]
	case "UNDELETED":
		return !s.deleted[m.ID]
	case "FROM":
		return has(m.From)
	case "TO", "CC", "BCC":
		return has(m.To)
	case "SUBJECT":
		return has(m.Subject)
	case "BODY":
		return has(m.Body)
	case "TEXT":
		return has(m.Subject, m.Body, m.From, m.To)
	case "SINCE", "SENTSINCE":
		when, ok := imapDate(t.value)
		return ok && !m.CreatedAt.Before(when)
	case "BEFORE", "SENTBEFORE":
		when, ok := imapDate(t.value)
		return ok && m.CreatedAt.Before(when)
	case "ON", "SENTON":
		when, ok := imapDate(t.value)
		return ok && m.CreatedAt.YearDay() == when.YearDay() && m.CreatedAt.Year() == when.Year()
	}
	return true
}

// imapTerm is one search criterion.
type imapTerm struct {
	name  string
	value string
}

// imapTerms reads a search query. Terms that take an argument take exactly one.
func imapTerms(args string) []imapTerm {
	fields := imapSplit(args)
	var out []imapTerm
	for i := 0; i < len(fields); i++ {
		name := strings.ToUpper(imapUnquote(fields[i]))
		switch name {
		case "CHARSET":
			i++ // and its value, which is always UTF-8 and never useful
			continue
		case "FROM", "TO", "CC", "BCC", "SUBJECT", "BODY", "TEXT",
			"SINCE", "BEFORE", "ON", "SENTSINCE", "SENTBEFORE", "SENTON":
			if i+1 < len(fields) {
				out = append(out, imapTerm{name: name, value: imapUnquote(fields[i+1])})
				i++
			}
		case "HEADER":
			// HEADER <name> <value>: only the value is matched, against
			// everything, which is closer to right than ignoring it.
			if i+2 < len(fields) {
				out = append(out, imapTerm{name: "TEXT", value: imapUnquote(fields[i+2])})
				i += 2
			}
		default:
			out = append(out, imapTerm{name: name})
		}
	}
	if len(out) == 0 {
		out = append(out, imapTerm{name: "ALL"})
	}
	return out
}

// imapDate reads the date format IMAP searches use: 18-Aug-2026.
func imapDate(s string) (time.Time, bool) {
	t, err := time.Parse("02-Jan-2006", strings.Trim(s, `"`))
	return t, err == nil
}

// store is STORE and UID STORE: the flags a client can change.
//
// Two of them, because two is what there is. \Seen is read state, which the
// whole product already has. \Deleted is a mark, not a deletion — EXPUNGE is
// what removes, and keeping them separate is what makes a delete in a mail
// client undoable right up until it is not.
func (s *imapSession) store(tag, args string, byUID bool) {
	if s.needFolder(tag) {
		return
	}
	if s.readOnly {
		s.no(tag, "this mailbox is open read-only")
		return
	}
	set, rest := imapCut(args)
	action, flags := imapCut(rest)
	action = strings.ToUpper(action)
	silent := strings.HasSuffix(action, ".SILENT")

	add := !strings.HasPrefix(action, "-")
	replace := !strings.HasPrefix(action, "+") && !strings.HasPrefix(action, "-")

	wantSeen := strings.Contains(strings.ToLower(flags), `\seen`)
	wantDeleted := strings.Contains(strings.ToLower(flags), `\deleted`)

	for _, i := range s.pick(set, byUID) {
		m := s.msgs[i]
		switch {
		case replace:
			s.setRead(m, wantSeen)
			s.deleted[m.ID] = wantDeleted
		case add:
			if wantSeen {
				s.setRead(m, true)
			}
			if wantDeleted {
				s.deleted[m.ID] = true
			}
		default:
			if wantSeen {
				s.setRead(m, false)
			}
			if wantDeleted {
				delete(s.deleted, m.ID)
			}
		}
		if !silent {
			out := "FLAGS " + s.flagsOf(m)
			if byUID {
				out = "UID " + strconv.FormatUint(uint64(s.uids[i]), 10) + " " + out
			}
			s.send(fmt.Sprintf("* %d FETCH (%s)", i+1, out))
		}
	}
	s.ok(tag, "STORE completed")
}

func (s *imapSession) setRead(m *Message, read bool) {
	if read == m.Read {
		return
	}
	if read {
		MarkAsRead(m.ID, s.account) //nolint:errcheck
		return
	}
	MarkAsUnread(m.ID, s.account) //nolint:errcheck
}

// expunge removes what STORE marked \Deleted, and tells the client which
// numbers went.
//
// Highest first, because each EXPUNGE renumbers everything above it — a client
// applying them in ascending order deletes the wrong messages from its own copy.
func (s *imapSession) expunge(tag string) {
	if s.needFolder(tag) {
		return
	}
	if s.readOnly {
		s.no(tag, "this mailbox is open read-only")
		return
	}
	for _, n := range s.expungeQuietly() {
		s.send(fmt.Sprintf("* %d EXPUNGE", n))
	}
	s.ok(tag, "EXPUNGE completed")
}

// expungeQuietly does the removal and reports the sequence numbers, highest
// first. CLOSE does the same thing without saying anything, which is the one
// difference between CLOSE and EXPUNGE.
func (s *imapSession) expungeQuietly() []int {
	if s.folder == "" || s.readOnly || len(s.deleted) == 0 {
		return nil
	}
	var gone []int
	for i := len(s.msgs) - 1; i >= 0; i-- {
		m := s.msgs[i]
		if !s.deleted[m.ID] {
			continue
		}
		if err := DeleteMessage(m.ID, s.account); err == nil {
			gone = append(gone, i+1)
		}
	}
	s.deleted = map[string]bool{}
	s.refresh()
	return gone
}

// ---- the wire format ----

// imapCut takes the first space-separated word off a line and returns the rest.
// A quoted word keeps its quotes, which imapUnquote removes.
func imapCut(s string) (first, rest string) {
	s = strings.TrimLeft(s, " ")
	if s == "" {
		return "", ""
	}
	if s[0] == '"' {
		if end := imapEndQuote(s); end > 0 {
			return s[:end+1], strings.TrimLeft(s[end+1:], " ")
		}
	}
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], strings.TrimLeft(s[i+1:], " ")
	}
	return s, ""
}

// imapEndQuote is the index of the closing quote, honouring backslash escapes.
func imapEndQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// imapSplit breaks a line into words, keeping a quoted string or a
// parenthesised group whole.
func imapSplit(s string) []string {
	var out []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			if start < 0 {
				start = i
			}
			if end := imapEndQuote(s[i:]); end > 0 {
				i += end
			}
		case c == '(' || c == '[':
			if start < 0 {
				start = i
			}
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		case c == ' ' && depth == 0:
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// imapItems reads a FETCH item list, expanding the three macros.
func imapItems(s string) []string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = s[1 : len(s)-1]
	}
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ALL":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE"}
	case "FAST":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE"}
	case "FULL":
		return []string{"FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE", "BODY"}
	}
	return imapSplit(s)
}

func imapHas(items []string, want string) bool {
	for _, i := range items {
		if strings.EqualFold(i, want) {
			return true
		}
	}
	return false
}

// imapUnquote removes the quotes a client put round a string, and the escapes
// inside them.
func imapUnquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, `\"`, `"`)
		s = strings.ReplaceAll(s, `\\`, `\`)
	}
	return s
}

// imapPlain reads the base64 blob AUTHENTICATE PLAIN sends:
// authorize\0authenticate\0password.
func imapPlain(line string) (user, pass string, ok bool) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(line))
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 3 {
		return "", "", false
	}
	user = parts[1]
	if user == "" {
		user = parts[0]
	}
	return user, parts[2], user != "" && parts[2] != ""
}

// imapMatch is the two wildcards LIST uses: * matches anything, % matches
// anything without crossing a delimiter.
func imapMatch(pattern, name string) bool {
	p, n := strings.ToUpper(pattern), strings.ToUpper(name)
	return imapMatchAt(p, n)
}

func imapMatchAt(p, n string) bool {
	for len(p) > 0 {
		switch p[0] {
		case '*':
			if len(p) == 1 {
				return true
			}
			for i := 0; i <= len(n); i++ {
				if imapMatchAt(p[1:], n[i:]) {
					return true
				}
			}
			return false
		case '%':
			for i := 0; i <= len(n); i++ {
				if i > 0 && strings.HasPrefix(n[i-1:], imapDelimiter) {
					break
				}
				if imapMatchAt(p[1:], n[i:]) {
					return true
				}
			}
			return false
		default:
			if len(n) == 0 || n[0] != p[0] {
				return false
			}
			p, n = p[1:], n[1:]
		}
	}
	return len(n) == 0
}
