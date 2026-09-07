package api

import (
	"mu/internal/auth"
	"mu/internal/service"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOperatorRequiresExplicitHumanAuthority(t *testing.T) {
	t.Setenv("MU_OPERATOR_ENABLED", "true")
	for _, tc := range []struct {
		id           string
		admin, agent bool
	}{{"operator_human", true, false}, {"operator_agent", true, true}, {"operator_user", false, false}} {
		acc := &auth.Account{ID: tc.id, Secret: "test-secret", Admin: tc.admin, Agent: tc.agent}
		if err := auth.Create(acc); err != nil {
			t.Fatal(err)
		}
		acc.Admin, acc.Agent = tc.admin, tc.agent
		if err := auth.UpdateAccount(acc); err != nil {
			t.Fatal(err)
		}
		for _, granted := range []bool{false, true} {
			permissions := []string{}
			if granted {
				permissions = append(permissions, "operator")
			}
			_, secret, err := auth.CreateToken(acc.ID, "probe", permissions, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("POST", "/mcp", nil)
			r.Header.Set("Authorization", "Bearer "+secret)
			want := tc.admin && !tc.agent && granted
			if operatorAllowed(r) != want {
				t.Fatalf("%s grant=%v: unexpected permission", tc.id, granted)
			}
			if operatorAllowed(r.WithContext(service.WithAgentRun(r.Context()))) {
				t.Fatal("agent run inherited operator")
			}
			t.Setenv("MU_OPERATOR_ENABLED", "false")
			if operatorAllowed(r) {
				t.Fatal("disabled operator allowed")
			}
			t.Setenv("MU_OPERATOR_ENABLED", "true")
		}
	}
	if operatorAllowed(httptest.NewRequest("POST", "/mcp", nil)) {
		t.Fatal("anonymous operator")
	}
}
