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
	"mime"
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
	case up == "TEXT":
		body = text
	case up == "1", up == "2", up == "3":
		body = imapPart(full, int(up[0]-'0'))
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

// imapPart is the content of one numbered part of a rendered message, as
// transmitted: BODY[1] is the text, BODY[2] the attachment.
//
// # What was wrong
//
// There was no part 2. BODY[1] returned everything after the headers and
// anything else returned nothing, which is right for a message of one part and
// wrong for every message carrying an attachment — and imapBodyStructure has
// always described those correctly, as a two-part MIXED. So a client did what
// the structure told it to, asked for BODY[2], and was handed an empty literal:
// the message showed the line the body carries in place of the attachment —
// "[report.zip (application/zip), 684 bytes — not shown]" — with no file behind
// it, while the same message rendered its report on the web. Reported against
// a DMARC report, which is the attachment this instance receives constantly.
//
// BODY[1] was wrong in the same message and less visibly: it returned the whole
// multipart body, boundaries and base64 included, as if it were the text.
//
// # Why it slices rather than parses
//
// A part fetched by number is returned exactly as it was transmitted — the
// client decodes it using the encoding BODYSTRUCTURE named, which for an
// attachment is BASE64. mime/multipart would decode quoted-printable on the way
// past and not base64, so what came back would be decoded or not depending on
// the part, and a client applying the stated encoding on top would corrupt one
// of them. Slicing between the boundaries cannot do that.
//
// Numbered from 1, and out of range is empty rather than an error: a client
// that asks about a structure this message does not have has guessed, and RFC
// 3501 says an absent part is the empty string.
func imapPart(full []byte, n int) []byte {
	if n < 1 {
		return nil
	}
	head, body := imapSplitMessage(full)
	boundary := imapBoundary(head)
	if boundary == "" {
		// One part, and it is the body. Anything else does not exist.
		if n == 1 {
			return body
		}
		return nil
	}

	// The parts are what lies between the delimiters. The preamble before the
	// first one is not a part, and neither is anything after the closing
	// delimiter, which is why the split is taken from index 1.
	sections := strings.Split(string(body), "--"+boundary)
	if n >= len(sections) {
		return nil
	}
	part := sections[n]
	if strings.HasPrefix(part, "--") {
		return nil // the closing delimiter: there is no part here
	}
	part = strings.TrimPrefix(part, "\r\n")
	// A part is its own headers, a blank line, then its content. BODY[n] is the
	// content; BODY[n.MIME] is the headers, which nothing here claims to serve.
	if i := strings.Index(part, "\r\n\r\n"); i >= 0 {
		part = part[i+4:]
	}
	return []byte(strings.TrimSuffix(part, "\r\n"))
}

