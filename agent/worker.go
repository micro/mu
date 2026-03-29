package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/apps"
	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/work"
)

const (
	creditPerCall     = 3
	maxFixAttempts    = 3
)

// StartWorker subscribes to task events and runs them using the agent's tools.
func StartWorker() {
	taskSub := event.Subscribe(event.EventTaskCreated)
	retrySub := event.Subscribe(event.EventTaskRetry)

	go func() {
		for evt := range taskSub.Chan {
			postID, _ := evt.Data["post_id"].(string)
			if postID != "" {
				go runTask(postID, "")
			}
		}
	}()

	go func() {
		for evt := range retrySub.Chan {
			postID, _ := evt.Data["post_id"].(string)
			feedback, _ := evt.Data["feedback"].(string)
			if postID != "" {
				go runTask(postID, feedback)
			}
		}
	}()
}

// runTask executes a work task. For app tasks it builds, verifies, and fixes.
// For other tasks it uses the tool planning approach.
func runTask(postID, feedback string) {
	post := work.GetPost(postID)
	if post == nil {
		return
	}

	desc := post.Description

	// Determine if this is an app-building task
	isAppTask := looksLikeAppTask(desc)

	if isAppTask {
		runAppTask(post, postID, feedback)
	} else {
		runGeneralTask(post, postID, feedback)
	}
}

