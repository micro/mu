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

// StartWorker subscribes to task events and runs them using the agent's tools.
func StartWorker() {
	taskSub := event.Subscribe(event.EventTaskCreated)
	retrySub := event.Subscribe(event.EventTaskRetry)

	go func() {
		for evt := range taskSub.Chan {
			postID, _ := evt.Data["post_id"].(string)
			if postID == "" {
				continue
			}
			go runTask(postID, "")
		}
	}()

	go func() {
		for evt := range retrySub.Chan {
			postID, _ := evt.Data["post_id"].(string)
			feedback, _ := evt.Data["feedback"].(string)
			if postID == "" {
				continue
			}
			go runTask(postID, feedback)
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
	if feedback != "" {
		prompt += "\n\nFeedback from previous attempt:\n" + feedback
	}

	work.AddLog(postID, "plan", "Planning task...", 0)

	// Step 1: Plan — ask AI what tools to use
	planResult, err := ai.Ask(&ai.Prompt{
		System: "You are an AI agent that completes tasks. Given a task description, output ONLY a JSON array of tool calls.\n\n" +
			agentToolsDesc +
			"\n\nOutput format: [{\"tool\":\"tool_name\",\"args\":{}}]\n" +
			"If the task asks to build an app, use apps_build with the full description as the prompt.\n" +
			"If the task asks to write a blog post, use blog_create.\n" +
			"If the task asks for research, use web_search, news, or chat.\n" +
			"Use at most 5 tool calls. Output [] if no tools needed.",
		Question: prompt,
		Priority: ai.PriorityHigh,
		Caller:   "work-agent-plan",
	})
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

	// Step 2: Execute tools as the task author
	var results []string
	for _, tc := range toolCalls {
		if tc.Tool == "" {
			continue
		}

		// Check budget
		remaining := work.BudgetRemaining(postID)
		if remaining <= 0 && post.Cost > 0 {
			work.AddLog(postID, "budget", "Budget exceeded", 0)
			break
		}

		work.AddLog(postID, "tool", fmt.Sprintf("Running %s...", tc.Tool), 3)

		text, isErr, execErr := api.ExecuteToolAs(post.AuthorID, tc.Tool, tc.Args)
		if execErr != nil || isErr {
			work.AddLog(postID, "error", fmt.Sprintf("%s failed: %v", tc.Tool, execErr), 0)
			continue
		}

		if len(text) > 4000 {
			text = text[:4000] + "..."
		}
		results = append(results, fmt.Sprintf("### %s\n%s", tc.Tool, text))

		// Extract app URL from apps_build result
		if tc.Tool == "apps_build" {
			var appResult struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			}
			if json.Unmarshal([]byte(text), &appResult) == nil && appResult.Slug != "" {
				delivery := fmt.Sprintf("%s — /apps/%s/run", appResult.Name, appResult.Slug)
				work.SetDelivery(postID, delivery)
				work.AddLog(postID, "tool", fmt.Sprintf("Built app: %s → /apps/%s/run", appResult.Name, appResult.Slug), 0)
			} else {
				work.AddLog(postID, "tool", fmt.Sprintf("%s — done", tc.Tool), 0)
			}
		} else {
			work.AddLog(postID, "tool", fmt.Sprintf("%s — done", tc.Tool), 0)
		}
	}

	// Step 3: Synthesise result
	work.AddLog(postID, "synth", "Composing result...", 0)

	answer, err := ai.Ask(&ai.Prompt{
		System:   "You are a helpful assistant completing a task. Summarise the results of your work. Use markdown.",
		Rag:      results,
		Question: "Task: " + prompt + "\n\nSummarise what was accomplished.",
		Priority: ai.PriorityHigh,
		Caller:   "work-agent-synth",
	})
	if err != nil {
		work.AddLog(postID, "error", "Synthesis failed: "+err.Error(), 0)
		failTask(postID)
		return
	}

	// Deliver — if an app was built, keep the app delivery and append the summary
	existing := work.GetPost(postID)
	if existing != nil && existing.Delivery != "" && strings.Contains(existing.Delivery, " — /apps/") {
		// App already delivered — append summary below
		work.SetDelivery(postID, existing.Delivery+"\n\n"+answer)
	} else {
		work.SetDelivery(postID, answer)
	}
	work.SetStatus(postID, "delivered")
	work.AddLog(postID, "complete", "Task delivered", 0)

	// Notify
	if work.Notify != nil {
		work.Notify(post.AuthorID, "Task completed: "+post.Title,
			fmt.Sprintf("Your task has been completed.\n\n[Review →](/work/%s)", postID), postID)
	}
}

func failTask(postID string) {
	work.SetStatus(postID, "open")
	post := work.GetPost(postID)
	if post != nil && work.Notify != nil {
		work.Notify(post.AuthorID, "Task failed: "+post.Title,
			"The agent could not complete this task.", postID)
	}
}
