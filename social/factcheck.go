package social

import (
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
	"mu/search"
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

// checkClaims searches the web for claims in the content and uses AI
// to assess accuracy. Returns nil if no verifiable claims are found
// (opinions, questions, personal reflections don't need fact-checking).
func checkClaims(title, content string) *CommunityNote {
	fullText := content
	if title != "" {
		fullText = title + "\n\n" + content
	}

	// Skip very short posts — unlikely to contain verifiable claims
	if len(fullText) < 50 {
		return nil
	}

	// Step 1: Search the web for context on the claims
	query := fullText
	if len(query) > 150 {
		// Use the first substantive portion as search query
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

	// Step 2: Ask AI to assess the claims against the web context
	var question strings.Builder
	question.WriteString("## User Post\n\n")
	if title != "" {
		question.WriteString("**" + title + "**\n\n")
	}
	question.WriteString(content)
	question.WriteString("\n\n")

	if len(webContext) > 0 {
		question.WriteString("## Web Search Results\n\n")
		for _, ctx := range webContext {
			question.WriteString("- " + ctx + "\n")
		}
	}

	prompt := &ai.Prompt{
		System: `You are a fact-checker for Mu, an independent truth-seeking platform. You will receive a user's social media post and web search results.

Your job:
1. Determine if the post contains verifiable factual claims (names, dates, numbers, events, statistics, quotes)
2. If it does, assess whether those claims are supported by the web search results and your knowledge

IMPORTANT: Not every post needs a note. Skip these:
- Personal opinions, reflections, or questions
- Religious/spiritual discussion without factual claims
- General commentary without specific assertions
- Posts that are clearly accurate based on widely known facts

Respond in ONE of these formats:

If no verifiable claims or claims are clearly accurate:
NO_NOTE

If claims need context or correction, write a concise community note:
STATUS: [accurate|misleading|missing_context]
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
		app.Log("social", "Fact-check AI failed: %v", err)
		return nil
	}

	response = strings.TrimSpace(app.StripLatexDollars(response))

	// Parse response — the AI should return "NO_NOTE" when no fact-check is needed,
	// but may add extra text after it. Treat any response starting with NO_NOTE as no note.
	if response == "" || strings.HasPrefix(response, "NO_NOTE") {
		return nil
	}

	status := "missing_context"
	noteText := response

	if strings.HasPrefix(response, "STATUS:") {
		lines := strings.SplitN(response, "\n", 3)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "STATUS:") {
				s := strings.TrimSpace(strings.TrimPrefix(line, "STATUS:"))
				s = strings.ToLower(s)
				if s == "accurate" || s == "misleading" || s == "missing_context" {
					status = s
				}
			}
			if strings.HasPrefix(line, "NOTE:") {
				noteText = strings.TrimSpace(strings.TrimPrefix(line, "NOTE:"))
			}
		}
	}

	// Don't create notes for accurate content — only add notes when
	// something is misleading or missing context
	if status == "accurate" {
		return nil
	}

	if noteText == "" || noteText == response {
		// Couldn't parse structured format, use full response as note
		noteText = response
		// Strip any STATUS: prefix from the note text
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

// renderCommunityNote renders a community note as HTML.
// Returns empty string if note is nil.
func renderCommunityNote(note *CommunityNote) string {
	if note == nil {
		return ""
	}

	icon := "&#9432;" // ℹ info circle
	label := "Context"
	borderColor := "#e0a800" // amber
	bgColor := "#fffdf0"

	switch note.Status {
	case "misleading":
		icon = "&#9888;" // ⚠ warning
		label = "Fact Check"
		borderColor = "#d94040"
		bgColor = "#fff5f5"
	case "missing_context":
		icon = "&#9432;" // ℹ info
		label = "Added Context"
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
