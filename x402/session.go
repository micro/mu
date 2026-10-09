package x402

import (
	"encoding/json"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// SessionHandler refreshes the browser CSRF cookie and returns session metadata.
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	sess, acc := auth.TrySession(r)
	if sess == nil {
		// Return guest session instead of error
		guestSess := map[string]interface{}{
			"type": "guest",
		}
		b, _ := json.Marshal(guestSess)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Build response with account info
	response := map[string]interface{}{
		"id":      sess.ID,
		"type":    sess.Type,
		"account": sess.Account,
		"created": sess.Created,
	}

	if acc != nil {
		response["admin"] = acc.Admin
	}

	b, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
