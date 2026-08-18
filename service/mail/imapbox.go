package mail

// What IMAP sees: folders, stable numbers, and a message on the wire.
//
// The protocol itself is in imap.go. This is the part that has to agree with
// the rest of Mu — which mail is in which folder, and which number a client
// will still recognise tomorrow.
//
// # Folders are aliases
//
// An account has one mailbox and any number of tags: you+research@ marks a
// message "research", which is how an agent asks for only its own mail without
// needing an account of its own. That is already a folder — it is a named
// subset of your mail with a stable name and an address of its own — so IMAP
// does not have to invent anything. INBOX/research is the tag, spelled the way
// IMAP spells a folder.
//
// INBOX holds everything, tagged or not, rather than only what is untagged. A
// client that syncs one folder gets the whole mailbox, which is the behaviour
// somebody setting up mail expects; anyone who wants the agent's mail on its own
// subscribes to its folder. Mail appearing in two folders at once is ordinary —
// it is what a label is — and the UIDs are per folder, so nothing is confused by
// it.
//
// # Numbers a client can trust
//
// IMAP's contract is that a UID, once given, means that message in that folder
// for as long as UIDVALIDITY does not change, and that UIDs only ever go up.
// Break it and clients do not fail visibly, they re-download the mailbox and
// silently duplicate or lose local state.
//
// Message ids here are nanosecond timestamps, which are neither 32-bit nor
// dense, so the mapping is assigned and kept: the first time a message is seen
// in a folder it gets the next number, and that pair is written down. It is the
// one piece of state IMAP adds, and it is what makes an IMAP client something
// other than a way to download your mail repeatedly.

