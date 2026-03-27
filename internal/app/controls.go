package app

import (
	"net/http"
	"strings"

	"mu/internal/auth"
	"mu/internal/flag"
)

// ControlsHandler handles generic content actions: /controls/{action}
func ControlsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}

	sess, _, err := auth.RequireSession(r)
	if err != nil {
		RedirectToLogin(w, r)
		return
	}

	action := strings.TrimPrefix(r.URL.Path, "/controls/")
	contentType := r.URL.Query().Get("type")
	contentID := r.URL.Query().Get("id")
	userParam := r.URL.Query().Get("user")

	redirect := r.Referer()
	if redirect == "" {
		redirect = "/"
	}

	switch action {
	case "flag":
		if contentType == "" || contentID == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		flag.Add(contentType, contentID, sess.Account)

	case "save":
		if contentType == "" || contentID == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		SaveItem(sess.Account, contentType, contentID)

	case "unsave":
		if contentType == "" || contentID == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		UnsaveItem(sess.Account, contentType, contentID)

	case "dismiss":
		if contentType == "" || contentID == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		DismissItem(sess.Account, contentType, contentID)

	case "block":
		if userParam == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		BlockUser(sess.Account, userParam)

	case "unblock":
		if userParam == "" {
			http.Redirect(w, r, redirect, http.StatusSeeOther)
			return
		}
		UnblockUser(sess.Account, userParam)

	default:
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}
