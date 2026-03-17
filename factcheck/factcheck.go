package factcheck

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

// Result is the output of a fact-check operation
type Result struct {
	Content   string   `json:"content"`            // the fact-check text
	Sources   []Source `json:"sources,omitempty"`   // reference links
	Status    string   `json:"status"`             // "accurate", "misleading", "missing_context", "none"
	CheckedAt time.Time `json:"checked_at"`
}

// Source is a reference link
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Check performs a fact-check on the given claim text.
// It searches the web for context and uses AI to assess accuracy.
// Returns a Result with status "none" if no verifiable claims are found.
func Check(claim string) *Result {
	claim = strings.TrimSpace(claim)

	// Skip very short claims
	if len(claim) < 20 {
		return &Result{
			Status:    "none",
			Content:   "Text too short to fact-check.",
			CheckedAt: time.Now(),
		}
	}

	// Step 1: Search the web for context
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

	// Step 2: Ask AI to assess the claims
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
		System: `You are a fact-checker. You will receive a claim or statement and web search results.

Your job:
1. Determine if the text contains verifiable factual claims (names, dates, numbers, events, statistics, quotes)
2. If it does, assess whether those claims are supported by the web search results and your knowledge

IMPORTANT: Not every statement needs a note. Skip these:
- Personal opinions, reflections, or questions
- Religious/spiritual discussion without factual claims
- General commentary without specific assertions
- Statements that are clearly accurate based on widely known facts

Respond in ONE of these formats:

If no verifiable claims found:
STATUS: none
NOTE: No verifiable factual claims found.

If claims are clearly accurate:
STATUS: accurate
NOTE: [1-2 sentences confirming the key facts with specifics]

If claims need context or correction:
STATUS: [misleading|missing_context]
NOTE: [1-2 sentences providing factual context, corrections, or important missing information]

Rules:
- Be specific — cite facts, dates, and numbers
- Be neutral — correct the record without taking sides
- Do NOT lecture or moralise
- Do NOT fact-check opinions or predictions
- Write dollar amounts as plain numbers
- CRITICAL: Keep the note under 300 characters`,
		Question: question.String(),
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(prompt)
	if err != nil {
		app.Log("factcheck", "AI failed: %v", err)
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
				if s == "accurate" || s == "misleading" || s == "missing_context" || s == "none" {
					status = s
				}
			}
			if strings.HasPrefix(line, "NOTE:") {
				noteText = strings.TrimSpace(strings.TrimPrefix(line, "NOTE:"))
			}
		}
	} else if strings.HasPrefix(response, "NO_NOTE") {
		return &Result{
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

	return &Result{
		Content:   noteText,
		Sources:   sources,
		Status:    status,
		CheckedAt: time.Now(),
	}
}

// Handler serves the /factcheck endpoint
// GET: renders the fact-check page
// POST: performs a fact-check and returns the result
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handlePage(w, r)
	case "POST":
		handleCheck(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	// JSON API — return usage info
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

func handleCheck(w http.ResponseWriter, r *http.Request) {
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
	result := Check(claim)

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
		page := app.RenderHTMLForRequest("Fact Check", "Fact Check", `<div class="card center-card-md"><p>Fact-check failed. Please try again.</p><p><a href="/factcheck">Try again</a></p></div>`, r)
		w.Write([]byte(page))
		return
	}

	// JSON response
	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, result)
		return
	}

	// HTML response
	var sb strings.Builder
	sb.WriteString(`<div class="card center-card-md">`)
	sb.WriteString(`<h2 style="margin-top:0;">Fact Check Result</h2>`)

	// Show the original claim
	sb.WriteString(fmt.Sprintf(`<div style="padding:10px 14px;background:#f8f8f8;border-radius:4px;margin-bottom:12px;font-size:13px;color:#555;">%s</div>`, app.RenderString(claim)))

	// Show the result
	sb.WriteString(RenderResult(result))

	sb.WriteString(`<p style="margin-top:16px;"><a href="/factcheck">Check another claim</a></p>`)
	sb.WriteString(`</div>`)

	page := app.RenderHTMLForRequest("Fact Check", "Fact Check Result", sb.String(), r)
	w.Write([]byte(page))
}

// RenderResult renders a fact-check result as HTML.
// Exported so social and other packages can use it.
func RenderResult(result *Result) string {
	if result == nil {
		return ""
	}

	icon := "&#9432;" // info
	label := "Result"
	borderColor := "#888"
	bgColor := "#f8f8f8"

	switch result.Status {
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
	case "none":
		icon = "&#8212;" // em dash
		label = "No Claims Found"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<div style="margin-top:8px;padding:10px 14px;border-left:3px solid %s;background:%s;border-radius:4px;font-size:13px;">`, borderColor, bgColor))
	sb.WriteString(fmt.Sprintf(`<div style="font-weight:600;margin-bottom:4px;color:%s;">%s %s</div>`, borderColor, icon, label))
	sb.WriteString(fmt.Sprintf(`<div style="color:#333;">%s</div>`, app.RenderString(result.Content)))

	if len(result.Sources) > 0 {
		sb.WriteString(`<div style="margin-top:6px;font-size:11px;color:#666;">`)
		for i, src := range result.Sources {
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
