package chat

// OMEMO's crypto state is committed with the message that advances it. All
// operations use a private snapshot under omemoMu: failed authentication or
// persistence must not consume a prekey or advance a live ratchet.
import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	omemo "github.com/jim-ww/omemo-go"
	"mu/internal/data"
)

const omemoStoreKey = "chat/omemo.json"

var omemoMu sync.Mutex

type omemoState struct {
	Identity      []byte
	Device        omemo.Device
	Signed        omemo.SignedPreKeyRecord
	Stale         *omemo.SignedPreKeyRecord
	Rotated       time.Time
	Prekeys       map[uint32]omemo.PreKeyRecord
	Next          uint32
	Sessions      map[string][]byte
	TrustKeys     map[string]omemo.TrustState
	Lists         map[string][]omemo.DeviceID
	RemoteKeys    map[string][]byte
	Bundles       map[string]omemo.Bundle
	IncomingBases map[string][]byte
	Enabled       map[string]bool
}

func deviceKey(d omemo.Device) string { return d.JID + "/" + strconv.FormatUint(uint64(d.ID), 10) }
func loadOMEMO() (*omemoState, error) {
	s := &omemoState{Prekeys: map[uint32]omemo.PreKeyRecord{}, Sessions: map[string][]byte{}, TrustKeys: map[string]omemo.TrustState{}, Lists: map[string][]omemo.DeviceID{}, RemoteKeys: map[string][]byte{}, Bundles: map[string]omemo.Bundle{}, IncomingBases: map[string][]byte{}, Enabled: map[string]bool{}}
	b, err := data.LoadFile(omemoStoreKey)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, s); err != nil {
			return nil, err
		}
	}
	if len(s.Identity) == 0 {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("OMEMO identity missing from existing state")
		}
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		id := omemo.DeviceID(binary.BigEndian.Uint32(b[:]) & 0x7fffffff)
		if id == 0 {
			id = 1
		}
		if err := omemo.InitIdentity(context.Background(), s, agentJID(), id, omemo.ProtocolV1); err != nil {
			return nil, err
		}
		s.Rotated = time.Now().UTC()
	} else if len(s.Identity) != 32 || s.Device.ID == 0 || s.Device.JID != agentJID() || s.Signed.ID == 0 || s.Prekeys == nil || s.Sessions == nil || s.RemoteKeys == nil || s.Lists == nil || s.TrustKeys == nil || s.Bundles == nil || s.IncomingBases == nil || s.Enabled == nil {
		return nil, fmt.Errorf("invalid OMEMO state or changed XMPP domain")
	}
	s.Lists[agentJID()] = []omemo.DeviceID{s.Device.ID}
	return s, nil
}
func (s *omemoState) manager() (*omemo.Manager, error) {
	return omemo.NewManager(context.Background(), s, s, omemo.ProtocolV1, omemo.WithTrustResolver(func(_ context.Context, d omemo.Device, key []byte) error {
		// Only devices published by this authenticated local account are eligible.
		b, ok := s.Bundles[deviceKey(d)]
		if !ok || !bytes.Equal(b.IdentityKey, key) {
			return fmt.Errorf("device identity is not published by its owner")
		}
		return nil
	}))
}
func (s *omemoState) maintain(m *omemo.Manager) error {
	ctx := context.Background()
	if time.Since(s.Rotated) > 14*24*time.Hour {
		if err := m.RotateSignedPreKey(ctx); err != nil {
			return err
		}
		s.Rotated = time.Now().UTC()
	}
	if len(s.Prekeys) < omemo.MinPreKeyCount {
		return m.GenerateOneTimePreKeys(ctx, omemo.DefaultPreKeyCount-len(s.Prekeys))
	}
	return nil
}
func (s *omemoState) IdentityKeyPair(context.Context) ([]byte, error) { return s.Identity, nil }
func (s *omemoState) SetIdentityKeyPair(_ context.Context, k []byte) error {
	s.Identity = k
	return nil
}
func (s *omemoState) LocalDevice(context.Context) (omemo.Device, error) { return s.Device, nil }
func (s *omemoState) SetLocalDevice(_ context.Context, d omemo.Device) error {
	s.Device = d
	return nil
}
func (s *omemoState) CurrentSignedPreKey(context.Context) (omemo.SignedPreKeyRecord, error) {
	return s.Signed, nil
}
func (s *omemoState) StaleSignedPreKey(context.Context) (omemo.SignedPreKeyRecord, bool, error) {
	if s.Stale == nil {
		return omemo.SignedPreKeyRecord{}, false, nil
	}
	return *s.Stale, true, nil
}
func (s *omemoState) RotateSignedPreKey(_ context.Context, k omemo.SignedPreKeyRecord) error {
	if s.Signed.ID != 0 {
		old := s.Signed
		s.Stale = &old
	}
	s.Signed = k
	return nil
}
func (s *omemoState) PreKeyCount(context.Context) (int, error) { return len(s.Prekeys), nil }
func (s *omemoState) PreKeys(context.Context) ([]omemo.PreKeyRecord, error) {
	out := make([]omemo.PreKeyRecord, 0, len(s.Prekeys))
	for _, k := range s.Prekeys {
		out = append(out, k)
	}
	return out, nil
}
func (s *omemoState) NextPreKeyID(context.Context) (uint32, error) {
	if s.Next >= 0xffffff {
		return 0, fmt.Errorf("OMEMO prekey IDs exhausted")
	}
	s.Next++
	return s.Next, nil
}
func (s *omemoState) PutPreKeys(_ context.Context, ks []omemo.PreKeyRecord) error {
	for _, k := range ks {
		s.Prekeys[k.ID] = k
	}
	return nil
}
func (s *omemoState) ConsumePreKey(_ context.Context, id uint32) (omemo.PreKeyRecord, error) {
	k, ok := s.Prekeys[id]
	if !ok {
		return k, fmt.Errorf("unknown OMEMO prekey")
	}
	delete(s.Prekeys, id)
	return k, nil
}
func (s *omemoState) Session(_ context.Context, d omemo.Device) ([]byte, bool, error) {
	b, ok := s.Sessions[deviceKey(d)]
	return b, ok, nil
}
func (s *omemoState) PutSession(_ context.Context, d omemo.Device, b []byte) error {
	s.Sessions[deviceKey(d)] = b
	return nil
}
func (s *omemoState) DeleteSession(_ context.Context, d omemo.Device) error {
	delete(s.Sessions, deviceKey(d))
	return nil
}
func (s *omemoState) Trust(_ context.Context, k []byte) (omemo.TrustState, error) {
	return s.TrustKeys[hex.EncodeToString(k)], nil
}
func (s *omemoState) SetTrust(_ context.Context, k []byte, v omemo.TrustState) error {
	s.TrustKeys[hex.EncodeToString(k)] = v
	return nil
}
func (s *omemoState) Devices(_ context.Context, jid string) ([]omemo.DeviceID, error) {
	return s.Lists[jid], nil
}
func (s *omemoState) SetDevices(_ context.Context, jid string, ids []omemo.DeviceID) error {
	s.Lists[jid] = ids
	return nil
}
func (s *omemoState) RemoteIdentityKey(_ context.Context, d omemo.Device) ([]byte, bool, error) {
	k, ok := s.RemoteKeys[deviceKey(d)]
	return k, ok, nil
}
func (s *omemoState) PutRemoteIdentityKey(_ context.Context, d omemo.Device, k []byte) error {
	if old := s.RemoteKeys[deviceKey(d)]; len(old) > 0 && !bytes.Equal(old, k) {
		return fmt.Errorf("OMEMO identity changed; use a new device ID")
	}
	s.RemoteKeys[deviceKey(d)] = k
	return nil
}
func (s *omemoState) FetchDeviceList(_ context.Context, jid string) (omemo.DeviceList, error) {
	return omemo.DeviceList{JID: jid, Devices: s.Lists[jid]}, nil
}
func (s *omemoState) PublishDeviceList(_ context.Context, l omemo.DeviceList) error {
	s.Lists[l.JID] = l.Devices
	return nil
}
func (s *omemoState) FetchBundle(_ context.Context, d omemo.Device) (omemo.Bundle, error) {
	b, ok := s.Bundles[deviceKey(d)]
	if !ok {
		return b, fmt.Errorf("OMEMO bundle unavailable")
	}
	return b, nil
}
func (s *omemoState) PublishBundle(_ context.Context, b omemo.Bundle) error {
	s.Bundles[deviceKey(b.Device)] = b
	return nil
}

var _ omemo.Store = (*omemoState)(nil)
var _ omemo.Transport = (*omemoState)(nil)
