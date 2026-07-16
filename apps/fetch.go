package apps

import (
	"fmt"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/safefetch"
)

// handleSDKFetch serves mu.server.fetch at /apps/{slug}/sdk/fetch: a guarded
// server-side HTTP fetch, so a client app can reach external APIs without CORS,
// keep keys server-side, and add server value. It requires a session (so the
// instance isn't an open proxy) and is SSRF-guarded by safefetch.
func handleSDKFetch(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	if _, _, err := auth.RequireSession(r); err != nil {
		app.RespondError(w, http.StatusUnauthorized, "Authentication required")
		return
	}

	var req struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.URL == "" {
		app.RespondError(w, http.StatusBadRequest, "url is required")
		return
	}

	// Metered: an external fetch has a real cost, so gate on the user's balance.
	if QuotaCheck != nil {
		canProceed, cost, qerr := QuotaCheck(r, "web_fetch")
		if !canProceed {
			msg := "Insufficient credits"
			if qerr != nil {
				msg = qerr.Error()
			}
			app.RespondError(w, http.StatusPaymentRequired, fmt.Sprintf("%s (costs %d credits)", msg, cost))
			return
		}
	}

	resp, err := safefetch.Fetch(r.Context(), req.URL, safefetch.Options{
		Method:  req.Method,
		Headers: req.Headers,
		Body:    req.Body,
	})
	if err != nil {
		app.RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	if ChargeQuota != nil {
		ChargeQuota(r, "web_fetch")
	}
	app.RespondJSON(w, resp)
}
