package auth

import (
	"errors"
	"mu/internal/data"
)

// DisableGoogleSignIn preserves the verified address and independent service grants.
func DisableGoogleSignIn(id string) error {
	mutex.Lock()
	defer mutex.Unlock()
	acc, ok := accounts[id]
	if !ok {
		return errors.New("account does not exist")
	}
	alternative := acc.SecretSet
	for _, pk := range passkeys {
		if pk.Account == id {
			alternative = true
			break
		}
	}
	if !alternative {
		return errors.New("set a password or add a passkey before disconnecting Google sign-in")
	}
	previous := acc.GoogleSignInDisabled
	acc.GoogleSignInDisabled = true
	if err := data.SaveJSON("accounts.json", accounts); err != nil {
		acc.GoogleSignInDisabled = previous
		return err
	}
	return nil
}
