package account

import "net/http"

// Old bookmarks lead to the canonical account pages; no second UI is retained.
func UsageMoved(w http.ResponseWriter, r *http.Request) {
	target := "/account/usage"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}

func BillingMoved(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/account/billing", http.StatusTemporaryRedirect)
}