import (
	"encoding/base64"
	"fmt"
	"mime"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

const (
	// imapInbox is the one folder name the protocol reserves. Case-insensitive
	// on the wire; this is the spelling sent back.
	imapInbox = "INBOX"
	// imapJunk is where the spam filter's decisions are visible. A client that
	// can see them can disagree with them, which is the only way a filter ever
	// gets corrected.
	imapJunk = "Junk"
	// imapDelimiter separates a folder from its parent. "/" rather than "."
	// because a tag may contain neither, and "/" is what a person reads as a
	// path.
	imapDelimiter = "/"
)

// imapUIDs is the assignment, per account and folder.
type imapUIDs struct {
	Validity uint32            `json:"validity"`
	Next     uint32            `json:"next"`
	Assigned map[string]uint32 `json:"assigned"` // message id -> uid
}

var (
	uidMu    sync.Mutex
	uidState = map[string]*imapUIDs{} // account + "\x00" + folder
	uidLoad  sync.Once
	uidDirty bool
)

const uidFile = "mail_imap_uids.json"

func loadUIDs() {
	uidLoad.Do(func() {
		data.LoadJSON(uidFile, &uidState) //nolint:errcheck
		if uidState == nil {
			uidState = map[string]*imapUIDs{}
		}
		go func() {
			for range time.Tick(2 * time.Second) {
				flushUIDs()
			}
		}()
	})
}

// flushUIDs writes the assignment out if it changed.
//
// On a tick rather than on every assignment: a client opening a folder for the
// first time assigns a number to every message in it, and writing the file once
// per message would be the whole mailbox rewritten per message.
func flushUIDs() {
	uidMu.Lock()
	if !uidDirty {
		uidMu.Unlock()
		return
	}
	uidDirty = false
	snapshot := make(map[string]*imapUIDs, len(uidState))
	for k, v := range uidState {
		copied := *v
		copied.Assigned = make(map[string]uint32, len(v.Assigned))
		for id, uid := range v.Assigned {
			copied.Assigned[id] = uid
		}
		snapshot[k] = &copied
	}
	uidMu.Unlock()
	data.SaveJSON(uidFile, snapshot) //nolint:errcheck
}

// uidsFor returns the assignment for one folder, starting one if there is none.
// Caller holds uidMu.
func uidsFor(accountID, folder string) *imapUIDs {
	key := accountID + "\x00" + folder
	s := uidState[key]
	if s == nil {
		s = &imapUIDs{
			// Seconds since the epoch: unique enough per folder, ascending, and
			// fits the 32 bits the protocol allows. It changes only if this
			// record is lost, which is exactly when a client must resynchronise.
			Validity: uint32(time.Now().Unix()),
			Next:     1,
			Assigned: map[string]uint32{},
		}
		uidState[key] = s
		uidDirty = true
	}
	if s.Assigned == nil {
		s.Assigned = map[string]uint32{}
	}
	if s.Next == 0 {
		s.Next = 1
	}
	return s
}

// imapValidity is a folder's UIDVALIDITY.
func imapValidity(accountID, folder string) uint32 {
	loadUIDs()
	uidMu.Lock()
	defer uidMu.Unlock()
	return uidsFor(accountID, folder).Validity
}

// imapAssign gives each message its number in a folder, in the order they
// arrived, and reports the next number a new arrival will take.
//
// Ordered by arrival because that is the one ordering IMAP requires: a UID
// assigned later must be higher. Assigning in any other order — newest first,
// as every page in the product lists them — hands a client a mailbox whose
// numbers go down, which is undefined behaviour it will not report.
func imapAssign(accountID, folder string, msgs []*Message) (uids []uint32, next uint32) {
	loadUIDs()
	uidMu.Lock()
	defer uidMu.Unlock()

	s := uidsFor(accountID, folder)
	uids = make([]uint32, len(msgs))
	for i, m := range msgs {
		uid, ok := s.Assigned[m.ID]
		if !ok {
			uid = s.Next
			s.Assigned[m.ID] = uid
			s.Next++
			uidDirty = true
		}
		uids[i] = uid
	}
	return uids, s.Next
}

// imapForget drops an account's numbering. For account deletion — the record of
// which message was number four is about that account's mail and goes with it.
func imapForget(accountID string) {
	loadUIDs()
	uidMu.Lock()
	defer uidMu.Unlock()
	prefix := accountID + "\x00"
	for k := range uidState {
		if strings.HasPrefix(k, prefix) {
			delete(uidState, k)
			uidDirty = true
		}
	}
}

// imapFolders is every folder an account has: INBOX, one per tag it has been
// written to at, and Junk.
//
// Derived from what has arrived rather than from a list somebody maintains,
// which is the same rule the inbox's own switcher follows: a folder that exists
// because there is mail in it is a truer statement than one that exists because
// a tag was configured once.
func imapFolders(accountID string) []string {
	mutex.RLock()
	tags := map[string]bool{}
	for _, m := range messages {
		if m.ToID != accountID || m.Spam || m.Tag == "" {
			continue
		}
		tags[m.Tag] = true
	}
	mutex.RUnlock()

	names := make([]string, 0, len(tags))
	for tag := range tags {
		names = append(names, imapInbox+imapDelimiter+tag)
	}
	sort.Strings(names)
	return append(append([]string{imapInbox}, names...), imapJunk)
}

// imapFolder returns a folder's messages, oldest first, and whether the name
// names a folder at all.
func imapFolder(accountID, name string) ([]*Message, bool) {
	tag, kind, ok := imapParse(name)
	if !ok {
		return nil, false
	}

	mutex.RLock()
	var out []*Message
	for _, m := range messages {
		if m.ToID != accountID {
			continue
		}
		switch kind {
		case folderJunk:
			if !m.Spam {
				continue
			}
		default:
			if m.Spam {
				continue
			}
			if tag != "" && !strings.EqualFold(m.Tag, tag) {
				continue
			}
		}
		out = append(out, m)
	}
	mutex.RUnlock()

	// Oldest first: the order UIDs are assigned in, and the order IMAP numbers
	// a mailbox in. Every page in the product shows the reverse.
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, true
}

type folderKind int

const (
	folderMail folderKind = iota
	folderJunk
)

// imapParse reads a folder name: its tag, if it names one, and what kind of
// folder it is. Names are matched case-insensitively, which the protocol
// requires for INBOX and every client assumes for everything else.
func imapParse(name string) (tag string, kind folderKind, ok bool) {
	name = strings.Trim(strings.TrimSpace(name), `"`)
	switch {
	case strings.EqualFold(name, imapInbox):
		return "", folderMail, true
	case strings.EqualFold(name, imapJunk):
		return "", folderJunk, true
	}
	prefix := imapInbox + imapDelimiter
	if len(name) > len(prefix) && strings.EqualFold(name[:len(prefix)], prefix) {
		if tag = strings.TrimSpace(name[len(prefix):]); tag != "" {
			return tag, folderMail, true
		}
	}
	return "", folderMail, false
}

// imapName is a folder's name as it should be sent back — INBOX in the
// protocol's spelling, and a tag as the account actually spells it.
func imapName(accountID, name string) string {
	tag, kind, ok := imapParse(name)
	if !ok {
		return name
	}
	switch {
	case kind == folderJunk:
		return imapJunk
	case tag == "":
		return imapInbox
	}
	return imapInbox + imapDelimiter + tag
}

// imapRender is a message as RFC 5322 bytes.
//
// Built rather than replayed. The original headers are kept (RawHeaders) and
// look like the obvious thing to send, but they describe the message as it
// arrived — including a Content-Type naming a multipart boundary that no longer
// exists, because the parser kept the text and dropped the envelope around it.
// A client handed those headers with this body sees a truncated multipart and
// renders nothing.
//
// So this states what is actually here: the fields, the one body, and the one
// attachment where there is one. Everything a client needs to display and thread
// the message, and nothing it cannot parse.
func imapRender(m *Message) []byte {
	var b strings.Builder

	header := func(k, v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		b.WriteString(k + ": " + imapHeaderValue(v) + "\r\n")
	}

	header("From", m.From)
	header("To", m.To)
	header("Subject", m.Subject)
	b.WriteString("Date: " + m.CreatedAt.Format(time.RFC1123Z) + "\r\n")
	header("Message-ID", m.MessageID)
	header("In-Reply-To", m.ReplyTo)
	b.WriteString("MIME-Version: 1.0\r\n")

	body := strings.ReplaceAll(m.Body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	bodyType := "text/plain; charset=utf-8"
	if imapLooksHTML(m.Body) {
		bodyType = "text/html; charset=utf-8"
	}

	if m.Attachment == "" {
		b.WriteString("Content-Type: " + bodyType + "\r\n")
		b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		b.WriteString(body)
		b.WriteString("\r\n")
		return []byte(b.String())
	}

	boundary := "mu-" + m.ID
	b.WriteString(`Content-Type: multipart/mixed; boundary="` + boundary + `"` + "\r\n\r\n")
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString("Content-Type: " + bodyType + "\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n--" + boundary + "\r\n")
	kind := m.AttachmentType
	if kind == "" {
		kind = "application/octet-stream"
	}
	name := m.AttachmentName
	if name == "" {
		name = "attachment"
	}
	b.WriteString("Content-Type: " + kind + "\r\n")
	b.WriteString(`Content-Disposition: attachment; filename="` + imapQuoteSafe(name) + `"` + "\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(imapWrap(m.Attachment))
	b.WriteString("\r\n--" + boundary + "--\r\n")
	return []byte(b.String())
}

// imapHeaderValue encodes anything that is not plain ASCII, because a raw
// UTF-8 byte in a header is not a header.
func imapHeaderValue(v string) string {
	v = strings.ReplaceAll(strings.ReplaceAll(v, "\r", " "), "\n", " ")
	for i := 0; i < len(v); i++ {
		if v[i] > 126 || v[i] < 32 {
			return mime.QEncoding.Encode("utf-8", v)
		}
	}
	return v
}

func imapQuoteSafe(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "")
}

// imapWrap breaks base64 into the lines RFC 2045 allows.
func imapWrap(s string) string {
	const width = 76
	var b strings.Builder
	for len(s) > width {
		b.WriteString(s[:width])
		b.WriteString("\r\n")
		s = s[width:]
	}
	b.WriteString(s)
	return b.String()
}

func imapLooksHTML(s string) bool {
	l := strings.ToLower(s)
	for _, tag := range []string{"<html", "<!doctype html", "<div", "<p>", "<br", "<table"} {
		if strings.Contains(l, tag) {
			return true
		}
	}
	return false
}

// imapEnvelope is the ENVELOPE a client asks for when it wants a list without
// downloading the mail: date, subject, and the addresses.
func imapEnvelope(m *Message) string {
	addr := func(a string) string {
		a = strings.TrimSpace(a)
		if a == "" {
			return "NIL"
		}
		name, mailbox, host := "NIL", a, "NIL"
		if i := strings.LastIndex(a, "@"); i > 0 {
			mailbox, host = a[:i], imapQuoted(a[i+1:])
		}
		return "((" + name + " NIL " + imapQuoted(mailbox) + " " + host + "))"
	}
	from := addr(m.From)
	to := addr(m.To)
	return "(" + imapQuoted(m.CreatedAt.Format(time.RFC1123Z)) + " " +
		imapQuoted(m.Subject) + " " + from + " " + from + " " + from + " " +
		to + " NIL NIL " + imapQuoted(m.ReplyTo) + " " + imapQuoted(m.MessageID) + ")"
}

// imapQuoted is a quoted string, or NIL for nothing. Literals are avoided by
// stripping what cannot be quoted, which is only ever whitespace here.
func imapQuoted(s string) string {
	if strings.TrimSpace(s) == "" {
		return "NIL"
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// imapBodyStructure describes the message without sending it.
//
// One part or two, matching what imapRender builds — a client that is told
// about a part that is not there will ask for it and get nothing back.
func imapBodyStructure(m *Message) string {
	sub := "PLAIN"
	if imapLooksHTML(m.Body) {
		sub = "HTML"
	}
	body := strings.ReplaceAll(m.Body, "\n", "\r\n")
	text := fmt.Sprintf(`("TEXT" %q ("CHARSET" "UTF-8") NIL NIL "8BIT" %d %d)`,
		sub, len(body), strings.Count(body, "\r\n")+1)
	if m.Attachment == "" {
		return text
	}
	kind, subtype := "APPLICATION", "OCTET-STREAM"
	if i := strings.Index(m.AttachmentType, "/"); i > 0 {
		kind = strings.ToUpper(m.AttachmentType[:i])
		subtype = strings.ToUpper(m.AttachmentType[i+1:])
	}
	size := base64.StdEncoding.DecodedLen(len(m.Attachment))
	att := fmt.Sprintf(`(%q %q ("NAME" %s) NIL NIL "BASE64" %d)`,
		kind, subtype, imapQuoted(m.AttachmentName), size)
	return "(" + text + att + ` "MIXED")`
}
