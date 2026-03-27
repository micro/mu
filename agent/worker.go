package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/event"
	"mu/work"
)

const (
	creditPerCall     = 3
	maxVerifyAttempts = 3
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

// runTask executes a work task using the agent's tool-calling loop.
func runTask(postID, feedback string) {
	post := work.GetPost(postID)
	if post == nil {
		return
	}

	prompt := post.Description
	isRetry := feedback != ""

	// On retry with an existing app, tell the agent to edit it
	if isRetry && post.AppSlug != "" {
		prompt = fmt.Sprintf("Edit the existing app '%s' (slug: %s).\n\nOriginal description:\n%s\n\nFeedback — what needs to change:\n%s\n\nUse apps_edit with the slug and updated HTML.",
			post.AppSlug, post.AppSlug, post.Description, feedback)
	} else if isRetry {
		prompt += "\n\nFeedback from previous attempt:\n" + feedback
	}

	// Step 1: Plan
	work.AddLog(postID, "plan", "Planning task...", 0)

	planResult, err := callAI(post, postID, "work-agent-plan",
		"You are an AI agent that completes tasks. Given a task description, output ONLY a JSON array of tool calls.\n\n"+
			agentToolsDesc+
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\n"+
			"If the task asks to build an app, use apps_build with the full description as the prompt.\n"+
			"If editing an existing app, use apps_edit with the slug and new HTML.\n"+
			"If the task asks to write a blog post, use blog_create.\n"+
			"If the task asks for research, use web_search, news, or chat.\n"+
			"Use at most 5 tool calls. Output [] if no tools needed.",
		prompt)
	if err != nil {
		work.AddLog(postID, "error", "Planning failed: "+err.Error(), 0)
		failTask(postID)
		return
	}

	// Parse tool calls
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

	// Step 2: Execute tools
	var results []string
	var builtAppSlug, builtAppName string

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
			work.AddLog(postID, "error", fmt.Sprintf("%s failed: %v", tc.Tool, execErr), 0)
			continue
		}

		if len(text) > 4000 {
			text = text[:4000] + "..."
		}
		results = append(results, fmt.Sprintf("### %s\n%s", tc.Tool, text))

		// Track app builds
		if tc.Tool == "apps_build" || tc.Tool == "apps_edit" {
			var appResult struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			}
			if json.Unmarshal([]byte(text), &appResult) == nil && appResult.Slug != "" {
				builtAppSlug = appResult.Slug
				builtAppName = appResult.Name
				work.AddLog(postID, "build", fmt.Sprintf("App ready: %s → /apps/%s/run", appResult.Name, appResult.Slug), 0)
			}
		} else {
			work.AddLog(postID, "tool", fmt.Sprintf("%s — done", tc.Tool), 0)
		}
	}

	if len(results) == 0 {
		work.AddLog(postID, "error", "No tools succeeded", 0)
		failTask(postID)
		return
	}

	// Step 3: Verify app builds (iterative)
	if builtAppSlug != "" {
		builtAppSlug, builtAppName = verifyAndFix(post, postID, builtAppSlug, builtAppName)
	}

	// Step 4: Synthesise result
	work.AddLog(postID, "synth", "Composing result...", 0)

	answer, err := callAI(post, postID, "work-agent-synth",
		"You are a helpful assistant completing a task. Summarise the results of your work. Use markdown.",
		"Task: "+prompt+"\n\nSummarise what was accomplished.")
	if err != nil {
		answer = "Task completed."
	}

	// Set delivery
	if builtAppSlug != "" {
		work.SetDelivery(postID, answer, builtAppSlug)
	} else {
		work.SetDelivery(postID, answer, "")
	}
	work.SetStatus(postID, "delivered")
	work.AddLog(postID, "complete", "Task delivered", 0)

	if work.Notify != nil {
		work.Notify(post.AuthorID, "Task completed: "+post.Title,
			fmt.Sprintf("Your task has been completed.\n\n[Review →](/work/%s)", postID), postID)
	}
}

// verifyAndFix runs the verify → fix loop for an app build.
func verifyAndFix(post *work.Post, postID, slug, name string) (string, string) {
	for i := 0; i < maxVerifyAttempts; i++ {
		if !spendCredit(post, postID) {
			break
		}

		work.AddLog(postID, "verify", fmt.Sprintf("Verifying app (attempt %d)...", i+1), creditPerCall)

		// Ask AI to review the app
		app := getAppHTML(slug)
		if app == "" {
			work.AddLog(postID, "error", "Could not read app HTML", 0)
			break
		}

		reviewPrompt := fmt.Sprintf("Requirements:\n%s\n\nApp HTML (first 3000 chars):\n%s",
			post.Description, truncateStr(app, 3000))

		result, err := callAI(post, postID, "work-verify",
			`You are a QA reviewer. Check if this app meets the requirements.
Reply with ONLY one of:
- "PASS" if the app works correctly
- "FAIL: <brief issues>" if there are problems
Focus on functional issues, not style.`,
			reviewPrompt)
		if err != nil {
			break
		}

		result = strings.TrimSpace(result)
		if strings.HasPrefix(strings.ToUpper(result), "PASS") {
			work.AddLog(postID, "verify", "App verified ✓", 0)
			return slug, name
		}

		issues := strings.TrimPrefix(result, "FAIL: ")
		work.AddLog(postID, "verify", "Issues found: "+issues, 0)

		// Fix
		if !spendCredit(post, postID) {
			break
		}

		work.AddLog(postID, "fix", "Fixing issues...", creditPerCall)

		fixPrompt := fmt.Sprintf("Fix this app. Issues:\n%s\n\nRequirements:\n%s\n\nCurrent HTML:\n%s",
			issues, post.Description, truncateStr(app, 3000))

		fixResult, err := callAI(post, postID, "work-fix",
			"You are an app builder. Output ONLY the complete fixed HTML document. No explanation, no markdown fences, just the HTML.",
			fixPrompt)
		if err != nil {
			work.AddLog(postID, "error", "Fix failed: "+err.Error(), 0)
			break
		}

		// Update the app in place
		_, isErr, _ := api.ExecuteToolAs(post.AuthorID, "apps_edit", map[string]any{
			"slug": slug,
			"html": fixResult,
		})
		if isErr {
			work.AddLog(postID, "error", "Could not update app", 0)
			break
		}

		work.AddLog(postID, "fix", "Applied fix", 0)
	}

	return slug, name
}

// callAI makes an AI call, checking budget first.
func callAI(post *work.Post, postID, caller, system, question string) (string, error) {
	return ai.Ask(&ai.Prompt{
		System:   system,
		Question: question,
		Priority: ai.PriorityHigh,
		Caller:   caller,
	})
}

// spendCredit deducts credits for a tool/AI call. Returns false if budget exceeded.
func spendCredit(post *work.Post, postID string) bool {
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

// getAppHTML reads the HTML of an app by slug via the apps_read tool.
func getAppHTML(slug string) string {
	text, isErr, err := api.ExecuteToolAs("micro", "apps_read", map[string]any{"slug": slug})
	if err != nil || isErr {
		return ""
	}
	var result struct {
		HTML string `json:"html"`
	}
	json.Unmarshal([]byte(text), &result)
	return result.HTML
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func failTask(postID string) {
	work.SetStatus(postID, "open")
	post := work.GetPost(postID)
	if post != nil && work.Notify != nil {
		work.Notify(post.AuthorID, "Task failed: "+post.Title,
			"The agent could not complete this task.", postID)
	}
}
