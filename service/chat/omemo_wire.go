package chat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	omemo "github.com/jim-ww/omemo-go"
	"mu/internal/data"
)

const (
	nsOMEMO      = "eu.siacs.conversations.axolotl"
	nsPubsub     = "http://jabber.org/protocol/pubsub"
	omemoDevices = nsOMEMO + ".devicelist"
	omemoBundles = nsOMEMO + ".bundles:"
)

type omemoDeviceXML struct {
	ID uint32 `xml:"id,attr"`
}
type omemoListXML struct {
	XMLName xml.Name         `xml:"eu.siacs.conversations.axolotl list"`
	Devices []omemoDeviceXML `xml:"device"`
}
type omemoKeyXML struct {
	ID   uint32 `xml:"preKeyId,attr"`
	Data string `xml:",chardata"`
}
type omemoSignedXML struct {
	ID   uint32 `xml:"signedPreKeyId,attr"`
	Data string `xml:",chardata"`
}
type omemoBundleXML struct {
	XMLName   xml.Name       `xml:"eu.siacs.conversations.axolotl bundle"`
	Signed    omemoSignedXML `xml:"signedPreKeyPublic"`
	Signature string         `xml:"signedPreKeySignature"`
	Identity  string         `xml:"identityKey"`
	Prekeys   []omemoKeyXML  `xml:"prekeys>preKeyPublic"`
}
type omemoRecipientXML struct {
	ID     uint32 `xml:"rid,attr"`
	Prekey bool   `xml:"prekey,attr,omitempty"`
	Data   string `xml:",chardata"`
}
type omemoEnvelope struct {
	XMLName xml.Name `xml:"eu.siacs.conversations.axolotl encrypted"`
	Header  struct {
		ID   uint32              `xml:"sid,attr"`
		Keys []omemoRecipientXML `xml:"key"`
		IV   string              `xml:"iv"`
	} `xml:"header"`
	Payload string `xml:"payload,omitempty"`
}

func b64(b []byte) string        { return base64.StdEncoding.EncodeToString(b) }
func frameOMEMO(k []byte) string { return b64(append([]byte{5}, k...)) }
func parseOMEMOKey(s string) ([]byte, error) {
	b, e := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if e != nil || len(b) != 33 || b[0] != 5 {
		return nil, fmt.Errorf("invalid OMEMO public key")
	}
	return b[1:], nil
}
func bundleXML(b omemo.Bundle) omemoBundleXML {
	x := omemoBundleXML{Signed: omemoSignedXML{b.SignedPreKey.ID, frameOMEMO(b.SignedPreKey.Public)}, Signature: b64(b.SignedPreKey.Signature), Identity: frameOMEMO(b.IdentityKey)}
	for _, k := range b.PreKeys {
		x.Prekeys = append(x.Prekeys, omemoKeyXML{k.ID, frameOMEMO(k.Public)})
	}
	return x
}
func (x omemoBundleXML) bundle(dev omemo.Device) (omemo.Bundle, error) {
	b := omemo.Bundle{Device: dev}
	var err error
	if b.IdentityKey, err = parseOMEMOKey(x.Identity); err != nil {
		return b, err
	}
	if b.SignedPreKey.Public, err = parseOMEMOKey(x.Signed.Data); err != nil {
		return b, err
	}
	b.SignedPreKey.ID = x.Signed.ID
	if b.SignedPreKey.Signature, err = base64.StdEncoding.DecodeString(strings.TrimSpace(x.Signature)); err != nil || len(b.SignedPreKey.Signature) != 64 {
		return b, fmt.Errorf("invalid OMEMO signature")
	}
	if len(x.Prekeys) == 0 || len(x.Prekeys) > 200 {
		return b, fmt.Errorf("invalid OMEMO prekey count")
	}
	seen := map[uint32]bool{}
	for _, k := range x.Prekeys {
		key, err := parseOMEMOKey(k.Data)
		if err != nil || seen[k.ID] {
			return b, fmt.Errorf("invalid OMEMO prekey")
		}
		seen[k.ID] = true
		b.PreKeys = append(b.PreKeys, omemo.PreKey{ID: k.ID, Public: key})
	}
	return b, nil
}
func marshalOMEMO(v any) (string, error) { b, e := xml.Marshal(v); return string(b), e }

