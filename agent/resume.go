package agent

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
)

// MicroHandler renders the signed-in agent at the front door.
func MicroHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	servePage(w, r)
}

func chatPath(owner, id string) string {
	if id == "" {
		return "/agent/micro"
	}
	return Path(owner, id)
}
