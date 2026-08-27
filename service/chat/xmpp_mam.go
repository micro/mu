package chat

// The archive: what was said before. XEP-0313.
//
// # Why this is the first thing after the handshake
//
// A client that can send and receive but shows nothing you said yesterday is
// not a chat client, it is a walkie-talkie. Open Conversations, tap your agent,
// and an empty screen is what you get — while internal/thread has every word of
// it, because every client here writes to the record on every turn. The
// messages were never missing. There was no way to ask for them.
//
// That is also why this comes before group chat: rooms are a feature some
// people want, and scrollback is what everybody assumes is there.
//
// # It is a read over this service's own record
//
// store.go holds what was said here, the way service/mail holds mail. This is
// a query over it and keeps nothing of its own: delete this file and the
// history is still there, you just cannot ask for it over XMPP.
//
// It reads stanzas rather than the prose copy an agent remembers, which is the
// point of chat owning its record. A client asking for its archive should get
// what was actually sent, with the addresses it was sent between and the ids it
// can page from — not a rendering of it made for something else to read.
//
// # What is implemented
//
// A query for one conversation (`with`), or the whole archive, with a page size
// and a cursor backwards. That is what a client does when it opens a chat and
// when it scrolls up, which is the whole of the common case. Date ranges and
// forward paging are in the XEP and are not here; a client that sends them gets
// the newest page rather than an error, which degrades to "correct but not what
// was asked" rather than to a broken screen.

import (
	"encoding/xml"
	"strconv"
	"strings"
	"time"
)

const (
	nsMAM     = "urn:xmpp:mam:2"
	nsForward = "urn:xmpp:forward:0"
	nsDelay   = "urn:xmpp:delay"
	nsRSM     = "http://jabber.org/protocol/rsm"
)

// mamDefaultPage is how many messages a client gets when it does not say.
//
// Fifty is what the common clients ask for, and the number matters less than
// having one: a query with no limit over an account with five thousand messages
// is a stanza storm the client will spend a minute rendering.
const mamDefaultPage = 50

// mamMax bounds a page however much a client asks for.
const mamMax = 250

// archiveQuery is the shape of a MAM request.
//
// Decoded rather than pattern-matched. The filters arrive inside a data form
// (XEP-0004) because the XEP reuses one, so `with` is a field value rather than
// an attribute — which is worth decoding properly, since guessing at it with a
// regular expression is how a query for one conversation quietly returns
// somebody's whole archive.
type archiveQuery struct {
	QueryID string `xml:"queryid,attr"`
	Form    struct {
		Fields []struct {
			Var    string   `xml:"var,attr"`
			Values []string `xml:"value"`
		} `xml:"field"`
	} `xml:"x"`
	Set struct {
		Max    string `xml:"max"`
		Before *struct {
			ID string `xml:",chardata"`
		} `xml:"before"`
		After string `xml:"after"`
	} `xml:"set"`
}

// with is the JID this query is about, or empty for the whole archive.
func (q archiveQuery) with() string {
	for _, f := range q.Form.Fields {
		if f.Var == "with" && len(f.Values) > 0 {
			return strings.ToLower(bareOf(f.Values[0]))
		}
	}
	return ""
}

func (q archiveQuery) page() int {
	n, err := strconv.Atoi(strings.TrimSpace(q.Set.Max))
	if err != nil || n <= 0 {
		return mamDefaultPage
	}
	if n > mamMax {
		return mamMax
	}
	return n
}

// archive answers a MAM query.
func (s *session) archive(st stanza) {
	var q archiveQuery
	// Inner is the iq's children, and for this query that is exactly one
	// element — <query xmlns='urn:xmpp:mam:2'>…</query> — so it is a whole
	// document already and needs no root wrapped around it.
	if err := xml.Unmarshal(st.Inner, &q); err != nil {
		s.send(`<iq type='error' id='%s'><error type='modify'>`+
			`<bad-request xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>`+
			`</error></iq>`, xmlAttr(st.ID)) //nolint:errcheck
		return
	}

	// Oldest first already — the store sorts, because the XEP requires that
	// order and a client renders in it.
	msgs := s.archived(q.with())

	// Page backwards from the end, because that is what opening a conversation
	// asks for: the most recent, then earlier when you scroll up.
	if before := q.Set.Before; before != nil && before.ID != "" {
		for i, m := range msgs {
			if m.ID == before.ID {
				msgs = msgs[:i]
				break
			}
		}
	}
	complete := true
	if n := q.page(); len(msgs) > n {
		msgs = msgs[len(msgs)-n:]
		// More behind this page, which is what tells a client it may scroll up
		// again. Saying complete when there is more is how a client stops
		// asking and a conversation appears to begin in the middle.
		complete = false
	}

	for _, m := range msgs {
		s.sendArchived(q.QueryID, m)
	}
	s.finish(st.ID, msgs, complete)
}

// archived is the messages this query is about.
//
// Mail is not in it and cannot be: this reads chat's own record. That used to
// need saying, because both lived in one store and the filter was the only
// thing keeping a year of email out of an answer to "what did we say".
func (s *session) archived(with string) []Said {
	if s.acc == nil {
		return nil
	}
	if with != "" {
		return Conversation(s.acc.ID, xmppRoom(s.bare(), with), mamMax)
	}
	return Everything(s.acc.ID, mamMax)
}

// sendArchived writes one stored message as the XEP wraps it.
func (s *session) sendArchived(queryID string, m Said) {
	var b strings.Builder
	b.WriteString(`<message to='` + xmlAttr(s.jid()) + `'>`)
	b.WriteString(`<result xmlns='` + nsMAM + `' queryid='` + xmlAttr(queryID) +
		`' id='` + xmlAttr(m.ID) + `'>`)
	b.WriteString(`<forwarded xmlns='` + nsForward + `'>`)
	b.WriteString(`<delay xmlns='` + nsDelay + `' stamp='` +
		xmlAttr(m.At.UTC().Format(time.RFC3339)) + `'/>`)
	b.WriteString(`<message xmlns='jabber:client' type='chat' id='` + xmlAttr(m.ID) +
		`' from='` + xmlAttr(m.From) + `' to='` + xmlAttr(m.To) + `'>`)
	b.WriteString(`<body>` + xmlText(m.Text) + `</body>`)
	b.WriteString(`</message></forwarded></result></message>`)
	s.send("%s", b.String()) //nolint:errcheck
}

// finish closes the query, and says whether there is more behind it.
func (s *session) finish(id string, msgs []Said, complete bool) {
	var rsm strings.Builder
	rsm.WriteString(`<set xmlns='` + nsRSM + `'>`)
	if len(msgs) > 0 {
		rsm.WriteString(`<first index='0'>` + xmlAttr(msgs[0].ID) + `</first>`)
		rsm.WriteString(`<last>` + xmlAttr(msgs[len(msgs)-1].ID) + `</last>`)
	}
	rsm.WriteString(`<count>` + strconv.Itoa(len(msgs)) + `</count></set>`)

	done := "false"
	if complete {
		done = "true"
	}
	s.send(`<iq type='result' id='%s'><fin xmlns='%s' complete='%s'>%s</fin></iq>`,
		xmlAttr(id), nsMAM, done, rsm.String()) //nolint:errcheck
}