// Parse direct child elements rather than matching untrusted XML substrings.
func childElement(inner []byte, namespace, name string) ([]byte, bool) {
	d := xml.NewDecoder(strings.NewReader("<root>" + string(inner) + "</root>"))
	depth := 0
	for {
		t, e := d.Token()
		if e == io.EOF {
			return nil, false
		}
		if e != nil {
			return nil, false
		}
		switch v := t.(type) {
		case xml.StartElement:
			if depth == 1 && v.Name.Local == name && (namespace == "" || v.Name.Space == namespace) {
				var body struct {
					Inner string `xml:",innerxml"`
				}
				if d.DecodeElement(&body, &v) != nil {
					return nil, false
				}
				var b bytes.Buffer
				en := xml.NewEncoder(&b)
				if en.EncodeToken(v) != nil {
					return nil, false
				}
				en.Flush()
				b.WriteString(body.Inner)
				en.EncodeToken(v.End())
				en.Flush()
				return b.Bytes(), true
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
}

type omemoPubsub struct {
	XMLName xml.Name    `xml:"http://jabber.org/protocol/pubsub pubsub"`
	Items   *omemoItems `xml:"items"`
	Publish *omemoItems `xml:"publish"`
}
type omemoItems struct {
	Node  string `xml:"node,attr"`
	Items []struct {
		ID    string `xml:"id,attr"`
		Inner []byte `xml:",innerxml"`
	} `xml:"item"`
}

func (s *session) omemoIQ(st stanza) bool {
	raw, ok := childElement(st.Inner, nsPubsub, "pubsub")
	if !ok {
		return false
	}
	target := strings.ToLower(bareOf(st.To))
	if target == "" || target == Domain() {
		target = s.bare()
	}
	var q omemoPubsub
	if len(raw) > 128*1024 || xml.Unmarshal(raw, &q) != nil {
		s.omemoIQError(st, target, "bad-request")
		return true
	}
	var changedDevices []omemo.DeviceID
	notifyDevices := false
	result, condition := func() (string, string) {
		omemoMu.Lock()
		defer omemoMu.Unlock()
		state, err := loadOMEMO()
		if err != nil {
			return "", "internal-server-error"
		}
		manager, err := state.manager()
		if err != nil {
			return "", "internal-server-error"
		}
		if err := state.maintain(manager); err != nil {
			return "", "internal-server-error"
		}
		if q.Publish != nil {
			if st.Type != "set" || q.Items != nil || target != s.bare() || len(q.Publish.Items) != 1 {
				return "", "forbidden"
			}
			item := q.Publish.Items[0]
			node := q.Publish.Node
			if item.ID != "" && item.ID != "current" {
				return "", "bad-request"
			}
			switch {
			case node == omemoDevices:
				var list omemoListXML
				if xml.Unmarshal(item.Inner, &list) != nil || len(list.Devices) > 100 {
					return "", "bad-request"
				}
				ids := []omemo.DeviceID{}
				seen := map[uint32]bool{}
				for _, d := range list.Devices {
					if d.ID == 0 || d.ID > 0x7fffffff || seen[d.ID] {
						return "", "bad-request"
					}
					seen[d.ID] = true
					ids = append(ids, omemo.DeviceID(d.ID))
				}
				state.Lists[target] = ids
			case strings.HasPrefix(node, omemoBundles):
				id, err := strconv.ParseUint(strings.TrimPrefix(node, omemoBundles), 10, 31)
				if err != nil || id == 0 {
					return "", "bad-request"
				}
				dev := omemo.Device{JID: target, ID: omemo.DeviceID(id)}
				var x omemoBundleXML
				if xml.Unmarshal(item.Inner, &x) != nil {
					return "", "bad-request"
				}
				b, err := x.bundle(dev)
				if err != nil {
					return "", "bad-request"
				}
				if old, ok := state.Bundles[deviceKey(dev)]; ok && !bytes.Equal(old.IdentityKey, b.IdentityKey) {
					return "", "not-authorized"
				}
				if old := state.RemoteKeys[deviceKey(dev)]; len(old) > 0 && !bytes.Equal(old, b.IdentityKey) {
					return "", "not-authorized"
				}
				n := 0
				for _, b := range state.Bundles {
					if b.Device.JID == target {
						n++
					}
				}
				if _, ok := state.Bundles[deviceKey(dev)]; !ok && n >= 100 {
					return "", "resource-constraint"
				}
				state.Bundles[deviceKey(dev)] = b
			default:
				return "", "feature-not-implemented"
			}
			if err := data.SaveJSON(omemoStoreKey, state); err != nil {
				return "", "internal-server-error"
			}
			if node == omemoDevices {
				changedDevices = state.Lists[target]
				notifyDevices = true
			}
			return "", ""
		}
		if st.Type != "get" || q.Items == nil {
			return "", "bad-request"
		}
		node := q.Items.Node
		var payload string
		switch {
		case node == omemoDevices:
			list := omemoListXML{}
			for _, id := range state.Lists[target] {
				list.Devices = append(list.Devices, omemoDeviceXML{uint32(id)})
			}
			payload, err = marshalOMEMO(list)
		case strings.HasPrefix(node, omemoBundles):
			id, e := strconv.ParseUint(strings.TrimPrefix(node, omemoBundles), 10, 31)
			if e != nil || id == 0 {
				return "", "bad-request"
			}
			var b omemo.Bundle
			if target == agentJID() && omemo.DeviceID(id) == state.Device.ID {
				b, err = manager.Bundle(context.Background())
			} else {
				b, err = state.FetchBundle(context.Background(), omemo.Device{JID: target, ID: omemo.DeviceID(id)})
			}
			if err != nil {
				return "", "item-not-found"
			}
			payload, err = marshalOMEMO(bundleXML(b))
		default:
			return "", "item-not-found"
		}
		if err != nil {
			return "", "internal-server-error"
		}
		// Persist newly generated/rotated keys before disclosing a public bundle.
		if err := data.SaveJSON(omemoStoreKey, state); err != nil {
			return "", "internal-server-error"
		}
		return `<pubsub xmlns='` + nsPubsub + `'><items node='` + xmlAttr(node) + `'><item id='current'>` + payload + `</item></items></pubsub>`, ""
	}()
	if condition != "" {
		s.omemoIQError(st, target, condition)
	} else {
		s.send(`<iq type='result' id='%s' from='%s' to='%s'>%s</iq>`, xmlAttr(st.ID), xmlAttr(target), xmlAttr(s.jid()), result)
	}
	if condition == "" && notifyDevices {
		notifyOMEMODevices(target, changedDevices)
	}
	return true
}
func (s *session) omemoIQError(st stanza, from, condition string) {
	s.send(`<iq type='error' id='%s' from='%s' to='%s'><error type='cancel'><%s xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/></error></iq>`, xmlAttr(st.ID), xmlAttr(from), xmlAttr(s.jid()), condition)
}
func notifyOMEMODevices(jid string, ids []omemo.DeviceID) {
	list := omemoListXML{}
	for _, id := range ids {
		list.Devices = append(list.Devices, omemoDeviceXML{uint32(id)})
	}
	payload, _ := marshalOMEMO(list)
	for _, s := range sessionsFor(jid) {
		s.send(`<message from='%s' to='%s'><event xmlns='http://jabber.org/protocol/pubsub#event'><items node='%s'><item id='current'>%s</item></items></event></message>`, xmlAttr(jid), xmlAttr(s.jid()), omemoDevices, payload)
	}
}

// OMEMOFingerprint is the stable assistant identity to compare in Conversations.
func OMEMOFingerprint() (string, error) {
	omemoMu.Lock()
	defer omemoMu.Unlock()
	state, err := loadOMEMO()
	if err != nil {
		return "", err
	}
	m, err := state.manager()
	if err != nil {
		return "", err
	}
	b, err := m.Bundle(context.Background())
	if err != nil {
		return "", err
	}
	if err := data.SaveJSON(omemoStoreKey, state); err != nil {
		return "", err
	}
	return hex.EncodeToString(b.IdentityKey), nil
}
