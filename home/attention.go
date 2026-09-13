package home

import (
	"crypto/sha256"
	"fmt"
	"html"
	"net/http"
	"sort"
	"time"

	"mu/internal/auth"
	"mu/internal/userdb"
	"mu/service/events"
	"mu/service/tasks"
)

// Attention is derived from owned records, never a second generated brief.
type attentionItem struct {
	Key, Title, Reason, Link string
	rank                     int
	at                       time.Time
}

func attentionItems(owner string, now time.Time) []attentionItem {
	if owner == "" {
		return nil
	}
	var out []attentionItem
	for _, status := range []string{tasks.StatusBlocked, tasks.StatusFailed, tasks.StatusTodo} {
		for _, t := range tasks.List(owner, status) {
			// Agent execution failures remain agent work, not a human decision.
			if t.Assignee != tasks.Me {
				continue
			}
			rank, reason := 1, "Needs your review"
			if status == tasks.StatusFailed {
				reason = "Failed · review before retrying"
			}
			if status == tasks.StatusBlocked {
				reason = "Blocked · review what is needed"
			}
			if status == tasks.StatusTodo {
				if t.Assignee != tasks.Me || t.Due.IsZero() || t.Due.After(now.Add(24*time.Hour)) {
					continue
				}
				rank, reason = 2, "Due soon"
				if t.Due.Before(now) {
					reason = "Overdue"
				}
			}
			key := fmt.Sprintf("%x", sha256.Sum256([]byte(t.ID+"|"+t.Title+"|"+t.Detail+"|"+t.Result+"|"+t.Due.UTC().Format(time.RFC3339Nano)+"|"+t.Status+"|"+t.Updated.UTC().Format(time.RFC3339Nano))))
			out = append(out, attentionItem{key, t.Title, reason, "/tasks?status=" + status + "#task-" + t.ID, rank, t.Due})
		}
	}
	// Local upcoming events are immediately available; opening Home never waits
	// for a calendar provider or calls a model to fill this space.
	for _, e := range events.Upcoming(owner) {
		if e.Paused || e.Builtin || e.When.Before(now) || e.When.After(now.Add(time.Hour)) {
			continue
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(e.ID+"|"+e.Title+"|"+e.When.UTC().Format(time.RFC3339Nano))))
		minutes := int(e.When.Sub(now).Minutes())
		out = append(out, attentionItem{key, e.Title, fmt.Sprintf("Starts in %d minutes", minutes), "/events", 0, e.When})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].at.Before(out[j].at)
	})
	return out
}

func attentionHidden(owner string, now time.Time) map[string]bool {
	hidden := map[string]bool{}
	records, err := userdb.List("home", owner, "attention", "mine", nil, "updated", "desc", 200)
	if err != nil {
		return hidden
	}
	for _, r := range records {
		key, _ := r.Data["key"].(string)
		until, _ := r.Data["until"].(string)
		if until == "" {
			hidden[key] = true
			continue
		}
		if at, err := time.Parse(time.RFC3339, until); err == nil && now.Before(at) {
			hidden[key] = true
		}
	}
	return hidden
}

func attentionHTML(r *http.Request, owner string) string {
	if owner == "" {
		return ""
	}
	now := time.Now()
	hidden := attentionHidden(owner, now)
	for _, a := range attentionItems(owner, now) {
		if hidden[a.Key] {
			continue
		}
		esc := html.EscapeString
		return `<aside class="home-attention" aria-label="Needs your attention" data-brief><p class="text-muted">Needs your attention</p><p>` + esc(a.Title) + `</p><p class="text-muted">` + esc(a.Reason) + `</p><a href="` + esc(a.Link) + `">Review →</a><form method="POST" action="/home" class="attention-actions"><input type="hidden" name="_csrf" value="` + esc(auth.CSRFToken(r)) + `"><input type="hidden" name="attention_key" value="` + a.Key + `"><button class="btn-link" name="attention_action" value="dismiss">Dismiss</button><button class="btn-link" name="attention_action" value="later" title="Show here again in one hour">Remind me later</button></form><script>(function(){var form=document.querySelector('.attention-actions');if(!form)return;form.addEventListener('submit',async function(e){e.preventDefault();var body=new FormData(form);body.set('attention_action',e.submitter.value);try{var response=await fetch('/home',{method:'POST',body:body});if(!response.ok)throw new Error();form.closest('aside').remove()}catch(err){var message=form.querySelector('[role=status]');if(!message){message=document.createElement('span');message.setAttribute('role','status');form.appendChild(message)}message.textContent='Could not save. Please try again.'}})})()</script></aside>`
	}
	return ""
}

func attentionAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return
	}
	if !auth.StrictCSRF(r) {
		http.Error(w, "Invalid request", http.StatusForbidden)
		return
	}
	action := r.FormValue("attention_action")
	if action != "dismiss" && action != "later" {
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}
	key := r.FormValue("attention_key")
	now := time.Now()
	valid := false
	for _, a := range attentionItems(sess.Account, now) {
		if a.Key == key {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "This item has changed", http.StatusConflict)
		return
	}
	records, err := userdb.List("home", sess.Account, "attention", "mine", nil, "updated", "desc", 200)
	if err != nil {
		http.Error(w, "Could not save", http.StatusInternalServerError)
		return
	}
	until := ""
	if action == "later" {
		until = now.Add(time.Hour).UTC().Format(time.RFC3339)
	}
	value := map[string]interface{}{"key": key, "until": until}
	id := ""
	for _, record := range records {
		if record.Data["key"] == key {
			id = record.ID
			break
		}
	}
	if id != "" {
		_, err = userdb.Update("home", sess.Account, "attention", id, value, false)
	} else {
		// Keep dismissal state bounded. Older, changed source versions can expire.
		if len(records) >= 200 {
			if err = userdb.Delete("home", sess.Account, "attention", records[len(records)-1].ID); err != nil {
				http.Error(w, "Could not save", 500)
				return
			}
		}
		_, err = userdb.Create("home", sess.Account, "attention", value, false)
	}
	if err != nil {
		http.Error(w, "Could not save", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}
