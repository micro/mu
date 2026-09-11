package agent

import (
	"mu/internal/auth"
	"sync"
	"testing"
)

func TestConcurrentScopeEditAndIssuanceCannotLeaveBroadToken(t *testing.T) {
	probes(t)
	id := owner(t, "scope_concurrent_owner")
	a, _, err := CreateAgent(id, "Reader", External, "", "", []string{"probealpha", "probebeta"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if _, err := UpdateAgent(id, a.ID, "Reader", "", "", []string{"probealpha", "probebeta"}); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if _, err := IssueToken(id, a.ID); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			if _, err := UpdateAgent(id, a.ID, "Reader", "", "", []string{"probealpha"}); err != nil {
				t.Error(err)
			}
		}()
		close(start)
		wg.Wait()
		for _, token := range auth.ListTokens(id) {
			if token.AllowsService("probebeta") {
				t.Fatal("concurrent edit left a live broad token")
			}
		}
	}
}

func TestEditingScopeRevokesTheOldCredential(t *testing.T) {
	probes(t)
	id := owner(t, "scope_revoke_owner")
	a, secret, err := CreateAgent(id, "Reader", External, "", "", []string{"probealpha", "probebeta"}, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := UpdateAgent(id, a.ID, "Reader", "", "", []string{"probealpha"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.TokenID != "" {
		t.Fatal("old token still attached")
	}
	if _, err := auth.ValidatePAT(secret); err == nil {
		t.Fatal("old broad credential still works")
	}
}

func TestUnavailableScopeDoesNotCreateUnrestrictedAgent(t *testing.T) {
	id := owner(t, "invalidscopeowner")
	a, secret, err := CreateAgent(id, "Restricted", External, "", "", []string{"unavailable-service"}, true)
	if err == nil || a != nil || secret != "" {
		t.Fatalf("invalid scope created agent: %v %v", a, err)
	}
	if len(Agents(id)) != 0 {
		t.Fatal("failed creation persisted an agent")
	}
}

func TestUnavailableScopeDoesNotWidenExistingAgent(t *testing.T) {
	probes(t)
	id := owner(t, "invalidscopeedit")
	a, _, err := CreateAgent(id, "Restricted", Hosted, "", "", []string{"probealpha"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateAgent(id, a.ID, "Changed", "", "", []string{"unavailable-service"}); err == nil {
		t.Fatal("invalid scope accepted")
	}
	got := Agents(id)
	if len(got) != 1 || got[0].Unscoped() || got[0].Name != "Restricted" {
		t.Fatalf("failed update changed agent: %+v", got)
	}
}
