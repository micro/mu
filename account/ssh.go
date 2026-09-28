package account

import (
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/sshaccess"
)

// handleSSHKey also accepts forms opened at the old Tokens location.
func handleSSHKey(w http.ResponseWriter, r *http.Request, accountID string) bool {
	if r.Method != http.MethodPost {
		return false
	}
	r.ParseForm()
	key, remove := r.PostForm.Get("sshkey"), r.PostForm.Get("removekey")
	if key == "" && remove == "" {
		return false
	}
	sess, _, err := auth.RequireSession(r)
	if err != nil || sess.Type != "account" || !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reopen Account security and try again.")
		return true
	}
	if key != "" {
		if err := auth.CheckCredentialAccess(accountID); err != nil {
			app.Forbidden(w, r, err.Error())
			return true
		}
		if err := auth.CheckPostRate(accountID); err != nil {
			app.TooManyRequests(w, r, err.Error())
			return true
		}
		_, err = sshaccess.Register(accountID, key, r.PostForm.Get("keyname"))
	} else {
		err = auth.RemoveSSHKey(accountID, remove)
	}
	if err != nil {
		app.RespondError(w, http.StatusBadRequest, err.Error())
		return true
	}
	http.Redirect(w, r, "/account#security", http.StatusSeeOther)
	return true
}
