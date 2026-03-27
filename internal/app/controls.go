package app

import (
	"encoding/json"
	"net/http"
	"strings"

	"mu/internal/auth"
	"mu/internal/flag"
)

// ControlsHandler handles generic content actions: /app/{action}
func ControlsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}

	sess, _, err := auth.RequireSession(r)
	if err != nil {
		if SendsJSON(r) || WantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		RedirectToLogin(w, r)
		return
	}

	action := strings.TrimPrefix(r.URL.Path, "/app/")
	contentType := r.URL.Query().Get("type")
	contentID := r.URL.Query().Get("id")
	userParam := r.URL.Query().Get("user")

	// Also accept JSON body
	if SendsJSON(r) {
		var body struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			User string `json:"user"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.Type != "" {
				contentType = body.Type
			}
			if body.ID != "" {
				contentID = body.ID
			}
			if body.User != "" {
				userParam = body.User
			}
		}
	}

	redirect := r.Referer()
	if redirect == "" {
		redirect = "/"
	}

	respond := func(status string) {
		if SendsJSON(r) || WantsJSON(r) {
			RespondJSON(w, map[string]string{"status": status})
			return
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	}

	switch action {
	case "flag":
		if contentType == "" || contentID == "" {
			respond("missing type or id")
			return
		}
		flag.Add(contentType, contentID, sess.Account)
		respond("flagged")

	case "save":
		if contentType == "" || contentID == "" {
			respond("missing type or id")
			return
		}
		SaveItem(sess.Account, contentType, contentID)
		respond("saved")

	case "unsave":
		if contentType == "" || contentID == "" {
			respond("missing type or id")
			return
		}
		UnsaveItem(sess.Account, contentType, contentID)
		respond("unsaved")

	case "dismiss":
		if contentType == "" || contentID == "" {
			respond("missing type or id")
			return
		}
		DismissItem(sess.Account, contentType, contentID)
		respond("dismissed")

	case "block":
		if userParam == "" {
			respond("missing user")
			return
		}
		BlockUser(sess.Account, userParam)
		respond("blocked")

	case "unblock":
		if userParam == "" {
			respond("missing user")
			return
		}
		UnblockUser(sess.Account, userParam)
		respond("unblocked")

	default:
		http.NotFound(w, r)
		return
	}
}
