package agent

import (
	"mu/internal/auth"
	"testing"
)

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
