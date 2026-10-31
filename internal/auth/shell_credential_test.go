package auth

import (
	"testing"
	"time"
)

func TestLegacyShellCredentialsAreRetired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "retired-shell-owner"
	SetAccountForTest(&Account{ID: owner, Approved: true})
	defer RemoveAccountForTest(owner)
	_, raw, err := CreateToken(owner, "shell session", nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ValidatePAT(raw); err == nil {
		t.Fatal("legacy shell token still authenticates")
	}
}