// looksLikeAppTask checks if the task description is asking to build an app.
func looksLikeAppTask(desc string) bool {
	lower := strings.ToLower(desc)
	for _, keyword := range []string{"build an app", "build a app", "create an app", "create a app",
		"build app", "make an app", "make a app", "weather app", "timer app", "calculator",
		"converter", "generator", "tracker", "editor", "viewer", "tester"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// runAppTask builds an app, verifies it, and iterates until it works.
func runAppTask(post *work.Post, postID, feedback string) {
	// Step 1: Build or edit the app
	if feedback != "" && post.AppSlug != "" {
		// Retry: edit existing app based on feedback
		editApp(post, postID, feedback)
	} else {
		// First build
		buildApp(post, postID)
	}

	post = work.GetPost(postID) // refresh
	if post == nil || post.AppSlug == "" {
		return // build failed
	}

	// Step 2: Verify and fix loop
	for i := 0; i < maxFixAttempts; i++ {
		if !spendCredit(post, postID) {
			break
		}

		work.AddLog(postID, "verify", fmt.Sprintf("Reviewing app (attempt %d)...", i+1), creditPerCall)

		issues := verifyApp(post, postID)
		if issues == "" {
			work.AddLog(postID, "verify", "App verified", 0)
			break
		}

		work.AddLog(postID, "verify", "Issues: "+issues, 0)

		// Fix
		if !spendCredit(post, postID) {
			break
		}
		work.AddLog(postID, "fix", "Fixing: "+issues, creditPerCall)
		fixApp(post, postID, issues)
	}

	// Deliver
	post = work.GetPost(postID)
	if post == nil {
		return
	}
	work.SetDelivery(postID, fmt.Sprintf("Built app: [%s](/apps/%s) — [Launch →](/apps/%s/run)", post.AppSlug, post.AppSlug, post.AppSlug), post.AppSlug)
	work.SetStatus(postID, "delivered")
	work.AddLog(postID, "complete", "App delivered", 0)
	notifyComplete(post, postID)
}

// buildApp calls apps_build to create a new app.
func buildApp(post *work.Post, postID string) {
	if !spendCredit(post, postID) {
		failTask(postID)
		return
	}

	work.AddLog(postID, "build", "Building app...", creditPerCall)

	text, isErr, execErr := api.ExecuteToolAs(post.AuthorID, "apps_build", map[string]any{
		"prompt": post.Description,
	})
	if execErr != nil || isErr {
		errMsg := errText(text, execErr)
		work.AddLog(postID, "error", "Build failed: "+errMsg, 0)
		failTask(postID)
		return
	}

	var result struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(text), &result) != nil || result.Slug == "" {
		work.AddLog(postID, "error", "Build returned invalid result", 0)
		failTask(postID)
		return
	}

	work.SetDelivery(postID, "", result.Slug)
	work.AddLog(postID, "build", fmt.Sprintf("Built: %s → /apps/%s/run", result.Name, result.Slug), 0)
}

// editApp edits an existing app based on user feedback.
func editApp(post *work.Post, postID, feedback string) {
	if !spendCredit(post, postID) {
		return
	}

	work.AddLog(postID, "build", "Updating app with feedback...", creditPerCall)

	// Get current HTML
	currentHTML := readAppHTML(post.AuthorID, post.AppSlug)
	if currentHTML == "" {
		work.AddLog(postID, "error", "Could not read current app", 0)
		// Fall back to full rebuild
		buildApp(post, postID)
		return
	}

	// Ask AI to produce updated HTML
	newHTML, err := ai.Ask(&ai.Prompt{
		System: apps.BuilderSystemPrompt() +
			"\n\nYou are updating an existing app. Output ONLY the complete updated HTML document. No explanation, no markdown fences.",
		Question: fmt.Sprintf("Current app HTML:\n%s\n\nUser feedback — what to change:\n%s",
			truncateStr(currentHTML, 8000), feedback),
		Priority: ai.PriorityHigh,
		Caller:   "work-edit-app",
	})
	if err != nil {
		work.AddLog(postID, "error", "Edit failed: "+err.Error(), 0)
		return
	}

	newHTML = cleanHTML(newHTML)
	if newHTML == "" {
		work.AddLog(postID, "error", "AI returned empty HTML", 0)
		return
	}

	// Update in place
	_, isErr, _ := api.ExecuteToolAs(post.AuthorID, "apps_edit", map[string]any{
		"slug": post.AppSlug,
		"html": newHTML,
	})
	if isErr {
		work.AddLog(postID, "error", "Could not save updated app", 0)
		return
	}

	work.AddLog(postID, "build", "App updated", 0)
}

// verifyApp tests the app by checking structure and executing API calls.
// Returns issues string (empty = passed).
func verifyApp(post *work.Post, postID string) string {
	result := apps.TestApp(post.AppSlug, post.AuthorID)
	if result == nil {
		return "Could not test app"
	}

	if len(result.Issues) > 0 {
		return strings.Join(result.Issues, "; ")
	}

	return ""
}

// fixApp fixes issues in an existing app.
func fixApp(post *work.Post, postID, issues string) {
	html := readAppHTML(post.AuthorID, post.AppSlug)
	if html == "" {
		work.AddLog(postID, "error", "Could not read app HTML for fix", 0)
		return
	}

	newHTML, err := ai.Ask(&ai.Prompt{
		System: apps.BuilderSystemPrompt() +
			"\n\nYou are fixing an existing app. Output ONLY the complete fixed HTML document. No explanation, no markdown fences, just the HTML starting with <!DOCTYPE html>.",
		Question: fmt.Sprintf("Issues to fix:\n%s\n\nOriginal requirements:\n%s\n\nCurrent HTML:\n%s",
			issues, post.Description, truncateStr(html, 6000)),
		Priority: ai.PriorityHigh,
		Caller:   "work-fix",
	})
	if err != nil {
		work.AddLog(postID, "error", "Fix generation failed: "+err.Error(), 0)
		return
	}

	newHTML = cleanHTML(newHTML)
	if newHTML == "" {
		work.AddLog(postID, "error", "Fix returned empty HTML", 0)
		return
	}

	_, isErr, _ := api.ExecuteToolAs(post.AuthorID, "apps_edit", map[string]any{
		"slug": post.AppSlug,
		"html": newHTML,
	})
	if isErr {
		work.AddLog(postID, "error", "Could not save fix", 0)
		return
	}

	work.AddLog(postID, "fix", "Fix applied", 0)
}

// runGeneralTask handles non-app tasks using the tool planning approach.
func runGeneralTask(post *work.Post, postID, feedback string) {
	prompt := post.Description
	if feedback != "" {
		prompt += "\n\nFeedback from previous attempt:\n" + feedback
	}

	work.AddLog(postID, "plan", "Planning task...", 0)

	planResult, err := ai.Ask(&ai.Prompt{
		System: "You are an AI agent. Given a task, output ONLY a JSON array of tool calls.\n\n" +
			agentToolsDesc +
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\nUse at most 5 tools.",
		Question: prompt,
		Priority: ai.PriorityHigh,
		Caller:   "work-agent-plan",
	})
	if err != nil {
		work.AddLog(postID, "error", "Planning failed: "+err.Error(), 0)
		failTask(postID)
		return
	}

	type toolCall struct {
		Tool string         `json:"tool"`
		Args map[string]any `json:"args"`
	}
	var toolCalls []toolCall
	planJSON := extractJSONArray(planResult)
	if err := json.Unmarshal([]byte(planJSON), &toolCalls); err != nil || len(toolCalls) == 0 {
		work.AddLog(postID, "error", "No tools planned", 0)
		failTask(postID)
		return
	}

	work.AddLog(postID, "plan", fmt.Sprintf("Planned %d tool calls", len(toolCalls)), 0)

	var results []string
	for _, tc := range toolCalls {
		if tc.Tool == "" {
			continue
		}
		if !spendCredit(post, postID) {
			break
		}

		work.AddLog(postID, "tool", fmt.Sprintf("Running %s...", tc.Tool), creditPerCall)

		text, isErr, execErr := api.ExecuteToolAs(post.AuthorID, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			work.AddLog(postID, "error", fmt.Sprintf("%s failed: %s", tc.Tool, errText(text, execErr)), 0)
			continue
		}

		if len(text) > 4000 {
			text = text[:4000] + "..."
		}
		results = append(results, fmt.Sprintf("### %s\n%s", tc.Tool, text))
		work.AddLog(postID, "tool", fmt.Sprintf("%s — done", tc.Tool), 0)
	}

	if len(results) == 0 {
		work.AddLog(postID, "error", "No tools succeeded", 0)
		failTask(postID)
		return
	}

	// Synthesise
	work.AddLog(postID, "synth", "Composing result...", 0)
	answer, err := ai.Ask(&ai.Prompt{
		System:   "Summarise the results. Use markdown.",
		Rag:      results,
		Question: "Task: " + prompt,
		Priority: ai.PriorityHigh,
		Caller:   "work-agent-synth",
	})
	if err != nil {
		answer = "Task completed."
	}

	work.SetDelivery(postID, answer, "")
	work.SetStatus(postID, "delivered")
	work.AddLog(postID, "complete", "Task delivered", 0)
	notifyComplete(post, postID)
}

// --- helpers ---

func readAppHTML(authorID, slug string) string {
	a := getAppBySlug(authorID, slug)
	if a == nil {
		return ""
	}
	return a.HTML
}

func getAppBySlug(authorID, slug string) *struct {
	HTML string `json:"html"`
	Name string `json:"name"`
	Slug string `json:"slug"`
} {
	text, isErr, err := api.ExecuteToolAs(authorID, "apps_read", map[string]any{"slug": slug})
	if err != nil || isErr {
		return nil
	}
	var result struct {
		HTML string `json:"html"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if json.Unmarshal([]byte(text), &result) != nil {
		return nil
	}
	return &result
}

// cleanHTML strips markdown fences and leading/trailing whitespace from AI HTML output.
func cleanHTML(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown code fences
	if strings.HasPrefix(s, "```html") {
		s = strings.TrimPrefix(s, "```html")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	s = strings.TrimSpace(s)
	// Must start with DOCTYPE or <html
	if !strings.HasPrefix(strings.ToLower(s), "<!doctype") && !strings.HasPrefix(strings.ToLower(s), "<html") {
		return ""
	}
	return s
}

func errText(text string, err error) string {
	if err != nil {
		return err.Error()
	}
	if len(text) > 200 {
		return text[:200]
	}
	return text
}

func spendCredit(post *work.Post, postID string) bool {
	// Admin bypasses budget and credit checks
	if acc, err := auth.GetAccount(post.AuthorID); err == nil && acc.Admin {
		return true
	}
	if post.Cost > 0 && work.BudgetRemaining(postID) < creditPerCall {
		work.AddLog(postID, "budget", "Budget exceeded", 0)
		return false
	}
	if work.SpendCredits != nil {
		if err := work.SpendCredits(post.AuthorID, creditPerCall, "work_agent"); err != nil {
			work.AddLog(postID, "budget", "Insufficient credits", 0)
			return false
		}
	}
	return true
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func notifyComplete(post *work.Post, postID string) {
	// No email — user sees live progress on the task page
}

func failTask(postID string) {
	work.SetStatus(postID, "open")
}
