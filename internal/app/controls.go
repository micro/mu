package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/internal/auth"
	"mu/internal/flag"
)

// ControlsHandler handles generic content actions: /app/{action}
func ControlsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" && r.Method != "GET" {
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

	redirect := r.URL.Query().Get("redirect")
	if redirect == "" {
		redirect = r.Referer()
	}
	if redirect == "" {
		redirect = "/app/saved"
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

	case "saved":
		renderSavedPage(w, r, sess.Account)

	case "blocked":
		renderBlockedPage(w, r, sess.Account)

	default:
		http.NotFound(w, r)
		return
	}
}

func renderSavedPage(w http.ResponseWriter, r *http.Request, userID string) {
	saved := GetSavedItems(userID)

	var sb strings.Builder
	sb.WriteString(`<div class="card"><h3>Saved</h3>`)

	if len(saved) == 0 {
		sb.WriteString(`<p class="text-muted">Nothing saved yet.</p>`)
	} else {
		// Sort by save time, newest first
		type item struct {
			key  string
			time string
			url  string
		}
		var items []item
		for key, t := range saved {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) != 2 {
				continue
			}
			ct, cid := parts[0], parts[1]
			u := contentURL(ct, cid)
			if u == "" {
				u = "#"
			}
			items = append(items, item{key: key, time: t.Format("2 Jan 15:04"), url: u})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].time > items[j].time
		})

		for _, it := range items {
			parts := strings.SplitN(it.key, ":", 2)
			ct, cid := parts[0], parts[1]
			label := ct + ": " + cid
			if len(cid) > 30 {
				label = ct + ": " + cid[:30] + "..."
			}
			sb.WriteString(fmt.Sprintf(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0">
				<a href="%s">%s</a>
				<span class="text-sm text-muted"> · %s · </span>
				<a href="/app/unsave?type=%s&id=%s" class="text-sm text-muted">remove</a>
			</div>`, it.url, label, it.time, ct, cid))
		}
	}
	sb.WriteString(`</div>`)

	html := RenderHTMLForRequest("Saved", "Your saved items", sb.String(), r)
	w.Write([]byte(html))
}

func renderBlockedPage(w http.ResponseWriter, r *http.Request, userID string) {
	blocked := GetBlockedUsers(userID)

	var sb strings.Builder
	sb.WriteString(`<div class="card"><h3>Blocked Users</h3>`)

	if len(blocked) == 0 {
		sb.WriteString(`<p class="text-muted">No blocked users.</p>`)
	} else {
		for uid, t := range blocked {
			sb.WriteString(fmt.Sprintf(`<div style="padding:6px 0;border-bottom:1px solid #f0f0f0">
				<a href="/@%s">%s</a>
				<span class="text-sm text-muted"> · blocked %s · </span>
				<a href="/app/unblock?user=%s" class="text-sm text-muted">unblock</a>
			</div>`, uid, uid, t.Format("2 Jan 2006"), uid))
		}
	}
	sb.WriteString(`</div>`)

	html := RenderHTMLForRequest("Blocked", "Blocked users", sb.String(), r)
	w.Write([]byte(html))
}
