package api

import (
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/settings"
	"net/http"
)

func operatorAllowed(r *http.Request) bool {
	if r == nil || settings.Get("MU_OPERATOR_ENABLED") != "true" || service.InAgentRun(r.Context()) {
		return false
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil || !acc.Admin || acc.Agent {
		return false
	}
	if token := auth.TokenFromRequest(r); token != nil {
		for _, permission := range token.Permissions {
			if permission == "operator" {
				return true
			}
		}
		return false
	}
	return true
}
