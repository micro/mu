package chat

// Private, persistent XEP-0045 rooms are projections of Groups membership.
// Room creation and invitations live in Groups, never in arbitrary join stanzas.
// OMEMO envelopes are relayed intact; this service has no participant private keys.
import (
	"context"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
	"mu/internal/group"
)

const nsMUC = "http://jabber.org/protocol/muc"
const nsMUCUser = nsMUC + "#user"
const nsMUCAdmin = nsMUC + "#admin"

var mucMu sync.Mutex

type mucRoom struct {
	sync.Mutex
	occupants map[*session]string
}

var mucRooms = map[string]*mucRoom{}

func mucState(room string, create bool) *mucRoom {
	mucMu.Lock()
	defer mucMu.Unlock()
	state := mucRooms[room]
	if state == nil && create {
		state = &mucRoom{occupants: map[*session]string{}}
		mucRooms[room] = state
	}
	return state
}
func mucSnapshot() map[string]*mucRoom {
	mucMu.Lock()
	defer mucMu.Unlock()
	out := make(map[string]*mucRoom, len(mucRooms))
	for id, state := range mucRooms {
		out[id] = state
	}
	return out
}

func mucDomain() string         { return "groups." + Domain() }
func mucJID(room string) string { return groupID(room) + "@" + mucDomain() }
func mucTarget(to string) (string, bool) {
	bare := strings.ToLower(bareOf(to))
	if bare == mucDomain() {
		return "", true
	}
	local, domain := splitJID(bare)
	if domain != mucDomain() {
		return "", false
	}
	return group.RoomPrefix + local, true
}
func (s *session) mucError(st stanza, kind, condition, text string) {
	if st.Type == "error" || st.Type == "result" {
		return
	}
	s.send(`<%s type='error' id='%s' from='%s' to='%s'><error type='cancel'><%s xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/><text xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'>%s</text></error></%s>`, kind, xmlAttr(st.ID), xmlAttr(st.To), xmlAttr(s.jid()), condition, xmlAttr(text), kind)
}
func (s *session) mucResult(st stanza, body string) {
	s.send(`<iq type='result' id='%s' from='%s' to='%s'>%s</iq>`, xmlAttr(st.ID), xmlAttr(bareOf(st.To)), xmlAttr(s.jid()), body)
}
func (s *session) mucIQ(st stanza) bool {
	room, ok := mucTarget(st.To)
	if !ok {
		return false
	}
	if st.Type == "result" || st.Type == "error" {
		return true
	}
	var g group.Group
	if room != "" {
		var err error
		g, err = group.Read(groupID(room), s.acc.ID)
		if err != nil {
			s.mucError(st, "iq", "item-not-found", "Group not found")
			return true
		}
	}

	if room != "" {
		if raw, ok := childElement(st.Inner, nsMAM, "query"); ok {
			s.mucArchive(st, room, raw)
			return true
		}
	}
	if st.Type != "get" {
		s.mucError(st, "iq", "not-allowed", "Manage groups and invitations in Micro's Groups page.")
		return true
	}
	var b strings.Builder
	if _, ok := childElement(st.Inner, nsDisco, "query"); ok {
		b.WriteString(`<query xmlns='` + nsDisco + `'><identity category='conference' type='text' name='`)
		if room == "" {
			b.WriteString("Micro groups")
		} else {
			b.WriteString(xmlAttr(g.Name))
		}
		b.WriteString(`'/><feature var='` + nsMUC + `'/><feature var='` + nsDisco + `'/><feature var='` + nsDiscoItem + `'/>`)
		if room != "" {
			for _, f := range []string{nsMAM, "muc_persistent", "muc_hidden", "muc_membersonly", "muc_nonanonymous", "muc_unmoderated", "muc_unsecured"} {
				b.WriteString(`<feature var='` + f + `'/>`)
			}
		}
		b.WriteString(`</query>`)
	} else if _, ok := childElement(st.Inner, nsDiscoItem, "query"); ok {
		b.WriteString(`<query xmlns='` + nsDiscoItem + `'>`)
		if room == "" {
			gs, _, err := group.List(s.acc.ID)
			if err != nil {
				s.mucError(st, "iq", "internal-server-error", "Groups unavailable")
				return true
			}
			for _, g := range gs {
				fmt.Fprintf(&b, `<item jid='%s' name='%s'/>`, xmlAttr(mucJID(group.RoomPrefix+g.ID)), xmlAttr(g.Name))
			}
		} else {
			state := mucState(room, true)
			state.Lock()
			for peer, nick := range state.occupants {
				if Member(room, peer.acc.ID) {
					fmt.Fprintf(&b, `<item jid='%s/%s' name='%s'/>`, xmlAttr(mucJID(room)), xmlAttr(nick), xmlAttr(nick))
				}
			}
			state.Unlock()
		}
		b.WriteString(`</query>`)
	} else if raw, ok := childElement(st.Inner, nsMUCAdmin, "query"); ok && room != "" {
		var q struct {
			Item struct {
				Affiliation string `xml:"affiliation,attr"`
			} `xml:"item"`
		}
		if xml.Unmarshal(raw, &q) != nil {
			s.mucError(st, "iq", "bad-request", "Invalid membership query")
			return true
		}
		a := q.Item.Affiliation
		if a != "owner" && a != "admin" && a != "member" {
			s.mucError(st, "iq", "bad-request", "Choose a member affiliation")
			return true
		}
		b.WriteString(`<query xmlns='` + nsMUCAdmin + `'>`)
		names := make([]string, 0, len(g.Members))
		for who := range g.Members {
			names = append(names, who)
		}
		sort.Strings(names)
		for _, who := range names {
			if g.Members[who] == a {
				fmt.Fprintf(&b, `<item affiliation='%s' jid='%s'/>`, a, xmlAttr(who+"@"+Domain()))
			}
		}
		b.WriteString(`</query>`)
	} else {
		s.mucError(st, "iq", "feature-not-implemented", "This group operation is not supported")
		return true
	}
	s.mucResult(st, b.String())
	return true
}

