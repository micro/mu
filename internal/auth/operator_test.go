package auth

import (
	"testing"
	"time"
)

func TestOperatorIdentityCannotAuthenticateOrBeDeleted(t *testing.T) {
	before := len(AllAccounts())
	a, err := GetAccount(OperatorID)
	if err != nil || !a.System || a.Admin || a.Agent || a.Secret != "" {
		t.Fatalf("wrong system identity: %+v %v", a, err)
	}
	if len(AllAccounts()) != before {
		t.Fatal("system identity affects human bootstrap")
	}
	if _, err := Login(OperatorID, ""); err == nil {
		t.Fatal("operator logged in")
	}
	if _, err := CreateSession(OperatorID); err == nil {
		t.Fatal("operator session created")
	}
	if _, _, err := CreateToken(OperatorID, "test", nil, time.Time{}); err == nil {
		t.Fatal("operator token created")
	}
	if err := DeleteAccount(OperatorID); err == nil {
		t.Fatal("operator deleted")
	}
	a.Admin = true
	if err := UpdateAccount(a); err == nil {
		t.Fatal("operator mutated")
	}
	again, _ := GetAccount(OperatorID)
	if again.Admin {
		t.Fatal("identity shared mutable state")
	}
}

func TestOperatorDoesNotTakeFirstHumanAdmin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ADMIN", "")
	mutex.Lock()
	old := accounts
	accounts = map[string]*Account{}
	mutex.Unlock()
	defer func() { mutex.Lock(); accounts = old; mutex.Unlock() }()
	if _, err := GetAccount(OperatorID); err != nil {
		t.Fatal(err)
	}
	if AdminExists() {
		t.Fatal("operator counted as human admin")
	}
	if err := Create(&Account{ID: "firstperson", Name: "First", Secret: "test-only-password"}); err != nil {
		t.Fatal(err)
	}
	a, _ := GetAccount("firstperson")
	if !a.Admin {
		t.Fatal("first person lost administrator bootstrap")
	}
}
