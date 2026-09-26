package chat

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/group"
	"mu/internal/service"
)

type groupCarrier struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (c *groupCarrier) Read([]byte) (int, error) { return 0, io.EOF }
func (c *groupCarrier) writeStanza(s string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, e := c.b.WriteString(s)
	return e
}
func (c *groupCarrier) SetReadDeadline(time.Time) error  { return nil }
func (c *groupCarrier) SetWriteDeadline(time.Time) error { return nil }
func (c *groupCarrier) Close() error                     { return nil }
func (c *groupCarrier) framed() bool                     { return false }
func (c *groupCarrier) text() string                     { c.mu.Lock(); defer c.mu.Unlock(); return c.b.String() }
func (c *groupCarrier) reset()                           { c.mu.Lock(); defer c.mu.Unlock(); c.b.Reset() }
func groupFixture(t *testing.T, encrypted bool) (group.Group, *session, *groupCarrier, *session, *groupCarrier) {
	t.Helper()
	a := &auth.Account{ID: "muc-owner"}
	b := &auth.Account{ID: "muc-member"}
	auth.SetAccountForTest(a)
	auth.SetAccountForTest(b)
	g, err := group.Create(a.ID, "Private family", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if err := group.Change(g.ID, a.ID, "invite", b.ID, ""); err != nil {
		t.Fatal(err)
	}
	ac, bc := &groupCarrier{}, &groupCarrier{}
	sa, sb := &session{acc: a, resource: "phone", conn: ac}, &session{acc: b, resource: "phone", conn: bc}
	t.Cleanup(func() {
		leaveMUC(sa)
		leaveMUC(sb)
		group.Change(g.ID, a.ID, "delete", "", "")
		auth.RemoveAccountForTest(a.ID)
		auth.RemoveAccountForTest(b.ID)
	})
	return g, sa, ac, sb, bc
}
func TestGroupChatMembershipAcrossTransports(t *testing.T) {
	g, a, ac, b, bc := groupFixture(t, false)
	room := group.RoomPrefix + g.ID
	address := mucJID(room)
	if !Private(room) || Listable(room) {
		t.Fatal("group listed publicly")
	}
	b.presence(stanza{To: address + "/member"})
	if !strings.Contains(bc.text(), "registration-required") {
		t.Fatal("pending invite joined")
	}
	bc.reset()
	if err := group.Change(g.ID, a.acc.ID, "invite", b.acc.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := group.Change(g.ID, b.acc.ID, "accept", "", ""); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*session{a, b} {
		s.presence(stanza{To: address + "/" + s.acc.ID})
	}
	if !strings.Contains(bc.text(), "code='110'") || !strings.Contains(bc.text(), a.bare()) {
		t.Fatal("missing self presence or actual member JIDs")
	}
	r := getOrCreateRoom(room)
	if err := post(context.Background(), r, RoomMessage{UserID: a.acc.ID, Content: "web to phone", Timestamp: time.Now()}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(bc.text(), "web to phone") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(bc.text(), "web to phone") {
		t.Fatal("web message did not reach XMPP")
	}
	a.message(stanza{Type: "groupchat", ID: "phone-msg", To: address, Body: "phone to web"})
	r.mutex.RLock()
	history := append([]RoomMessage(nil), r.Messages...)
	r.mutex.RUnlock()
	if len(history) != 2 || history[1].Content != "phone to web" {
		t.Fatal("XMPP message missing from web history", history)
	}
	if len(loadRoomMessages(room)) != 2 {
		t.Fatal("group history was not durable")
	}
	ac.reset()
	a.iq(stanza{ID: "members", Type: "get", To: address, Inner: []byte(`<query xmlns='` + nsMUCAdmin + `'><item affiliation='member'/></query>`)})
	if !strings.Contains(ac.text(), b.bare()) {
		t.Fatal("member list unavailable for OMEMO")
	}
	if err := group.Change(g.ID, a.acc.ID, "remove", b.acc.ID, ""); err != nil {
		t.Fatal(err)
	}
	bc.reset()
	broadcastMUC(room, RoomMessage{UserID: a.acc.ID, Content: "after removal"})
	if strings.Contains(bc.text(), "after removal") {
		t.Fatal("removed member received broadcast")
	}
	var rsp MessagesResponse
	if err := (Server{}).Messages(service.WithAccount(context.Background(), b.acc.ID), &MessagesRequest{Room: room}, &rsp); err == nil {
		t.Fatal("removed member read tool history")
	}
	if err := post(context.Background(), r, RoomMessage{UserID: b.acc.ID, Content: "removed send"}); err == nil {
		t.Fatal("removed member posted")
	}
	pruneMUC()
	if !strings.Contains(bc.text(), "321") {
		t.Fatal("revoked member not disconnected")
	}
	sess, _ := auth.CreateSession(b.acc.ID)
	req := httptest.NewRequest("GET", "/chat?id="+room, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	Handler(w, req)
	if w.Code != 404 {
		t.Fatal("removed member opened web chat", w.Code)
	}
	if microDM(room, a.acc.ID) {
		t.Fatal("group started a private assistant conversation")
	}
}
func TestEncryptedGroupRejectsPlaintextAndRetainsOnlyEnvelope(t *testing.T) {
	g, a, _, b, bc := groupFixture(t, true)
	room := group.RoomPrefix + g.ID
	address := mucJID(room)
	if err := group.Change(g.ID, b.acc.ID, "accept", "", ""); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*session{a, b} {
		s.presence(stanza{To: address + "/" + s.acc.ID})
	}
	r := getOrCreateRoom(room)
	if err := post(context.Background(), r, RoomMessage{UserID: a.acc.ID, Content: "web plaintext"}); err == nil {
		t.Fatal("encrypted group accepted web plaintext")
	}
	// A valid opaque envelope is relayed; the server must not decrypt it or store the fallback body.
	var x omemoEnvelope
	x.Header.ID = 123
	x.Header.Keys = []omemoRecipientXML{{ID: 456, Data: "AQID"}}
	x.Payload = "BAUG"
	wire, _ := marshalOMEMO(x)
	a.message(stanza{Type: "groupchat", To: address, Body: "plaintext fallback must never persist", Inner: []byte(wire)})
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(bc.text(), "<encrypted") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(bc.text(), "<encrypted") {
		t.Fatal("envelope not delivered", bc.text())
	}
	saved, err := data.LoadFile("room_" + room + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "plaintext fallback") || strings.Contains(string(saved), "web plaintext") {
		t.Fatal("plaintext persisted in encrypted room")
	}
	m := loadRoomMessages(room)[0]
	var back omemoEnvelope
	if err := xml.Unmarshal([]byte(m.OMEMO), &back); err != nil || back.Payload != x.Payload {
		t.Fatal("encrypted history changed", err)
	}
}

func TestGroupOMEMORecipientDecryptsRelay(t *testing.T) {
	a, ac := omemoFixture(t)
	bacc := &auth.Account{ID: "muc-crypto-peer"}
	auth.SetAccountForTest(bacc)
	defer auth.RemoveAccountForTest(bacc.ID)
	bc := &omemoCarrier{}
	b := &session{acc: bacc, conn: bc, resource: "phone"}
	alice := newOMEMOPeer(t, a, ac, 8101)
	bob := newOMEMOPeer(t, b, bc, 8102)
	g, err := group.Create(a.acc.ID, "Encrypted round trip", true)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(g.ID, a.acc.ID, "delete", "", "")
	group.Change(g.ID, a.acc.ID, "invite", b.acc.ID, "")
	group.Change(g.ID, b.acc.ID, "accept", "", "")
	room := group.RoomPrefix + g.ID
	address := mucJID(room)
	a.presence(stanza{To: address + "/Alice"})
	b.presence(stanza{To: address + "/Bob"})
	defer leaveMUC(b)
	encrypted, fail, err := alice.EncryptMessage(context.Background(), b.bare(), []byte("Family secret only Bob can decrypt"))
	if err != nil || len(fail) > 0 {
		t.Fatal("could not encrypt", err, fail)
	}
	wire, err := envelopeXML(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	a.message(stanza{Type: "groupchat", ID: "encrypted-roundtrip", To: address, Inner: []byte(wire)})
	var delivered string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.writeMu.Lock()
		delivered = bc.String()
		b.writeMu.Unlock()
		if strings.Contains(delivered, "encrypted-roundtrip") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	dec := xml.NewDecoder(strings.NewReader(delivered))
	var envelope []byte
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		var st stanza
		if dec.DecodeElement(&st, &start) != nil {
			break
		}
		if st.ID == "encrypted-roundtrip" {
			envelope, _ = childElement(st.Inner, nsOMEMO, "encrypted")
		}
	}
	if len(envelope) == 0 {
		t.Fatal("no encrypted relay", delivered)
	}
	var x omemoEnvelope
	if err := xml.Unmarshal(envelope, &x); err != nil {
		t.Fatal(err)
	}
	message, err := x.message(a.bare())
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := bob.DecryptMessage(context.Background(), message)
	if err != nil || string(plaintext) != "Family secret only Bob can decrypt" {
		t.Fatal("recipient could not decrypt", err)
	}
	saved, _ := data.LoadFile("room_" + room + ".json")
	if strings.Contains(string(saved), string(plaintext)) {
		t.Fatal("server retained group plaintext")
	}
}

func TestGroupArchiveRetainsHistoryAndScopesPaging(t *testing.T) {
	g, a, ac, b, bc := groupFixture(t, false)
	room := group.RoomPrefix + g.ID
	address := mucJID(room)
	r := getOrCreateRoom(room)
	for i := 0; i < 25; i++ {
		if err := post(context.Background(), r, RoomMessage{UserID: a.acc.ID, Content: fmt.Sprintf("message %02d", i), Timestamp: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	saved := loadRoomMessages(room)
	if len(saved) != 25 {
		t.Fatal("group history silently truncated", len(saved))
	}
	query := `<query xmlns='` + nsMAM + `' queryid='page'><set xmlns='` + nsRSM + `'><max>2</max><before/></set></query>`
	ac.reset()
	a.iq(stanza{ID: "history", Type: "set", To: address, Inner: []byte(query)})
	if !strings.Contains(ac.text(), "message 24") || !strings.Contains(ac.text(), "message 23") || strings.Contains(ac.text(), "message 22") {
		t.Fatal("wrong archive page", ac.text())
	}
	bc.reset()
	b.iq(stanza{ID: "unauthorized", Type: "set", To: address, Inner: []byte(query)})
	if strings.Contains(bc.text(), "message 24") || !strings.Contains(bc.text(), "item-not-found") {
		t.Fatal("invitee read archive")
	}
	query = strings.Replace(query, "<before/>", "<before>"+saved[23].ID+"</before>", 1)
	ac.reset()
	a.iq(stanza{ID: "older", Type: "set", To: address, Inner: []byte(query)})
	if !strings.Contains(ac.text(), "message 22") || strings.Contains(ac.text(), "message 24") {
		t.Fatal("wrong cursor page", ac.text())
	}
}
