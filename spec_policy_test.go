package main

import (
	"testing"

	"mu/internal/service"
	"mu/service/db"
	"mu/service/index"
	"mu/service/mail"
	"mu/service/wallet"
	"mu/service/web"
)

// The real specs must reproduce the policy the deleted hand-written maps held.
func TestSpecsReproduceTheOldPolicy(t *testing.T) {
	for _, s := range []service.Spec{mail.Spec, index.Spec, db.Spec, wallet.Spec, web.Spec} {
		if err := service.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.Name, err)
		}
	}
	// accountScoped, deleted from internal/service/dynamic.go
	for _, n := range []string{"mail", "index", "db", "wallet"} {
		if !service.AccountScoped(n) {
			t.Errorf("%s lost its account scoping", n)
		}
	}
	if service.AccountScoped("web") {
		t.Error("web is public and must not be scoped")
	}
	// destructiveTools, deleted from agent/native.go
	if !service.Destructive("wallet", "Charge") || !service.Destructive("db", "Delete") {
		t.Error("a destructive method lost its guard")
	}
	if service.Destructive("wallet", "Balance") || service.Destructive("db", "Get") {
		t.Error("a read was marked destructive")
	}
	// agentToolLabels, deleted from agent/native.go
	if got := service.Label("web"); got != "Search" {
		t.Errorf("web label = %q, want Search", got)
	}
	if got := service.Label("db"); got != "Storage" {
		t.Errorf("db label = %q, want Storage", got)
	}
	if got := service.Label("mail"); got != "Mail" {
		t.Errorf("mail label = %q, want Mail", got)
	}
}