// imapBoundary is the multipart boundary a rendered message declares, or "" for
// a message of one part.
func imapBoundary(head []byte) string {
	for _, line := range strings.Split(string(head), "\r\n") {
		if !strings.HasPrefix(strings.ToLower(line), "content-type:") {
			continue
		}
		_, params, err := mime.ParseMediaType(strings.TrimSpace(line[len("content-type:"):]))
		if err != nil {
			return ""
		}
		return params["boundary"]
	}
	return ""
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
//
// That last rule has one exception it took a real client to find, and it is
// why the parser below is a parser rather than a list. Ignoring a *criterion*
// is safe; ignoring a *modifier* inverts the query. Android's mail sync — the
// AOSP engine Gmail uses for a third-party IMAP account — asks for a mailbox
// with `UID SEARCH 1:50 NOT DELETED`, and with NOT dropped that reads as
// "deleted", which nothing here is. So the answer was an empty SEARCH, on the
// one command the client builds its message list from: the folders were right,
// the counts were right, and every mailbox was empty.
func (s *imapSession) search(tag, args string, byUID bool) {
	if s.needFolder(tag) {
		return
	}
	terms := imapTerms(args)

	var hits []string
	for i, m := range s.msgs {
		var uid uint32
		if i < len(s.uids) {
			uid = s.uids[i]
		}
		if !s.matches(m, i+1, uid, terms) {
			continue
		}
		n := uint64(i + 1)
		if byUID {
			n = uint64(uid)
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

// matches is one message against every term, all of which must hold. seq is its
// place in the folder, counting from one, and uid its number there — both are
// what a sequence set in the query is asking about.
func (s *imapSession) matches(m *Message, seq int, uid uint32, terms []imapTerm) bool {
	for _, t := range terms {
		if !s.matchesOne(m, seq, uid, t) {
			return false
		}
	}
	return true
}

func (s *imapSession) matchesOne(m *Message, seq int, uid uint32, t imapTerm) bool {
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

	// The three that are made of other terms.
	case "AND":
		return s.matches(m, seq, uid, t.sub)
	case "NOT":
		return len(t.sub) == 1 && !s.matchesOne(m, seq, uid, t.sub[0])
	case "OR":
		for _, sub := range t.sub {
			if s.matchesOne(m, seq, uid, sub) {
				return true
			}
		}
		return false

	// A bare set is sequence numbers; UID <set> is the numbers this server
	// handed out. Both are how a client asks for a window of a mailbox rather
	// than all of it.
	case "SET":
		return imapInSet(t.value, uint64(seq), uint64(len(s.msgs)))
	case "UID":
		high := uint64(0)
		if len(s.uids) > 0 {
			high = uint64(s.uids[len(s.uids)-1])
		}
		return imapInSet(t.value, uint64(uid), high)

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
	case "LARGER":
		n, err := strconv.Atoi(t.value)
		return err == nil && len(imapRender(m)) > n
	case "SMALLER":
		n, err := strconv.Atoi(t.value)
		return err == nil && len(imapRender(m)) < n

	// Flags this server does not keep. Answered honestly rather than left to
	// the permissive default, because ANSWERED matching everything is a worse
	// answer than ANSWERED matching nothing — there is no such flag here.
	case "ANSWERED", "FLAGGED", "DRAFT", "KEYWORD":
		return false
	case "UNANSWERED", "UNFLAGGED", "UNDRAFT", "UNKEYWORD", "OLD":
		return true
	}
	return true
}

// imapTerm is one search criterion. NOT, OR and a parenthesised group are made
// of other terms and hold them in sub.
type imapTerm struct {
	name  string
	value string
	sub   []imapTerm
}

// imapTerms reads a search query.
//
// A parser rather than a scan over a list, because IMAP's search grammar nests:
// NOT takes a term, OR takes two, and either may be a parenthesised group of
// more. A scan sees those as three unknown words and drops them, which is fine
// for a criterion and wrong for a modifier — see search, where a dropped NOT
// emptied every mailbox on Android.
func imapTerms(args string) []imapTerm {
	out, _ := imapParseTerms(imapSplit(args), 0)
	if len(out) == 0 {
		return []imapTerm{{name: "ALL"}}
	}
	return out
}

// imapParseTerms reads terms from i until the words run out.
func imapParseTerms(f []string, i int) ([]imapTerm, int) {
	var out []imapTerm
	for i < len(f) {
		t, next, ok := imapParseTerm(f, i)
		if next <= i {
			break // no progress: a malformed tail, and better dropped than looped on
		}
		i = next
		if ok {
			out = append(out, t)
		}
	}
	return out, i
}

// imapParseTerm reads one term and says where the next one starts.
func imapParseTerm(f []string, i int) (t imapTerm, next int, ok bool) {
	if i >= len(f) {
		return imapTerm{}, i, false
	}
	raw := f[i]

	// A parenthesised group is every term inside it, all of which must hold.
	if strings.HasPrefix(raw, "(") {
		inner, _ := imapParseTerms(imapGroup(raw), 0)
		return imapTerm{name: "AND", sub: inner}, i + 1, len(inner) > 0
	}

	name := strings.ToUpper(imapUnquote(raw))
	switch name {
	case "CHARSET":
		// And its value, which is always UTF-8 and never useful.
		return imapParseTerm(f, i+2)
	case "NOT":
		sub, next, ok := imapParseTerm(f, i+1)
		if !ok {
			return imapTerm{}, next, false
		}
		return imapTerm{name: "NOT", sub: []imapTerm{sub}}, next, true
	case "OR":
		a, afterA, okA := imapParseTerm(f, i+1)
		b, afterB, okB := imapParseTerm(f, afterA)
		if !okA || !okB {
			return imapTerm{}, max(afterB, i+1), false
		}
		return imapTerm{name: "OR", sub: []imapTerm{a, b}}, afterB, true
	case "FROM", "TO", "CC", "BCC", "SUBJECT", "BODY", "TEXT",
		"SINCE", "BEFORE", "ON", "SENTSINCE", "SENTBEFORE", "SENTON",
		"LARGER", "SMALLER", "KEYWORD", "UNKEYWORD", "UID":
		if i+1 >= len(f) {
			return imapTerm{}, i + 1, false
		}
		return imapTerm{name: name, value: imapUnquote(f[i+1])}, i + 2, true
	case "HEADER":
		// HEADER <name> <value>: only the value is matched, against everything,
		// which is closer to right than ignoring it.
		if i+2 >= len(f) {
			return imapTerm{}, i + 3, false
		}
		return imapTerm{name: "TEXT", value: imapUnquote(f[i+2])}, i + 3, true
	}

	// A bare sequence set — "1:50", "2,4:7", "*" — which is how a client asks
	// for a window of a mailbox. Recognised by its shape, since it is the one
	// term with no keyword in front of it.
	if imapIsSet(name) {
		return imapTerm{name: "SET", value: name}, i + 1, true
	}

	// Anything else narrows nothing. See the note on search.
	return imapTerm{name: name}, i + 1, true
}

// imapIsSet reports whether a word is a sequence set rather than a criterion.
func imapIsSet(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && c != ':' && c != ',' && c != '*' {
			return false
		}
	}
	return strings.ContainsAny(s, "0123456789*")
}

// imapInSet reports whether n falls in a sequence set, with high standing in
// for "*".
func imapInSet(set string, n, high uint64) bool {
	for _, part := range strings.Split(set, ",") {
		lo, hi, ok := imapRange(part, high)
		if ok && n >= lo && n <= hi {
			return true
		}
	}
	return false
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
