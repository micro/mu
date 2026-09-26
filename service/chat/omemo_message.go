package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	omemo "github.com/jim-ww/omemo-go"
	"github.com/jim-ww/xochimilco/legacysignal"
	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/event"
)

func (x omemoEnvelope) message(from string) (*omemo.EncryptedMessage, error) {
	if x.Header.ID == 0 || x.Header.ID > 0x7fffffff || len(x.Header.Keys) == 0 || len(x.Header.Keys) > 200 {
		return nil, fmt.Errorf("invalid OMEMO envelope")
	}
	m := &omemo.EncryptedMessage{Sender: omemo.Device{JID: from, ID: omemo.DeviceID(x.Header.ID)}}
	var err error
	if x.Payload != "" {
		m.Payload, err = base64.StdEncoding.DecodeString(strings.TrimSpace(x.Payload))
		if err != nil || len(m.Payload) > 65536 {
			return nil, fmt.Errorf("invalid OMEMO payload")
		}
	}
	if x.Header.IV != "" {
		m.IV, err = base64.StdEncoding.DecodeString(strings.TrimSpace(x.Header.IV))
		if err != nil {
			return nil, err
		}
	}
	seen := map[uint32]bool{}
	for _, k := range x.Header.Keys {
		if seen[k.ID] {
			return nil, fmt.Errorf("duplicate OMEMO recipient")
		}
		seen[k.ID] = true
		b, e := base64.StdEncoding.DecodeString(strings.TrimSpace(k.Data))
		if e != nil || len(b) > 4096 {
			return nil, fmt.Errorf("invalid OMEMO key")
		}
		key := omemo.RecipientKey{Device: omemo.DeviceID(k.ID), Data: b}
		if k.Prekey {
			key.KeyExchange = &omemo.KeyExchange{}
		}
		m.Keys = append(m.Keys, key)
	}
	return m, nil
}
func envelopeXML(m *omemo.EncryptedMessage) (string, error) {
	x := omemoEnvelope{}
	x.Header.ID = uint32(m.Sender.ID)
	if len(m.IV) > 0 {
		x.Header.IV = b64(m.IV)
	}
	if m.Payload != nil {
		x.Payload = b64(m.Payload)
	}
	for _, k := range m.Keys {
		x.Header.Keys = append(x.Header.Keys, omemoRecipientXML{ID: uint32(k.Device), Prekey: k.KeyExchange != nil, Data: b64(k.Data)})
	}
	return marshalOMEMO(x)
}

