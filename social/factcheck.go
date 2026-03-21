package social

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/search"
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

// checkClaims searches the web and uses AI to verify factual claims.
// Returns nil if no actionable claims found.
func checkClaims(title, content string) *CommunityNote {
	fullText := content
	if title != "" {
		fullText = title + "\n\n" + content
	}

	// Skip very short posts
	if len(fullText) < 50 {
		return nil
	}

	note := runFactCheck(fullText)
	if note == nil {
		return nil
	}

	// Don't create notes for accurate or no-claims content
	if note.Status == "accurate" || note.Status == "none" {
		return nil
	}

	return note
}

// runFactCheck performs a fact-check on the given text.
// It searches the web for context and uses AI to assess accuracy.
func runFactCheck(claim string) *CommunityNote {
	claim = strings.TrimSpace(claim)

	if len(claim) < 20 {
		return &CommunityNote{
			Status:    "none",
			Content:   "Text too short to fact-check.",
			CheckedAt: time.Now(),
		}
	}

	// Search the web for current reporting
	query := claim
	if len(query) > 150 {
		query = query[:150]
	}

	var webContext []string
	var allResults []search.BraveResult

	results, err := search.SearchBraveCached(query, 5)
	if err == nil && len(results) > 0 {
		allResults = append(allResults, results...)
		for _, r := range results {
			ctx := fmt.Sprintf("%s: %s (Source: %s)", r.Title, r.Description, r.URL)
			if len(ctx) > 500 {
				ctx = ctx[:500] + "..."
			}
			webContext = append(webContext, ctx)
		}
	}

	// Search for deeper background context (Wikipedia, encyclopedias, reports)
	// This gives the AI historical and factual grounding beyond news coverage.
	bgQuery := query + " wikipedia"
	if len(bgQuery) > 160 {
		bgQuery = bgQuery[:160]
	}
	var backgroundContext string
	bgResults, bgErr := search.SearchBraveCached(bgQuery, 3)
	if bgErr == nil {
		for _, r := range bgResults {
			allResults = append(allResults, r)
			// Fetch the first Wikipedia or reference article for full content
			if backgroundContext == "" && isReferenceSource(r.URL) {
				_, content, fetchErr := search.FetchAndExtract(r.URL)
				if fetchErr == nil && len(content) > 0 {
					if len(content) > 3000 {
						content = content[:3000]
					}
					backgroundContext = content
				}
			}
		}
	}

	// Build the question with all context
	var question strings.Builder
	question.WriteString("## Claim to Fact-Check\n\n")
	question.WriteString(claim)
	question.WriteString("\n\n")

	if len(webContext) > 0 {
		question.WriteString("## Current Reporting\n\n")
		for _, ctx := range webContext {
			question.WriteString("- " + ctx + "\n")
		}
	}

	if backgroundContext != "" {
		question.WriteString("\n## Background Context\n\n")
		question.WriteString(backgroundContext)
		question.WriteString("\n")
	}

	prompt := &ai.Prompt{
		System: `You are a community note writer. You receive a claim or news article and web search results.

Your DEFAULT response should be "STATUS: none" — most articles are fine and don't need a note. Only write a note when there is a SIGNIFICANT issue that would leave someone with a materially wrong understanding.

What DOES warrant a note (high bar — must be clear and specific):
- A factual claim that is demonstrably wrong (wrong numbers, wrong dates, misattributed quotes)
- A critical fact that is omitted and without which the reader is seriously misled (e.g. reporting a military strike without mentioning verified civilian casualties that multiple sources confirm)
- Framing so one-sided that it dehumanises or erases an affected population (e.g. describing a famine as "food insecurity" when people are starving, or omitting that "targets" were residential areas)
- Market/crypto coverage designed to induce panic or FOMO when the underlying data tells a different story

What does NOT warrant a note:
- An article that is factually accurate, even if you could add more context
- Reporting from one perspective, unless it actively erases or dehumanises people
- Headlines that are simplified — that's normal journalism, not bias
- Opinion pieces, analysis, or commentary
- Articles where the "missing context" is just additional background rather than something that changes the fundamental understanding
- Anything where you're unsure — if in doubt, return "none"

Respond in this format:

STATUS: [none|accurate|misleading|missing_context|biased]
NOTE: [If not "none": 1-3 sentences with specific facts, dates, numbers, and named sources. If "none": No issues found.]

The statuses mean:
- none: Article is fine, no note needed (THIS SHOULD BE YOUR MOST COMMON RESPONSE)
- accurate: Claims are verified correct (only use when someone might doubt the claims)
- misleading: Contains specific factual errors you can correct with evidence
- missing_context: Omits a critical fact that fundamentally changes understanding
- biased: Framing actively dehumanises or erases affected people

Rules:
- Be specific — cite verifiable facts, not general observations
- Do NOT note what "could" be added — only note what MUST be known
- Do NOT lecture or moralise
- Write dollar amounts as plain numbers
- Keep the note under 400 characters`,
		Question: question.String(),
		Priority:  ai.PriorityLow,
		Caller:    "factcheck",
		MaxTokens: 512,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("social", "Fact-check AI failed: %v", err)
		return nil
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))

	if response == "" {
		return nil
	}

	// Parse response
	status := "none"
	noteText := ""

	if strings.HasPrefix(response, "STATUS:") {
		lines := strings.SplitN(response, "\n", 3)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "STATUS:") {
				s := strings.TrimSpace(strings.TrimPrefix(line, "STATUS:"))
				s = strings.ToLower(s)
				if s == "accurate" || s == "misleading" || s == "missing_context" || s == "biased" || s == "none" {
					status = s
				}
			}
			if strings.HasPrefix(line, "NOTE:") {
				noteText = strings.TrimSpace(strings.TrimPrefix(line, "NOTE:"))
			}
		}
	} else if strings.HasPrefix(response, "NO_NOTE") {
		return &CommunityNote{
			Status:    "none",
			Content:   "No verifiable factual claims found.",
			CheckedAt: time.Now(),
		}
	}

	if noteText == "" {
		noteText = response
		if idx := strings.Index(noteText, "NOTE:"); idx >= 0 {
			noteText = strings.TrimSpace(noteText[idx+5:])
		}
	}

	// Collect sources
	var sources []Source
	seen := map[string]bool{}
	for _, r := range allResults {
		if r.URL == "" || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		sources = append(sources, Source{Title: r.Title, URL: r.URL})
		if len(sources) >= 3 {
			break
		}
	}

	return &CommunityNote{
		Content:   noteText,
		Sources:   sources,
		Status:    status,
		CheckedAt: time.Now(),
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

	// Perform fact-check
	fullText := title + "\n\n" + content
	note := runFactCheck(fullText)

	// Consume quota
	wallet.ConsumeQuota(acc.ID, wallet.OpFactCheck)

	if note == nil {
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
	if note.Status != "none" {
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
		app.RespondJSON(w, note)
		return
	}
	http.Redirect(w, r, "/social?id="+threadID, http.StatusSeeOther)
}

// FactCheckPageHandler serves the standalone /factcheck endpoint.
func FactCheckPageHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleFactCheckPage(w, r)
	case "POST":
		handleFactCheckSubmit(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleFactCheckPage(w http.ResponseWriter, r *http.Request) {
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"service":     "fact_check",
			"description": "Submit a claim or statement to fact-check against web sources",
			"cost":        wallet.CostFactCheck,
			"method":      "POST",
			"params": map[string]string{
				"claim": "The claim or statement to fact-check (required, min 20 chars)",
			},
		})
		return
	}

	_, acc := auth.TrySession(r)

	var sb strings.Builder
	sb.WriteString(`<div class="card center-card-md">
<h2 style="margin-top:0;">Fact Check</h2>
<p class="text-muted">Submit any claim or statement to verify it against web sources.</p>`)

	if acc != nil {
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/factcheck" class="blog-form">
<textarea name="claim" rows="4" placeholder="Enter a claim or statement to fact-check..." required minlength="20"></textarea>
<button type="submit">Fact Check (%dp)</button>
</form>`, wallet.CostFactCheck))
	} else {
		sb.WriteString(`<p class="text-muted"><a href="/login?redirect=/factcheck">Login</a> to use the fact-checker.</p>`)
	}

	sb.WriteString(`</div>`)

	page := app.RenderHTMLForRequest("Fact Check", "Fact Check", sb.String(), r)
	w.Write([]byte(page))
}

func handleFactCheckSubmit(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	var claim string

	if app.SendsJSON(r) {
		var req struct {
			Claim string `json:"claim"`
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.BadRequest(w, r, "invalid json")
			return
		}
		claim = strings.TrimSpace(req.Claim)
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		claim = strings.TrimSpace(r.FormValue("claim"))
	}

	if claim == "" {
		app.BadRequest(w, r, "Claim is required")
		return
	}
	if len(claim) < 20 {
		app.BadRequest(w, r, "Claim must be at least 20 characters")
		return
	}

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpFactCheck)
	if !canProceed {
		if app.SendsJSON(r) {
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

	// Perform the fact-check
	note := runFactCheck(claim)

	// Consume quota
	wallet.ConsumeQuota(acc.ID, wallet.OpFactCheck)

	if note == nil {
		if app.SendsJSON(r) || app.WantsJSON(r) {
			app.RespondJSON(w, map[string]interface{}{
				"status":  "error",
				"message": "Fact-check failed. Please try again.",
			})
			return
		}
		page := app.RenderHTMLForRequest("Fact Check", "Fact Check", `<div class="card center-card-md"><p>Fact-check failed. Please try again.</p><p><a href="/factcheck">Try again</a></p></div>`, r)
		w.Write([]byte(page))
		return
	}

	// JSON response
	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, note)
		return
	}

	// HTML response
	var sb strings.Builder
	sb.WriteString(`<div class="card center-card-md">`)
	sb.WriteString(`<h2 style="margin-top:0;">Fact Check Result</h2>`)
	sb.WriteString(fmt.Sprintf(`<div style="padding:10px 14px;background:#f8f8f8;border-radius:4px;margin-bottom:12px;font-size:13px;color:#555;">%s</div>`, app.RenderString(claim)))
	sb.WriteString(renderCommunityNote(note))
	sb.WriteString(`<p style="margin-top:16px;"><a href="/factcheck">Check another claim</a></p>`)
	sb.WriteString(`</div>`)

	page := app.RenderHTMLForRequest("Fact Check", "Fact Check Result", sb.String(), r)
	w.Write([]byte(page))
}

// renderCommunityNote renders a community note as HTML.
func renderCommunityNote(note *CommunityNote) string {
	if note == nil {
		return ""
	}

	icon := "&#9432;" // info
	label := "Result"
	borderColor := "#888"
	bgColor := "#f8f8f8"

	switch note.Status {
	case "accurate":
		icon = "&#10003;" // checkmark
		label = "Accurate"
		borderColor = "#2d8a4e"
		bgColor = "#f0faf4"
	case "misleading":
		icon = "&#9888;" // warning
		label = "Misleading"
		borderColor = "#d94040"
		bgColor = "#fff5f5"
	case "missing_context":
		icon = "&#9432;" // info
		label = "Missing Context"
		borderColor = "#e0a800"
		bgColor = "#fffdf0"
	case "biased":
		icon = "&#9670;" // diamond
		label = "Framing Bias"
		borderColor = "#7c5cbf"
		bgColor = "#f8f5ff"
	case "none":
		icon = "&#8212;" // em dash
		label = "No Claims Found"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<div style="margin-top:8px;padding:10px 14px;border-left:3px solid %s;background:%s;border-radius:4px;font-size:13px;">`, borderColor, bgColor))
	sb.WriteString(fmt.Sprintf(`<div style="font-weight:600;margin-bottom:4px;color:%s;">%s %s</div>`, borderColor, icon, label))
	sb.WriteString(fmt.Sprintf(`<div style="color:#333;">%s</div>`, app.RenderString(note.Content)))

	if len(note.Sources) > 0 {
		sb.WriteString(`<div style="margin-top:6px;font-size:11px;color:#666;">`)
		for i, src := range note.Sources {
			if i > 0 {
				sb.WriteString(" · ")
			}
			sb.WriteString(fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener" style="color:#666;">%s</a>`, src.URL, src.Title))
		}
		sb.WriteString(`</div>`)
	}

	sb.WriteString(`</div>`)
	return sb.String()
}

// isReferenceSource returns true if the URL is a reference/encyclopedia source
// worth fetching for deeper background context.
func isReferenceSource(url string) bool {
	ref := []string{
		"wikipedia.org",
		"britannica.com",
		"who.int",
		"un.org",
		"icrc.org",
		"amnesty.org",
		"hrw.org",
		"reliefweb.int",
		"worldbank.org",
	}
	lower := strings.ToLower(url)
	for _, r := range ref {
		if strings.Contains(lower, r) {
			return true
		}
	}
	return false
}

// renderFactCheckButton renders a "Fact Check" button for a thread.
func renderFactCheckButton(threadID string) string {
	return fmt.Sprintf(`<form method="POST" action="/social?id=%s&factcheck=true" style="display:inline;margin-left:8px;">
<button type="submit" style="background:none;border:1px solid #ccc;color:#666;font-size:11px;cursor:pointer;padding:2px 8px;border-radius:3px;" title="Fact-check this thread">Fact Check</button>
</form>`, threadID)
}
