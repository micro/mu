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

	// Search the web for context
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

	// Ask AI to assess the claims
	var question strings.Builder
	question.WriteString("## Claim to Fact-Check\n\n")
	question.WriteString(claim)
	question.WriteString("\n\n")

	if len(webContext) > 0 {
		question.WriteString("## Web Search Results\n\n")
		for _, ctx := range webContext {
			question.WriteString("- " + ctx + "\n")
		}
	}

	prompt := &ai.Prompt{
		System: `You are a truth-seeking community note writer. You receive a claim or news article and web search results. Your mission is to help people see past bias, framing, and misinformation to understand what is actually true and who is actually affected.

Your job — analyse the text for:
1. FACTUAL ACCURACY: Are specific claims (names, dates, numbers, events, statistics) correct?
2. FRAMING BIAS: Whose perspective is centred? Whose is erased or dehumanised? Is the language loaded?
3. MISSING CONTEXT: What critical context, history, or human impact is omitted? Are civilian casualties, displacement, or suffering being minimised or ignored?
4. EMOTIONAL MANIPULATION: Is this designed to induce panic, FOMO, outrage, or fear rather than inform?

IMPORTANT: Not every statement needs a note. Skip these:
- Personal reflections, questions, or genuine spiritual discussion
- Straightforward reporting with balanced framing
- Statements that are clearly accurate AND fairly framed

Respond in ONE of these formats:

STATUS: none
NOTE: No issues found.

STATUS: accurate
NOTE: [1-2 sentences confirming key facts]

STATUS: misleading
NOTE: [1-3 sentences correcting factual errors with specifics]

STATUS: missing_context
NOTE: [1-3 sentences providing the critical missing context — human cost, historical background, or the perspective being erased]

STATUS: biased
NOTE: [1-3 sentences identifying the framing bias and providing a more complete picture — e.g. who is actually affected, what the language obscures, what other sources report]

Guidelines:
- Humanise: Behind every conflict are real people — families, children, communities. Centre them.
- Contextualise: Provide historical context that reframes simplistic narratives. Colonial history, occupation, sanctions — these matter.
- Counter FOMO/panic: For markets and crypto, ground the reader. Note what the actual data says vs the emotional framing.
- Be specific — cite facts, dates, numbers, and name sources
- Be direct — state what's missing or wrong without hedging
- Do NOT lecture or moralise — provide facts and context, let people draw conclusions
- Do NOT fact-check opinions or predictions
- Write dollar amounts as plain numbers
- CRITICAL: Keep the note under 400 characters`,
		Question: question.String(),
		Priority: ai.PriorityLow,
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

// renderFactCheckButton renders a "Fact Check" button for a thread.
func renderFactCheckButton(threadID string) string {
	return fmt.Sprintf(`<form method="POST" action="/social?id=%s&factcheck=true" style="display:inline;margin-left:8px;">
<button type="submit" style="background:none;border:1px solid #ccc;color:#666;font-size:11px;cursor:pointer;padding:2px 8px;border-radius:3px;" title="Fact-check this thread">Fact Check</button>
</form>`, threadID)
}
