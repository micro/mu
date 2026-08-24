package auth

// The public keys an account may connect with.
//
// The same shape as passkeys, one file along: a public key somebody registers,
// which proves who they are without a secret ever crossing the wire. The
// difference is only which protocol asks — WebAuthn for a browser, SSH for a
// terminal — and that is the argument for putting it here rather than in the
// service that happens to answer on port 22. Credentials belong with
// credentials; see passkey.go, which this is modelled on line for line.
//
// # What is stored
//
// The key in its authorized_keys form and its fingerprint. The fingerprint is
// what a lookup matches on, because that is what an SSH server has in hand at
// authentication time and comparing whole key blobs by string is how a
// trailing comment or a re-exported key stops matching itself.
//
// Nothing here parses SSH wire format. Whoever registers a key has already
// parsed it — the fingerprint arrives computed — which keeps this package free
// of a protocol it has no other business knowing.

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// The three ways registering a key can fail, said the way somebody reading a
// form would want to hear them.
var (
	errBadKey   = errors.New("that does not look like a public key")
	errKeyTaken = errors.New("another account here has already registered that key")
	errNoKey    = errors.New("no such key on this account")
)

// SSHKey is one public key somebody may connect with.
type SSHKey struct {
	Account string    `json:"account"`
	Name    string    `json:"name"`        // what they called it: "laptop"
	Key     string    `json:"key"`         // the authorized_keys line, without the comment
	Print   string    `json:"fingerprint"` // SHA256:… as ssh-keygen -lf prints it
	Added   time.Time `json:"added"`
	Used    time.Time `json:"used,omitempty"`
}

var (
	sshMutex sync.RWMutex
	sshKeys  map[string]*SSHKey // fingerprint → key
)

// Read at init, like passkeys one file along. Nothing has to remember to call
// it, which is the point: a credential store that loads only if somebody wired
// it up is a credential store that is empty on the instance where they forgot.
func init() {
	sshKeys = map[string]*SSHKey{}
	if b, err := data.LoadFile("sshkeys.json"); err == nil {
		json.Unmarshal(b, &sshKeys) //nolint:errcheck
	}
}

// AddSSHKey registers a key for an account.
//
// The fingerprint is the identity: registering the same key twice updates the
// name rather than making a second row that can never be told apart.
func AddSSHKey(accountID, name, key, print string) error {
	accountID = strings.ToLower(strings.TrimSpace(accountID))
	key, print = strings.TrimSpace(key), strings.TrimSpace(print)
	if accountID == "" || key == "" || print == "" {
		return errBadKey
	}

	sshMutex.Lock()
	if sshKeys == nil {
		sshKeys = map[string]*SSHKey{}
	}
	// A key already registered to somebody else is refused rather than moved.
	// Two accounts sharing one key means neither can be held to what was done
	// with it, and silently reassigning it would take the first account's
	// access away without telling them.
	if existing, ok := sshKeys[print]; ok && existing.Account != accountID {
		sshMutex.Unlock()
		return errKeyTaken
	}
	sshKeys[print] = &SSHKey{
		Account: accountID,
		Name:    strings.TrimSpace(name),
		Key:     key,
		Print:   print,
		Added:   time.Now(),
	}
	b, err := json.Marshal(sshKeys)
	sshMutex.Unlock()

	if err != nil {
		return err
	}
	return data.SaveFile("sshkeys.json", string(b))
}

// SSHKeys lists an account's keys, oldest first.
func SSHKeys(accountID string) []*SSHKey {
	accountID = strings.ToLower(strings.TrimSpace(accountID))
	sshMutex.RLock()
	defer sshMutex.RUnlock()

	var out []*SSHKey
	for _, k := range sshKeys {
		if k.Account == accountID {
			out = append(out, k)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Added.Before(out[j-1].Added); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// AccountForSSHKey is whose key this is, or "" for one nobody registered.
//
// It also records that the key was used, which is the only way somebody can
// tell a key they have forgotten about from one that is live — the question
// you ask when deciding whether it is safe to remove.
func AccountForSSHKey(print string) string {
	print = strings.TrimSpace(print)
	if print == "" {
		return ""
	}

	sshMutex.Lock()
	k := sshKeys[print]
	if k == nil {
		sshMutex.Unlock()
		return ""
	}
	who := k.Account
	k.Used = time.Now()
	b, err := json.Marshal(sshKeys)
	sshMutex.Unlock()

	// Written on use, and a failure here is not a reason to refuse the
	// connection: the fact that matters — this key is theirs — is already
	// known, and losing a timestamp is not worth locking somebody out.
	if err == nil {
		data.SaveFile("sshkeys.json", string(b)) //nolint:errcheck
	}
	return who
}

// RemoveSSHKey takes a key away, and refuses to remove somebody else's.
func RemoveSSHKey(accountID, print string) error {
	accountID = strings.ToLower(strings.TrimSpace(accountID))

	sshMutex.Lock()
	k := sshKeys[print]
	if k == nil || k.Account != accountID {
		sshMutex.Unlock()
		return errNoKey
	}
	delete(sshKeys, print)
	b, err := json.Marshal(sshKeys)
	sshMutex.Unlock()

	if err != nil {
		return err
	}
	return data.SaveFile("sshkeys.json", string(b))
}

// DeleteSSHKeysFor removes every key an account had. Called when the account
// is deleted: a credential that outlives its account is a way in to nothing,
// until somebody creates an account with the same name.
func DeleteSSHKeysFor(accountID string) {
	accountID = strings.ToLower(strings.TrimSpace(accountID))

	sshMutex.Lock()
	for print, k := range sshKeys {
		if k.Account == accountID {
			delete(sshKeys, print)
		}
	}
	b, err := json.Marshal(sshKeys)
	sshMutex.Unlock()

	if err == nil {
		data.SaveFile("sshkeys.json", string(b)) //nolint:errcheck
	}
}
