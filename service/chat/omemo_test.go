package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	omemo "github.com/jim-ww/omemo-go"
	"github.com/jim-ww/omemo-go/memstore"
	"mu/internal/auth"
	"mu/internal/data"
)

type omemoCarrier struct{ bytes.Buffer }

func (c *omemoCarrier) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *omemoCarrier) writeStanza(s string) error       { _, err := c.WriteString(s); return err }
func (c *omemoCarrier) SetReadDeadline(time.Time) error  { return nil }
func (c *omemoCarrier) SetWriteDeadline(time.Time) error { return nil }
func (c *omemoCarrier) Close() error                     { return nil }
func (c *omemoCarrier) framed() bool                     { return false }

type peerOMEMOTransport struct{}

func (peerOMEMOTransport) FetchDeviceList(_ context.Context, jid string) (omemo.DeviceList, error) {
	s, e := loadOMEMO()
	if e != nil {
		return omemo.DeviceList{}, e
	}
	return omemo.DeviceList{JID: jid, Devices: s.Lists[jid]}, nil
}
func (peerOMEMOTransport) PublishDeviceList(context.Context, omemo.DeviceList) error { return nil }
func (peerOMEMOTransport) FetchBundle(ctx context.Context, d omemo.Device) (omemo.Bundle, error) {
	s, e := loadOMEMO()
	if e != nil {
		return omemo.Bundle{}, e
	}
	if d == s.Device {
		m, e := s.manager()
		if e != nil {
			return omemo.Bundle{}, e
		}
		return m.Bundle(ctx)
	}
	return s.FetchBundle(ctx, d)
}
func (peerOMEMOTransport) PublishBundle(context.Context, omemo.Bundle) error { return nil }

