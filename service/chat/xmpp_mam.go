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
// # It is a read, not a store
//
// No archive of its own. internal/thread is the system of record and this is a
// query over it, which is the same relationship service/recall has to the same
// data from the other side — one of them is a person searching on purpose and
// this one is a client asking for the last fifty. Delete this file and nothing
// is lost except the ability to ask over XMPP.
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
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/internal/thread"
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

	msgs := s.archived(q.with())

	// Oldest first, which is the order the XEP requires results in and the
	// order a client renders them.
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].At.Before(msgs[j].At) })

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
func (s *session) archived(with string) []thread.Message {
	if s.acc == nil {
		return nil
	}
	if with != "" {
		th := thread.Find(s.acc.ID, thread.ChatClient, xmppRoom(s.bare(), with))
		if th == nil {
			return nil
		}
		return thread.Messages(s.acc.ID, th.ID, mamMax)
	}
	// The whole archive: every chat conversation this account has. Mail is not
	// in it, deliberately — an XMPP client asking for its archive is asking
	// about chat, and handing it a year of email is not what it meant.
	var out []thread.Message
	for _, t := range thread.List(s.acc.ID, mamMax) {
		if t.Client != thread.ChatClient {
			continue
		}
		out = append(out, thread.Messages(s.acc.ID, t.ID, mamMax)...)
	}
	return out
}

// sendArchived writes one stored message as the XEP wraps it.
func (s *session) sendArchived(queryID string, m thread.Message) {
	from, to := s.addressed(m)
	var b strings.Builder
	b.WriteString(`<message to='` + xmlAttr(s.jid()) + `'>`)
	b.WriteString(`<result xmlns='` + nsMAM + `' queryid='` + xmlAttr(queryID) +
		`' id='` + xmlAttr(m.ID) + `'>`)
	b.WriteString(`<forwarded xmlns='` + nsForward + `'>`)
	b.WriteString(`<delay xmlns='` + nsDelay + `' stamp='` +
		xmlAttr(m.At.UTC().Format(time.RFC3339)) + `'/>`)
	b.WriteString(`<message xmlns='jabber:client' type='chat' id='` + xmlAttr(m.ID) +
		`' from='` + xmlAttr(from) + `' to='` + xmlAttr(to) + `'>`)
	b.WriteString(`<body>` + xmlText(m.Text) + `</body>`)
	b.WriteString(`</message></forwarded></result></message>`)
	s.send("%s", b.String()) //nolint:errcheck
}

// addressed is who a stored message was between.
//
// The record keeps From where the client knew one — person-to-person XMPP sets
// it — and leaves it empty otherwise, because for most clients the author is
// simply the account. So Role is the fallback and it is enough: a conversation
// has two sides, and an agent's answer came from the address it answers on.
func (s *session) addressed(m thread.Message) (from, to string) {
	me := s.bare()
	if m.Role == thread.RoleAgent {
		other := strings.TrimSpace(m.From)
		if other == "" {
			other = agentJID()
		}
		return other, me
	}
	if f := strings.TrimSpace(m.From); f != "" && !strings.EqualFold(f, me) {
		return f, me
	}
	if t := strings.TrimSpace(m.To); t != "" {
		return me, t
	}
	return me, agentJID()
}

// finish closes the query, and says whether there is more behind it.
func (s *session) finish(id string, msgs []thread.Message, complete bool) {
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
