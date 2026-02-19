package auth

import (
	"encoding/json"
	"errors"
	"time"

	"mu/data"

	"github.com/go-webauthn/webauthn/webauthn"
)

var passkeys = map[string]*Passkey{} // passkeyID -> Passkey

// Passkey stores a WebAuthn credential for an account
type Passkey struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Account    string             `json:"account"`
	Credential webauthn.Credential `json:"credential"`
	Created    time.Time          `json:"created"`
	LastUsed   time.Time          `json:"last_used"`
}

// WebAuthnUser implements the webauthn.User interface
type WebAuthnUser struct {
	account *Account
	creds   []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte {
	return []byte(u.account.ID)
}

func (u *WebAuthnUser) WebAuthnName() string {
	return u.account.ID
}

func (u *WebAuthnUser) WebAuthnDisplayName() string {
	return u.account.Name
}

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.creds
}

func init() {
	b, _ := data.LoadFile("passkeys.json")
	json.Unmarshal(b, &passkeys)
}

// NewWebAuthnUser creates a WebAuthnUser for the given account
func NewWebAuthnUser(acc *Account) *WebAuthnUser {
	var creds []webauthn.Credential
	for _, pk := range passkeys {
		if pk.Account == acc.ID {
			creds = append(creds, pk.Credential)
		}
	}
	return &WebAuthnUser{account: acc, creds: creds}
}

// FindUserByWebAuthnID looks up an account by its WebAuthn user handle
func FindUserByWebAuthnID(userHandle []byte) (*WebAuthnUser, error) {
	acc, err := GetAccount(string(userHandle))
	if err != nil {
		return nil, err
	}
	return NewWebAuthnUser(acc), nil
}

// SavePasskey stores a new passkey credential
func SavePasskey(pk *Passkey) error {
	mutex.Lock()
	defer mutex.Unlock()

	passkeys[pk.ID] = pk
	data.SaveJSON("passkeys.json", passkeys)
	return nil
}

// GetPasskeys returns all passkeys for an account
func GetPasskeys(accountID string) []*Passkey {
	mutex.Lock()
	defer mutex.Unlock()

	var result []*Passkey
	for _, pk := range passkeys {
		if pk.Account == accountID {
			result = append(result, pk)
		}
	}
	return result
}

// UpdatePasskeyUsage updates the sign count and last used time
func UpdatePasskeyUsage(credentialID []byte, signCount uint32) {
	mutex.Lock()
	defer mutex.Unlock()

	for _, pk := range passkeys {
		if string(pk.Credential.ID) == string(credentialID) {
			pk.Credential.Authenticator.SignCount = signCount
			pk.LastUsed = time.Now()
			data.SaveJSON("passkeys.json", passkeys)
			return
		}
	}
}

// DeletePasskey removes a passkey
func DeletePasskey(id, accountID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	pk, exists := passkeys[id]
	if !exists {
		return errors.New("passkey does not exist")
	}

	if pk.Account != accountID {
		return errors.New("unauthorized")
	}

	delete(passkeys, id)
	data.SaveJSON("passkeys.json", passkeys)
	return nil
}