// Decrypt only for Micro's own endpoint. Unsupported encrypted messages never
// fall through to the ordinary body (which is often just a fallback notice).
func (s *session) omemoMessage(st stanza) bool {
	raw, encrypted := childElement(st.Inner, "", "encrypted")
	if !encrypted {
		return false
	}
	if st.Type == "error" {
		return true
	}
	if strings.ToLower(bareOf(st.To)) != agentJID() {
		s.omemoMessageError(st, "feature-not-implemented", "OMEMO is currently supported for your chat with "+agentJID()+".")
		return true
	}
	if len(raw) > 192*1024 {
		s.omemoMessageError(st, "resource-constraint", "Encrypted message is too large.")
		return true
	}
	var x omemoEnvelope
	if xml.Unmarshal(raw, &x) != nil {
		s.omemoMessageError(st, "feature-not-implemented", "This OMEMO version is not supported.")
		return true
	}
	if err := s.receiveOMEMO(st, x); err != nil {
		app.Log("chat", "OMEMO message refused for %s: %v", s.acc.ID, err)
		s.omemoMessageError(st, "not-acceptable", "Micro could not decrypt this message. Check its OMEMO fingerprint and your device keys; encryption was not disabled.")
	}
	return true
}
func (s *session) receiveOMEMO(st stanza, x omemoEnvelope) error {
	omemoMu.Lock()
	var acknowledgement string
	defer func() {
		omemoMu.Unlock()
		if acknowledgement != "" {
			deliverOMEMO(agentJID(), s.bare(), acknowledgement, "")
		}
	}()
	state, err := loadOMEMO()
	if err != nil {
		return err
	}
	m, err := state.manager()
	if err != nil {
		return err
	}
	msg, err := x.message(s.bare())
	if err != nil {
		return err
	}
	wire, err := marshalOMEMO(x)
	if err != nil {
		return err
	}
	// Client retries must not execute the assistant twice or consume a ratchet key.
	for _, old := range Everything(s.acc.ID, heldPerAccount) {
		if old.From == s.bare() && old.OMEMO == wire {
			return nil
		}
	}
	var ours *omemo.RecipientKey
	for i := range msg.Keys {
		if msg.Keys[i].Device == state.Device.ID {
			ours = &msg.Keys[i]
			break
		}
	}
	if ours == nil {
		return fmt.Errorf("message has no key for Micro")
	}
	wasPrekey := ours.KeyExchange != nil
	remote := deviceKey(msg.Sender)
	bundle, ok := state.Bundles[remote]
	if !ok {
		return fmt.Errorf("sender must publish its OMEMO bundle first")
	}
	if ours.KeyExchange != nil {
		pk, err := legacysignal.ParsePreKeyMessage(ours.Data)
		if err != nil {
			return err
		}
		if !bytes.Equal(bundle.IdentityKey, pk.IdentityKey) {
			return fmt.Errorf("sender identity differs from published device")
		}
		if old := state.RemoteKeys[remote]; len(old) > 0 && !bytes.Equal(old, pk.IdentityKey) {
			return fmt.Errorf("sender identity changed")
		}
		// libsignal clients keep wrapping in PreKeyWhisperMessage until receiving
		// an acknowledgement. Continue that session instead of consuming its OPK twice.
		if bytes.Equal(state.IncomingBases[remote], pk.BaseKey) {
			ours.KeyExchange = nil
			ours.Data = pk.Message
		} else {
			state.IncomingBases[remote] = pk.BaseKey
		}
	}
	plaintext, err := m.DecryptMessage(context.Background(), msg)
	if err != nil {
		return err
	}
	if err := state.maintain(m); err != nil {
		return err
	}
	state.Enabled[s.bare()] = true
	// Authenticated inbound device keys are pinned; changed keys are rejected.
	if key := state.RemoteKeys[remote]; !bytes.Equal(key, bundle.IdentityKey) {
		return fmt.Errorf("sender identity changed")
	}
	state.SetTrust(context.Background(), bundle.IdentityKey, omemo.TrustTrusted)
	if len(plaintext) == 0 {
		if !wasPrekey {
			return data.SaveJSON(omemoStoreKey, state)
		}
		// Complete Conversations' session warm-up with an encrypted acknowledgement.
		reply, failures, err := m.EncryptKeyTransport(context.Background(), s.bare())
		if err != nil || len(failures) > 0 {
			return fmt.Errorf("could not encrypt key acknowledgement: %v %v", err, failures)
		}
		out, err := envelopeXML(reply)
		if err != nil {
			return err
		}
		if err := data.SaveJSON(omemoStoreKey, state); err != nil {
			return err
		}
		acknowledgement = out
		return nil
	}
	if len(plaintext) > 65536 {
		return fmt.Errorf("message too large")
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = keepSaved(s.acc.ID, Said{Conv: xmppRoom(s.bare(), agentJID()), From: s.bare(), To: agentJID(), Text: string(plaintext), OMEMO: wire, WireID: st.ID}, map[string][]byte{omemoStoreKey: stateBytes}, event.ChatAddressed)
	return err
}
func (s *session) omemoMessageError(st stanza, condition, text string) {
	s.send(`<message type='error' id='%s' from='%s' to='%s'><error type='cancel'><%s xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/><text xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'>%s</text></error></message>`, xmlAttr(st.ID), xmlAttr(st.To), xmlAttr(s.jid()), condition, xmlText(text))
}
func deliverOMEMO(from, to, wire, id string) bool {
	delivered := false
	for _, s := range sessionsFor(to) {
		if s.send(`<message type='chat' id='%s' from='%s' to='%s'>%s<encryption xmlns='urn:xmpp:eme:0' namespace='%s' name='OMEMO'/><store xmlns='urn:xmpp:hints'/></message>`, xmlAttr(id), xmlAttr(from), xmlAttr(s.jid()), wire, nsOMEMO) == nil {
			delivered = true
		}
	}
	return delivered
}

// Once OMEMO is established, a failed encryption must never produce a plain reply.
func sayOMEMO(account, from, text string) (handled, delivered bool) {
	omemoMu.Lock()
	var committedWire, committedID, recipient string
	defer func() {
		omemoMu.Unlock()
		if committedWire != "" {
			delivered = deliverOMEMO(from, recipient, committedWire, committedID)
		}
	}()
	if _, err := data.LoadFile(omemoStoreKey); os.IsNotExist(err) {
		return hasOMEMOHistory(account), false
	}
	state, err := loadOMEMO()
	if err != nil {
		app.Log("chat", "OMEMO state unavailable: %v", err)
		return true, false
	}
	to := strings.ToLower(account) + "@" + Domain()
	if !state.Enabled[to] {
		return hasOMEMOHistory(account), false
	}
	if from != agentJID() {
		return true, false
	}
	m, err := state.manager()
	if err != nil {
		return true, false
	}
	message, failures, err := m.EncryptMessage(context.Background(), to, []byte(text))
	if err != nil || len(failures) > 0 {
		app.Log("chat", "could not encrypt OMEMO reply to %s: %v %v", account, err, failures)
		return true, false
	}
	wire, err := envelopeXML(message)
	if err != nil {
		return true, false
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return true, false
	}
	id, err := keepSaved(account, Said{Conv: xmppRoom(from, to), From: from, To: to, Text: text, OMEMO: wire}, map[string][]byte{omemoStoreKey: stateBytes}, "")
	if err != nil {
		app.Log("chat", "could not commit OMEMO reply: %v", err)
		return true, false
	}
	committedWire, committedID, recipient = wire, id, to
	return true, false
}
func omemoRequired(jid string) bool {
	omemoMu.Lock()
	defer omemoMu.Unlock()
	b, err := data.LoadFile(omemoStoreKey)
	if os.IsNotExist(err) {
		account, _ := splitJID(jid)
		return hasOMEMOHistory(account)
	}
	if err != nil {
		return true
	}
	var state omemoState
	if json.Unmarshal(b, &state) != nil {
		return true
	}
	account, _ := splitJID(jid)
	return state.Enabled[jid] || hasOMEMOHistory(account)
}
func forgetOMEMO(account string) {
	omemoMu.Lock()
	defer omemoMu.Unlock()
	if _, err := data.LoadFile(omemoStoreKey); os.IsNotExist(err) {
		return
	}
	state, err := loadOMEMO()
	if err != nil {
		app.Log("chat", "could not remove OMEMO state: %v", err)
		return
	}
	jid := strings.ToLower(account) + "@" + Domain()
	delete(state.Lists, jid)
	delete(state.Enabled, jid)
	for key, b := range state.Bundles {
		if b.Device.JID == jid {
			delete(state.Bundles, key)
		}
	}
	for key := range state.RemoteKeys {
		if strings.HasPrefix(key, jid+"/") {
			delete(state.RemoteKeys, key)
			delete(state.Sessions, key)
			delete(state.IncomingBases, key)
		}
	}
	// Trust is keyed by identity; keep only identities still associated with a device.
	remaining := map[string]bool{}
	for _, key := range state.RemoteKeys {
		remaining[hex.EncodeToString(key)] = true
	}
	for key := range state.TrustKeys {
		if !remaining[key] {
			delete(state.TrustKeys, key)
		}
	}
	if err := data.SaveJSON(omemoStoreKey, state); err != nil {
		app.Log("chat", "could not remove OMEMO state: %v", err)
	}
}

func hasOMEMOHistory(account string) bool {
	for _, m := range Everything(account, heldPerAccount) {
		if m.OMEMO != "" {
			return true
		}
	}
	return false
}