func omemoFixture(t *testing.T) (*session, *omemoCarrier) {
	t.Helper()
	before, err := data.LoadFile(omemoStoreKey)
	existed := err == nil
	data.DeleteFile(omemoStoreKey)
	t.Cleanup(func() {
		if existed {
			data.SaveFile(omemoStoreKey, string(before))
		} else {
			data.DeleteFile(omemoStoreKey)
		}
	})
	a := &auth.Account{ID: "omemo-test", Name: "OMEMO Test"}
	auth.SetAccountForTest(a)
	c := &omemoCarrier{}
	s := &session{acc: a, conn: c, resource: "phone"}
	join(s)
	t.Cleanup(func() { leave(s); Forget(a.ID); auth.RemoveAccountForTest(a.ID) })
	return s, c
}
func publishPeer(t *testing.T, s *session, c *omemoCarrier, b omemo.Bundle) {
	t.Helper()
	payload, _ := marshalOMEMO(bundleXML(b))
	for _, v := range []struct{ node, payload string }{{omemoBundles + fmt.Sprint(b.Device.ID), payload}, {omemoDevices, fmt.Sprintf(`<list xmlns='%s'><device id='%d'/></list>`, nsOMEMO, b.Device.ID)}} {
		c.Reset()
		s.iq(stanza{ID: "publish", Type: "set", Inner: []byte(`<pubsub xmlns='` + nsPubsub + `'><publish node='` + v.node + `'><item id='current'>` + v.payload + `</item></publish></pubsub>`)})
		if !strings.Contains(c.String(), "type='result'") {
			t.Fatalf("publish failed: %s", c.String())
		}
	}
}
func newOMEMOPeer(t *testing.T, s *session, c *omemoCarrier, id omemo.DeviceID) *omemo.Manager {
	t.Helper()
	ctx := context.Background()
	store := memstore.New()
	if err := omemo.InitIdentity(ctx, store, s.bare(), id, omemo.ProtocolV1); err != nil {
		t.Fatal(err)
	}
	m, err := omemo.NewManager(ctx, store, peerOMEMOTransport{}, omemo.ProtocolV1, omemo.WithTrustResolver(func(context.Context, omemo.Device, []byte) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Bundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publishPeer(t, s, c, b)
	return m
}
func sendOMEMOPeer(t *testing.T, s *session, p *omemo.Manager, text string) string {
	t.Helper()
	m, fail, err := p.EncryptMessage(context.Background(), agentJID(), []byte(text))
	if err != nil || len(fail) > 0 {
		t.Fatalf("encrypt: %v %v", err, fail)
	}
	wire, err := envelopeXML(m)
	if err != nil {
		t.Fatal(err)
	}
	s.message(stanza{ID: newID(), Type: "chat", To: agentJID(), Body: "This message is encrypted", Inner: []byte(wire)})
	return wire
}
func decryptOMEMOPeer(t *testing.T, p *omemo.Manager, wire string) string {
	t.Helper()
	var x omemoEnvelope
	if xml.Unmarshal([]byte(wire), &x) != nil {
		t.Fatal("invalid reply XML")
	}
	m, err := x.message(agentJID())
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.DecryptMessage(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestOMEMOConversationRoundTripRestartArchiveAndReplay(t *testing.T) {
	s, c := omemoFixture(t)
	p := newOMEMOPeer(t, s, c, 42)
	fingerprint, err := OMEMOFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	wire := sendOMEMOPeer(t, s, p, "private question")
	rows := Everything(s.acc.ID, 20)
	if len(rows) != 1 || rows[0].Text != "private question" || rows[0].OMEMO == "" {
		t.Fatalf("not decrypted and saved: %#v / %s", rows, c.String())
	}
	// Same ciphertext retried must not create another agent request.
	s.message(stanza{ID: "retry", To: agentJID(), Inner: []byte(wire)})
	if len(Everything(s.acc.ID, 20)) != 1 {
		t.Fatal("retry was executed twice")
	}
	c.Reset()
	if !SayTo(s.acc.ID, agentJID(), "private reply") {
		t.Fatalf("reply failed: %s", c.String())
	}
	if strings.Contains(c.String(), "private reply") || strings.Contains(c.String(), "<body>") {
		t.Fatal("plain reply leaked on XMPP")
	}
	rows = Everything(s.acc.ID, 20)
	if got := decryptOMEMOPeer(t, p, rows[1].OMEMO); got != "private reply" {
		t.Fatal(got)
	}
	c.Reset()
	s.sendArchived("history", rows[1])
	if strings.Contains(c.String(), "private reply") || !strings.Contains(c.String(), nsOMEMO) {
		t.Fatal("archive sent cleartext")
	}
	// All operations reload persistent state: no in-memory manager survives here.
	second, err := OMEMOFingerprint()
	if err != nil || second != fingerprint {
		t.Fatal("fingerprint changed")
	}
	sendOMEMOPeer(t, s, p, "after restart")
	rows = Everything(s.acc.ID, 20)
	if rows[len(rows)-1].Text != "after restart" {
		t.Fatalf("restart failed: %s", c.String())
	}
	c.Reset()
	s.message(stanza{ID: "plain", To: agentJID(), Body: "do not downgrade"})
	if !strings.Contains(c.String(), "not-acceptable") {
		t.Fatal("plain downgrade accepted")
	}
}
func TestOMEMORejectsForeignPublicationAndChangedIdentity(t *testing.T) {
	s, c := omemoFixture(t)
	p := newOMEMOPeer(t, s, c, 42)
	sendOMEMOPeer(t, s, p, "hello")
	before, _ := data.LoadFile(omemoStoreKey)
	c.Reset()
	s.iq(stanza{ID: "foreign", Type: "set", To: agentJID(), Inner: []byte(`<pubsub xmlns='` + nsPubsub + `'><publish node='` + omemoDevices + `'><item><list xmlns='` + nsOMEMO + `'><device id='99'/></list></item></publish></pubsub>`)})
	if !strings.Contains(c.String(), "forbidden") {
		t.Fatal("foreign key publication accepted")
	}
	after, _ := data.LoadFile(omemoStoreKey)
	if !bytes.Equal(before, after) {
		t.Fatal("foreign publication changed state")
	}
	ctx := context.Background()
	store := memstore.New()
	omemo.InitIdentity(ctx, store, s.bare(), 42, omemo.ProtocolV1)
	other, _ := omemo.NewManager(ctx, store, peerOMEMOTransport{}, omemo.ProtocolV1)
	bundle, _ := other.Bundle(ctx)
	payload, _ := marshalOMEMO(bundleXML(bundle))
	c.Reset()
	s.iq(stanza{ID: "changed", Type: "set", Inner: []byte(`<pubsub xmlns='` + nsPubsub + `'><publish node='` + omemoBundles + `42'><item>` + payload + `</item></publish></pubsub>`)})
	if !strings.Contains(c.String(), "not-authorized") {
		t.Fatalf("changed identity accepted: %s", c.String())
	}
	// If the only recipient device disappears, replies cannot fall back to plaintext.
	state, _ := loadOMEMO()
	state.Lists[s.bare()] = nil
	data.SaveJSON(omemoStoreKey, state)
	c.Reset()
	if SayTo(s.acc.ID, agentJID(), "must stay private") || strings.Contains(c.String(), "must stay private") {
		t.Fatal("encryption failure fell back to plain text")
	}
	Forget(s.acc.ID)
	state, _ = loadOMEMO()
	if state.Enabled[s.bare()] || len(state.Sessions) > 0 || len(state.Bundles) > 0 {
		b, _ := json.Marshal(state.Lists)
		t.Fatalf("account deletion left peer state: %s", b)
	}
}

func TestOMEMOReplyReachesEveryDevice(t *testing.T) {
	s, c := omemoFixture(t)
	phone := newOMEMOPeer(t, s, c, 42)
	tablet := newOMEMOPeer(t, s, c, 43)
	c.Reset()
	s.iq(stanza{ID: "both", Type: "set", Inner: []byte(`<pubsub xmlns='` + nsPubsub + `'><publish node='` + omemoDevices + `'><item><list xmlns='` + nsOMEMO + `'><device id='42'/><device id='43'/></list></item></publish></pubsub>`)})
	if !strings.Contains(c.String(), "type='result'") {
		t.Fatal(c.String())
	}
	sendOMEMOPeer(t, s, phone, "both devices")
	c.Reset()
	if !SayTo(s.acc.ID, agentJID(), "shared encrypted reply") {
		t.Fatal(c.String())
	}
	rows := Everything(s.acc.ID, 20)
	wire := rows[len(rows)-1].OMEMO
	for _, peer := range []*omemo.Manager{phone, tablet} {
		if got := decryptOMEMOPeer(t, peer, wire); got != "shared encrypted reply" {
			t.Fatal(got)
		}
	}
	// Corrupt or absent state after establishing encryption must fail closed.
	for _, corrupt := range []string{"{}", ""} {
		if err := data.SaveFile(omemoStoreKey, corrupt); err != nil {
			t.Fatal(err)
		}
		c.Reset()
		if SayTo(s.acc.ID, agentJID(), "never cleartext") || c.Len() != 0 {
			t.Fatal("corrupt state allowed reply")
		}
	}
	if err := data.DeleteFile(omemoStoreKey); err != nil {
		t.Fatal(err)
	}
	if SayTo(s.acc.ID, agentJID(), "never cleartext") {
		t.Fatal("missing state allowed reply")
	}
}
