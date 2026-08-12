package auth

import (
	"os"
	"testing"
	"time"
)

func withAccounts(t *testing.T, seed map[string]*Account) {
	t.Helper()
	mutex.Lock()
	prev := accounts
	accounts = seed
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts = prev
		mutex.Unlock()
	})
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	os.MkdirAll(dir+"/.mu/data", 0o755) //nolint:errcheck
}

// TestMicroWaitsForAHuman is the footgun this was written around.
//
// Account creation bootstraps the first account on an empty instance as admin,
// so creating Micro at boot on a fresh install would hand the instance to the
// agent and leave the person who runs it locked out of /admin/env.
func TestMicroWaitsForAHuman(t *testing.T) {
	withAccounts(t, map[string]*Account{})

	EnsureMicro()
	if _, err := GetAccount(MicroID); err == nil {
		t.Fatal("Micro was created on an empty instance — the first account " +
			"belongs to the human who runs it")
	}
}

// TestMicroIsAnAgentAndNotAnAdmin — it was an admin, briefly, to get the
// billing exemption that admins happen to carry. That granted /admin/env, the
// console and the power to ban, to avoid a balance check. The exemption is now
// its own rule in internal/quota and this account has no more privilege than it
// needs.
func TestMicroIsAnAgentAndNotAnAdmin(t *testing.T) {
	withAccounts(t, map[string]*Account{
		"asim": {ID: "asim", Name: "Asim", Admin: true, Created: time.Now()},
	})

	EnsureMicro()
	acc, err := GetAccount(MicroID)
	if err != nil {
		t.Fatalf("Micro was not created once a human admin existed: %v", err)
	}
	if acc.Admin {
		t.Error("Micro is an admin — that is /admin/env, the console and the " +
			"power to ban, granted to a program to avoid a balance check")
	}
	if !acc.Agent {
		t.Error("Micro is not marked as an agent — a system that cannot tell a " +
			"program from a person will eventually mail one a password reset")
	}
	if acc.Secret == "" {
		t.Error("Micro has an empty secret, which is a password of \"\" the day " +
			"somebody adds a login path that forgets this account is different")
	}
	if !IsAgent(MicroID) {
		t.Error("IsAgent does not recognise Micro")
	}
	if IsAgent("asim") {
		t.Error("a human was reported as an agent")
	}
}

func TestEnsureMicroIsIdempotent(t *testing.T) {
	withAccounts(t, map[string]*Account{
		"asim": {ID: "asim", Name: "Asim", Admin: true, Created: time.Now()},
	})

	EnsureMicro()
	first, err := GetAccount(MicroID)
	if err != nil {
		t.Fatal(err)
	}
	secret := first.Secret

	EnsureMicro() // every boot calls this
	again, err := GetAccount(MicroID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Secret != secret {
		t.Error("a second boot replaced Micro's account")
	}
}