func mucPresenceXML(room, nick string, peer, recipient *session, unavailable bool, code string) string {
	typ := ""
	role := "participant"
	affiliation := group.Role(groupID(room), peer.acc.ID)
	if affiliation == "owner" || affiliation == "admin" {
		role = "moderator"
	}
	if unavailable {
		typ = ` type='unavailable'`
		role = "none"
	}
	if affiliation == "" {
		affiliation = "none"
	}
	status := ""
	if peer == recipient {
		status = `<status code='110'/>`
	}
	if code != "" {
		status += `<status code='` + code + `'/>`
	}
	return fmt.Sprintf(`<presence from='%s/%s' to='%s'%s><x xmlns='%s'><item affiliation='%s' role='%s' jid='%s'/>%s</x></presence>`, xmlAttr(mucJID(room)), xmlAttr(nick), xmlAttr(recipient.jid()), typ, nsMUCUser, affiliation, role, xmlAttr(peer.jid()), status)
}
func (s *session) mucPresence(st stanza) bool {
	room, ok := mucTarget(st.To)
	if !ok {
		return false
	}
	if st.Type == "error" {
		return true
	}
	if st.Type == "unavailable" {
		if state := mucState(room, false); state != nil {
			state.Lock()
			leaveMUCLocked(room, state, s, "")
			state.Unlock()
		}
		return true
	}
	if st.Type != "" {
		return true
	}
	if room == "" || !Member(room, s.acc.ID) {
		s.mucError(st, "presence", "registration-required", "Accept the group's invitation in Micro first.")
		return true
	}
	state := mucState(room, true)
	state.Lock()
	defer state.Unlock()
	_, nick, has := strings.Cut(st.To, "/")
	if !has || nick == "" || len(nick) > 100 || strings.ContainsAny(nick, "/\x00\n\r") {
		s.mucError(st, "presence", "jid-malformed", "Choose a room nickname")
		return true
	}
	if nick != s.acc.ID {
		if _, err := auth.GetAccount(nick); err == nil {
			s.mucError(st, "presence", "conflict", "That nickname belongs to another account")
			return true
		}
	}
	for peer, n := range state.occupants {
		if n == nick && peer.acc.ID != s.acc.ID {
			s.mucError(st, "presence", "conflict", "Nickname already in use")
			return true
		}
	}
	if old := state.occupants[s]; old != "" {
		if old == nick {
			s.send("%s", mucPresenceXML(room, nick, s, s, false, ""))
			return true
		}
		s.mucError(st, "presence", "not-allowed", "Leave before changing your nickname")
		return true
	}
	r := getOrCreateRoom(room)
	if r == nil {
		s.mucError(st, "presence", "item-not-found", "Group not found")
		return true
	}
	// Holding the room lock keeps live broadcasts behind presence and history replay.
	for peer, n := range state.occupants {
		if Member(room, peer.acc.ID) {
			s.send("%s", mucPresenceXML(room, n, peer, s, false, ""))
		}
	}
	state.occupants[s] = nick
	for peer := range state.occupants {
		if Member(room, peer.acc.ID) {
			peer.send("%s", mucPresenceXML(room, nick, s, peer, false, ""))
		}
	}
	r.mutex.RLock()
	history := append([]RoomMessage(nil), r.Messages...)
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	r.mutex.RUnlock()
	// Respect a client's request to omit join history.
	var x struct {
		History struct {
			Max string `xml:"maxstanzas,attr"`
		} `xml:"history"`
	}
	if raw, ok := childElement(st.Inner, nsMUC, "x"); ok {
		_ = xml.Unmarshal(raw, &x)
	}
	if x.History.Max != "0" {
		for _, m := range history {
			if !m.System && Member(room, s.acc.ID) {
				s.send("%s", mucMessageXML(room, m, s, true))
			}
		}
	}
	s.send(`<message type='groupchat' from='%s' to='%s'><subject>%s</subject></message>`, xmlAttr(mucJID(room)), xmlAttr(s.jid()), xmlAttr(groupTitle(room)))
	return true
}
func leaveMUCLocked(room string, state *mucRoom, s *session, code string) {
	nick, ok := state.occupants[s]
	if !ok {
		return
	}
	for peer := range state.occupants {
		if peer == s || Member(room, peer.acc.ID) {
			peer.send("%s", mucPresenceXML(room, nick, s, peer, true, code))
		}
	}
	delete(state.occupants, s)
}
func leaveMUC(s *session) {
	for room, state := range mucSnapshot() {
		state.Lock()
		leaveMUCLocked(room, state, s, "")
		state.Unlock()
	}
}
func pruneMUC() {
	for room, state := range mucSnapshot() {
		state.Lock()
		for s := range state.occupants {
			if !Member(room, s.acc.ID) {
				leaveMUCLocked(room, state, s, "321")
			}
		}
		state.Unlock()
	}
}
func (s *session) mucMessage(st stanza) bool {
	room, ok := mucTarget(st.To)
	if !ok {
		return false
	}
	if st.Type == "error" {
		return true
	}
	if st.Type != "groupchat" || strings.Contains(st.To, "/") {
		s.mucError(st, "message", "feature-not-implemented", "Send a groupchat message to the group address")
		return true
	}
	state := mucState(room, false)
	nick := ""
	if state != nil {
		state.Lock()
		nick = state.occupants[s]
		state.Unlock()
	}
	if nick == "" || !Member(room, s.acc.ID) {
		s.mucError(st, "message", "forbidden", "Join the group before sending")
		return true
	}
	g, err := group.Read(groupID(room), s.acc.ID)
	if err != nil {
		s.mucError(st, "message", "item-not-found", "Group not found")
		return true
	}
	m := RoomMessage{UserID: s.acc.ID, Content: strings.TrimSpace(st.Body), Timestamp: time.Now().UTC(), WireID: st.ID, Nick: nick}
	raw, encrypted := childElement(st.Inner, "", "encrypted")
	if encrypted {
		if !g.Encrypted {
			s.mucError(st, "message", "not-acceptable", "This group uses web and XMPP chat. Create an encrypted group to use OMEMO.")
			return true
		}
		var x omemoEnvelope
		if len(raw) > 192*1024 || xml.Unmarshal(raw, &x) != nil {
			s.mucError(st, "message", "bad-request", "Invalid OMEMO envelope")
			return true
		}
		if _, err := x.message(s.bare()); err != nil {
			s.mucError(st, "message", "bad-request", "Invalid OMEMO envelope")
			return true
		}
		m.OMEMO, err = marshalOMEMO(x)
		if err != nil {
			s.mucError(st, "message", "bad-request", "Invalid OMEMO envelope")
			return true
		}
		m.Content = "Encrypted message — open in your XMPP client."
	} else if g.Encrypted {
		s.mucError(st, "message", "not-acceptable", "This group requires OMEMO encryption; plaintext was not sent.")
		return true
	}
	if m.Content == "" {
		return true
	}
	if len(m.Content) > 65536 || len(m.WireID) > 256 {
		s.mucError(st, "message", "resource-constraint", "Message is too large")
		return true
	}
	if m.WireID == "" {
		m.WireID = newID()
	}
	r := getOrCreateRoom(room)
	if r == nil {
		s.mucError(st, "message", "item-not-found", "Group not found")
		return true
	}
	if err := post(context.Background(), r, m); err != nil {
		s.mucError(st, "message", "internal-server-error", "Message could not be saved")
		return true
	}
	return true
}
func mucMessageXML(room string, m RoomMessage, s *session, history bool) string {
	nick := m.Nick
	if nick == "" {
		nick = m.UserID
	}
	payload := `<body>` + xmlAttr(m.Content) + `</body>`
	if m.OMEMO != "" {
		payload = m.OMEMO + `<encryption xmlns='urn:xmpp:eme:0' namespace='` + nsOMEMO + `' name='OMEMO'/>`
	}
	if history {
		payload += `<delay xmlns='` + nsDelay + `' stamp='` + m.Timestamp.UTC().Format(time.RFC3339Nano) + `' from='` + xmlAttr(mucJID(room)) + `'/>`
	}
	payload += `<stanza-id xmlns="urn:xmpp:sid:0" by="` + xmlAttr(mucJID(room)) + `" id="` + xmlAttr(m.ID) + `"/>`
	payload += `<x xmlns='` + nsMUCUser + `'><item jid='` + xmlAttr(m.UserID+"@"+Domain()) + `'/></x>`
	return fmt.Sprintf(`<message type='groupchat' id='%s' from='%s/%s' to='%s'>%s</message>`, xmlAttr(m.WireID), xmlAttr(mucJID(room)), xmlAttr(nick), xmlAttr(s.jid()), payload)
}
func broadcastMUC(room string, m RoomMessage) {
	if !isGroup(room) || m.System {
		return
	}
	state := mucState(room, false)
	if state == nil {
		return
	}
	state.Lock()
	defer state.Unlock()
	for s := range state.occupants {
		if Member(room, s.acc.ID) {
			s.send("%s", mucMessageXML(room, m, s, false))
		}
	}
}
