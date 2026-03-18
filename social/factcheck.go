package social

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"mu/internal/check"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/wallet"
)

// factCheckThread runs a background fact-check on a user-posted thread.
// It searches the web for claims made in the post, then uses AI to assess
// whether the claims are accurate, misleading, or missing context.
// The result is stored as a CommunityNote on the thread.
func factCheckThread(threadID string) {
	// Small delay to let the thread settle
	time.Sleep(2 * time.Second)

	mutex.RLock()
	t := getThread(threadID)
	if t == nil {
		mutex.RUnlock()
		return
	}
	// Skip system-seeded threads (already fact-checked during seeding)
	if t.AuthorID == app.SystemUserID {
		mutex.RUnlock()
		return
	}
	title := t.Title
	content := t.Content
	mutex.RUnlock()

	note := checkClaims(title, content)
	if note == nil {
		return
	}

	mutex.Lock()
	t = getThread(threadID)
	if t != nil {
		t.Note = note
	}
	mutex.Unlock()

	save()
	updateCache()
	app.Log("social", "Fact-checked thread %s: %s", threadID, note.Status)
}

// factCheckReply runs a background fact-check on a user reply.
func factCheckReply(threadID, replyID string) {
	time.Sleep(2 * time.Second)

	mutex.RLock()
	t := getThread(threadID)
	if t == nil {
		mutex.RUnlock()
		return
	}
	var reply *Reply
	for _, r := range t.Replies {
		if r.ID == replyID {
			reply = r
			break
		}
	}
	if reply == nil || reply.AuthorID == app.SystemUserID {
		mutex.RUnlock()
		return
	}
	content := reply.Content
	mutex.RUnlock()

	note := checkClaims("", content)
	if note == nil {
		return
	}

	mutex.Lock()
	t = getThread(threadID)
	if t != nil {
		for _, r := range t.Replies {
			if r.ID == replyID {
				r.Note = note
				break
			}
		}
	}
	mutex.Unlock()

	save()
	updateCache()
	app.Log("social", "Fact-checked reply %s: %s", replyID, note.Status)
}

// checkClaims uses the factcheck package to verify claims and converts
// the result to a CommunityNote. Returns nil if no actionable claims found.
func checkClaims(title, content string) *CommunityNote {
	fullText := content
	if title != "" {
		fullText = title + "\n\n" + content
	}

	// Skip very short posts
	if len(fullText) < 50 {
		return nil
	}

	result := check.Check(fullText)
	if result == nil {
		return nil
	}

	// Don't create notes for accurate or no-claims content
	if result.Status == "accurate" || result.Status == "none" {
		return nil
	}

	// Convert check.Result to CommunityNote
	var sources []Source
	for _, s := range result.Sources {
		sources = append(sources, Source{Title: s.Title, URL: s.URL})
	}

	return &CommunityNote{
		Content:   result.Content,
		Sources:   sources,
		Status:    result.Status,
		CheckedAt: result.CheckedAt,
	}
}

// FactCheckHandler handles POST /social?id={id}&factcheck=true
// Allows users to manually trigger a fact-check on a thread.
func FactCheckHandler(w http.ResponseWriter, r *http.Request, threadID string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	mutex.RLock()
	t := getThread(threadID)
	if t == nil {
		mutex.RUnlock()
		http.NotFound(w, r)
		return
	}
	title := t.Title
	content := t.Content
	mutex.RUnlock()

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpFactCheck)
	if !canProceed {
		if app.SendsJSON(r) || app.WantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(402)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "quota_exceeded",
				"message": "Daily limit reached. Please top up credits at /wallet",
				"cost":    cost,
			})
			return
		}
		c := wallet.QuotaExceededPage(wallet.OpFactCheck, cost)
		page := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", c, r)
		w.Write([]byte(page))
		return
	}

	// Perform fact-check using the factcheck package
	fullText := title + "\n\n" + content
	result := check.Check(fullText)

	// Consume quota
	wallet.ConsumeQuota(acc.ID, wallet.OpFactCheck)

	if result == nil {
		if app.SendsJSON(r) || app.WantsJSON(r) {
			app.RespondJSON(w, map[string]interface{}{
				"status":  "error",
				"message": "Fact-check failed. Please try again.",
			})
			return
		}
		http.Redirect(w, r, "/social?id="+threadID, http.StatusSeeOther)
		return
	}

	// Store result as community note on the thread (if actionable)
	if result.Status != "none" {
		var sources []Source
		for _, s := range result.Sources {
			sources = append(sources, Source{Title: s.Title, URL: s.URL})
		}
		note := &CommunityNote{
			Content:   result.Content,
			Sources:   sources,
			Status:    result.Status,
			CheckedAt: result.CheckedAt,
		}

		mutex.Lock()
		t = getThread(threadID)
		if t != nil {
			t.Note = note
		}
		mutex.Unlock()

		save()
		updateCache()
		app.Log("social", "Manual fact-check on thread %s by %s: %s", threadID, acc.ID, note.Status)
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, result)
		return
	}
	http.Redirect(w, r, "/social?id="+threadID, http.StatusSeeOther)
}

// renderCommunityNote renders a community note as HTML.
// Returns empty string if note is nil.
func renderCommunityNote(note *CommunityNote) string {
	if note == nil {
		return ""
	}

	// Convert to check.Result for rendering
	var sources []check.Source
	for _, s := range note.Sources {
		sources = append(sources, check.Source{Title: s.Title, URL: s.URL})
	}

	return check.RenderResult(&check.Result{
		Content:   note.Content,
		Sources:   sources,
		Status:    note.Status,
		CheckedAt: note.CheckedAt,
	})
}

// renderFactCheckButton renders a "Fact Check" button for a thread.
func renderFactCheckButton(threadID string) string {
	return fmt.Sprintf(`<form method="POST" action="/social?id=%s&factcheck=true" style="display:inline;margin-left:8px;">
<button type="submit" style="background:none;border:1px solid #ccc;color:#666;font-size:11px;cursor:pointer;padding:2px 8px;border-radius:3px;" title="Fact-check this thread">Fact Check</button>
</form>`, threadID)
}

