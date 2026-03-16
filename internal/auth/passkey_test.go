package auth

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func TestWebAuthnUser(t *testing.T) {
	// Clear passkeys for clean test
	mutex.Lock()
	passkeys = map[string]*Passkey{}
	mutex.Unlock()

	acc := &Account{
		ID:      "testuser",
		Name:    "Test User",
		Created: time.Now(),
	}

	user := NewWebAuthnUser(acc)

	if string(user.WebAuthnID()) != "testuser" {
		t.Errorf("expected WebAuthnID 'testuser', got '%s'", string(user.WebAuthnID()))
	}
	if user.WebAuthnName() != "testuser" {
		t.Errorf("expected WebAuthnName 'testuser', got '%s'", user.WebAuthnName())
	}
	if user.WebAuthnDisplayName() != "Test User" {
		t.Errorf("expected WebAuthnDisplayName 'Test User', got '%s'", user.WebAuthnDisplayName())
	}
	if len(user.WebAuthnCredentials()) != 0 {
		t.Errorf("expected 0 credentials, got %d", len(user.WebAuthnCredentials()))
	}
}

func TestSaveAndGetPasskeys(t *testing.T) {
	// Clear passkeys for test
	mutex.Lock()
	passkeys = map[string]*Passkey{}
	mutex.Unlock()

	pk := &Passkey{
		ID:      "pk-1",
		Name:    "My Passkey",
		Account: "testuser",
		Credential: webauthn.Credential{
			ID:        []byte("cred-id-1"),
			PublicKey: []byte("pubkey-1"),
		},
		Created: time.Now(),
	}

	if err := SavePasskey(pk); err != nil {
		t.Fatalf("SavePasskey failed: %v", err)
	}

	// Get passkeys
	pks := GetPasskeys("testuser")
	if len(pks) != 1 {
		t.Fatalf("expected 1 passkey, got %d", len(pks))
	}
	if pks[0].ID != "pk-1" {
		t.Errorf("expected passkey ID 'pk-1', got '%s'", pks[0].ID)
	}
	if pks[0].Name != "My Passkey" {
		t.Errorf("expected passkey name 'My Passkey', got '%s'", pks[0].Name)
	}

	// Get passkeys for different user
	pks2 := GetPasskeys("otheruser")
	if len(pks2) != 0 {
		t.Errorf("expected 0 passkeys for otheruser, got %d", len(pks2))
	}
}

func TestDeletePasskey(t *testing.T) {
	mutex.Lock()
	passkeys = map[string]*Passkey{}
	mutex.Unlock()

	pk := &Passkey{
		ID:      "pk-del",
		Name:    "Delete Me",
		Account: "testuser",
		Credential: webauthn.Credential{
			ID: []byte("cred-del"),
		},
		Created: time.Now(),
	}
	SavePasskey(pk)

	// Try deleting with wrong account
	err := DeletePasskey("pk-del", "wronguser")
	if err == nil {
		t.Error("expected error deleting passkey with wrong account")
	}

	// Delete with correct account
	err = DeletePasskey("pk-del", "testuser")
	if err != nil {
		t.Fatalf("DeletePasskey failed: %v", err)
	}

	// Verify deleted
	pks := GetPasskeys("testuser")
	if len(pks) != 0 {
		t.Errorf("expected 0 passkeys after delete, got %d", len(pks))
	}

	// Try deleting non-existent
	err = DeletePasskey("nonexistent", "testuser")
	if err == nil {
		t.Error("expected error deleting non-existent passkey")
	}
}

func TestUpdatePasskeyUsage(t *testing.T) {
	mutex.Lock()
	passkeys = map[string]*Passkey{}
	mutex.Unlock()

	pk := &Passkey{
		ID:      "pk-usage",
		Name:    "Usage Test",
		Account: "testuser",
		Credential: webauthn.Credential{
			ID: []byte("cred-usage"),
			Authenticator: webauthn.Authenticator{
				SignCount: 0,
			},
		},
		Created: time.Now(),
	}
	SavePasskey(pk)

	UpdatePasskeyUsage([]byte("cred-usage"), 5)

	pks := GetPasskeys("testuser")
	if len(pks) != 1 {
		t.Fatalf("expected 1 passkey, got %d", len(pks))
	}
	if pks[0].Credential.Authenticator.SignCount != 5 {
		t.Errorf("expected sign count 5, got %d", pks[0].Credential.Authenticator.SignCount)
	}
	if pks[0].LastUsed.IsZero() {
		t.Error("expected LastUsed to be set")
	}
}

func TestCreateSession(t *testing.T) {
	// Set up a test account
	mutex.Lock()
	accounts["sessiontest"] = &Account{
		ID:      "sessiontest",
		Name:    "Session Test",
		Secret:  "$2a$10$dummyhash",
		Created: time.Now(),
	}
	mutex.Unlock()

	// Test creating session for valid account
	sess, err := CreateSession("sessiontest")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.Account != "sessiontest" {
		t.Errorf("expected account 'sessiontest', got '%s'", sess.Account)
	}
	if sess.Type != "account" {
		t.Errorf("expected type 'account', got '%s'", sess.Type)
	}
	if sess.Token == "" {
		t.Error("expected non-empty token")
	}

	// Test creating session for non-existent account
	_, err = CreateSession("nonexistent")
	if err == nil {
		t.Error("expected error creating session for non-existent account")
	}

	// Clean up
	mutex.Lock()
	delete(accounts, "sessiontest")
	delete(sessions, sess.ID)
	mutex.Unlock()
}

func TestFindUserByWebAuthnID(t *testing.T) {
	// Set up test account
	mutex.Lock()
	accounts["findtest"] = &Account{
		ID:      "findtest",
		Name:    "Find Test",
		Created: time.Now(),
	}
	mutex.Unlock()

	user, err := FindUserByWebAuthnID([]byte("findtest"))
	if err != nil {
		t.Fatalf("FindUserByWebAuthnID failed: %v", err)
	}
	if user.WebAuthnName() != "findtest" {
		t.Errorf("expected 'findtest', got '%s'", user.WebAuthnName())
	}

	// Non-existent user
	_, err = FindUserByWebAuthnID([]byte("nosuchuser"))
	if err == nil {
		t.Error("expected error for non-existent user")
	}

	// Clean up
	mutex.Lock()
	delete(accounts, "findtest")
	mutex.Unlock()
}
