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

var typeLabels = map[string]string{
	"post":   "Blog post",
	"work":   "Work",
	"app":    "App",
	"social": "Thread",
	"video":  "Video",
	"news":   "News",
	"web":    "Web page",
}

func renderSavedPage(w http.ResponseWriter, r *http.Request, userID string) {
	saved := GetSavedItems(userID)

	var sb strings.Builder

	if len(saved) == 0 {
		sb.WriteString(`<div class="card"><p class="text-muted">Nothing saved yet. Use the ☆ icon on any content to save it.</p></div>`)
	} else {
		type item struct {
			ct, cid, url, label, time string
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
			label := typeLabels[ct]
			if label == "" {
				label = ct
			}
			items = append(items, item{ct: ct, cid: cid, url: u, label: label, time: t.Format("2 Jan 15:04")})
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].time > items[j].time
		})

		for _, it := range items {
			displayID := it.cid
			if len(displayID) > 40 {
				displayID = displayID[:40] + "..."
			}
			sb.WriteString(fmt.Sprintf(`<div class="card">
				<p><a href="%s"><strong>%s</strong> — %s</a></p>
				<p class="text-sm text-muted">Saved %s · <a href="#" onclick="fetch('/app/unsave?type=%s&id=%s',{method:'POST'}).then(function(){location.reload()});return false;">Remove</a></p>
			</div>`, it.url, it.label, displayID, it.time, it.ct, it.cid))
		}
	}

	html := RenderHTMLForRequest("Saved", "Your saved items", sb.String(), r)
	w.Write([]byte(html))
}

func renderBlockedPage(w http.ResponseWriter, r *http.Request, userID string) {
	blocked := GetBlockedUsers(userID)

	var sb strings.Builder

	if len(blocked) == 0 {
		sb.WriteString(`<div class="card"><p class="text-muted">No blocked users.</p></div>`)
	} else {
		for uid, t := range blocked {
			sb.WriteString(fmt.Sprintf(`<div class="card">
				<p><a href="/@%s">%s</a></p>
				<p class="text-sm text-muted">Blocked %s · <a href="#" onclick="fetch('/app/unblock?user=%s',{method:'POST'}).then(function(){location.reload()});return false;">Unblock</a></p>
			</div>`, uid, uid, t.Format("2 Jan 2006"), uid))
		}
	}

	html := RenderHTMLForRequest("Blocked Users", "Blocked users", sb.String(), r)
	w.Write([]byte(html))
}
