package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
	"mu/tools"
)

// StepCallback is called when a step completes (for streaming)
type StepCallback func(step *Step, final bool)

// ToolResult is what a tool returns (for UI rendering)
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	HTML    string      `json:"html,omitempty"`
	Action  string      `json:"action,omitempty"`
	URL     string      `json:"url,omitempty"`
}

// Agent orchestrates tasks across mu services
type Agent struct {
	userID   string
	maxSteps int
}

// Step represents one step in the agent's execution
type Step struct {
	Reasoning  string      `json:"reasoning"`
	Tool       string      `json:"tool"`
	Parameters interface{} `json:"parameters"`
	Result     *ToolResult `json:"result,omitempty"`
}

// Result is the final output from the agent
type Result struct {
	Success  bool    `json:"success"`
	Answer   string  `json:"answer"`
	Steps    []*Step `json:"steps"`
	HTML     string  `json:"html,omitempty"`
	Action   string  `json:"action,omitempty"`
	URL      string  `json:"url,omitempty"`
	Duration string  `json:"duration"`
}

// New creates a new agent for a user
func New(userID string) *Agent {
	return &Agent{
		userID:   userID,
		maxSteps: 5,
	}
}

// Run executes the agent on a task
func (a *Agent) Run(task string) *Result {
	return a.RunStreaming(task, nil)
}

// RunStreaming executes the agent and calls the callback after each step
func (a *Agent) RunStreaming(task string, onStep StepCallback) *Result {
	start := time.Now()

	result := &Result{
		Steps: []*Step{},
	}

	systemPrompt := a.buildSystemPrompt()

	var history ai.History
	currentPrompt := task

	for i := 0; i < a.maxSteps; i++ {
		llmPrompt := &ai.Prompt{
			System:   systemPrompt,
			Question: currentPrompt,
			Context:  history,
			Priority: ai.PriorityHigh,
			Provider: ai.ProviderAnthropic, // Use Anthropic for speed
		}

		response, err := ai.Ask(llmPrompt)
		if err != nil {
			result.Success = false
			result.Answer = fmt.Sprintf("Error: %v", err)
			break
		}

		step, err := a.parseStep(response)
		if err != nil {
			app.Log("agent", "Failed to parse step: %v, response: %s", err, response)
			result.Answer = response
			result.Success = true
			break
		}

		result.Steps = append(result.Steps, step)

		// Check for final_answer (special case)
		if step.Tool == "final_answer" {
			params, _ := step.Parameters.(map[string]interface{})
			if answer, ok := params["answer"].(string); ok {
				result.Answer = answer
			}
			result.Success = true
			step.Result = &ToolResult{Success: true, Data: result.Answer}
			if onStep != nil {
				onStep(step, true)
			}
			break
		}

		// Execute tool from registry
		toolResult := a.executeTool(step)
		step.Result = toolResult

		// Propagate action/URL from tool results
		if toolResult != nil && toolResult.Action != "" {
			result.Action = toolResult.Action
			result.URL = toolResult.URL
			result.HTML = toolResult.HTML
		}

		// Stream the step to client
		if onStep != nil {
			onStep(step, false)
		}

		// Add to history for next iteration
		resultJSON, _ := json.Marshal(toolResult)
		history = append(history, ai.Message{
			Prompt: currentPrompt,
			Answer: response,
		})
		currentPrompt = fmt.Sprintf("Tool result: %s\n\nWhat's next?", string(resultJSON))
	}

	result.Duration = time.Since(start).String()
	return result
}

// executeTool calls a tool from the registry
func (a *Agent) executeTool(step *Step) *ToolResult {
	tool := tools.Get(step.Tool)
	if tool == nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Unknown tool: %s", step.Tool),
		}
	}

	// Convert parameters
	params, _ := step.Parameters.(map[string]interface{})
	if params == nil {
		params = make(map[string]interface{})
	}

	// Create context with user
	ctx := tools.WithUser(context.Background(), a.userID)

	// Call the tool
	data, err := tools.Call(ctx, step.Tool, params)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   err.Error(),
		}
	}

	// Check if result has action/URL (for navigation)
	result := &ToolResult{
		Success: true,
		Data:    data,
	}

	// Extract action/URL if present in data
	if m, ok := data.(map[string]interface{}); ok {
		if action, ok := m["_action"].(string); ok {
			result.Action = action
		}
		if url, ok := m["_url"].(string); ok {
			result.URL = url
		}
		if html, ok := m["_html"].(string); ok {
			result.HTML = html
		}
	}

	return result
}

// buildSystemPrompt creates the system prompt with available tools
func (a *Agent) buildSystemPrompt() string {
	toolsJSON := a.buildToolsJSON()
	return fmt.Sprintf(`You are a task execution agent for the Mu platform. You execute specific tasks using the available tools.

Available tools:
%s

You must respond with valid JSON in this exact format:
{
  "reasoning": "Why this step is needed",
  "tool": "tool_name",
  "parameters": { "param1": "value1" }
}

Rules:
1. You execute TASKS - playing videos, searching news, creating apps, sending emails, checking prices, saving notes, etc.
2. If the user asks a general question, philosophical question, or wants conversation, use final_answer to say: "I help with tasks like playing videos, searching news, or creating apps. For questions and conversation, try Chat."
3. Do NOT search for answers to general questions - just politely redirect to Chat.
4. Use one tool at a time
5. After each tool result, decide if you need more steps or can provide final_answer
6. Always end with final_answer when done
7. Be concise - minimize steps
8. IMPORTANT: When providing final_answer, include the ACTUAL DATA from tool results (headlines, prices, search results, etc.) - don't just say you found them, SHOW them

Current user: %s`, toolsJSON, a.userID)
}

// buildToolsJSON creates the tools description for the LLM
func (a *Agent) buildToolsJSON() string {
	registeredTools := tools.List()

	var toolDefs []map[string]interface{}

	// Add registered tools
	for _, t := range registeredTools {
		params := make(map[string]interface{})
		for name, p := range t.Input {
			params[name] = map[string]interface{}{
				"type":        p.Type,
				"description": p.Description,
				"required":    p.Required,
			}
		}

		toolDefs = append(toolDefs, map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		})
	}

	// Add final_answer tool (always available)
	toolDefs = append(toolDefs, map[string]interface{}{
		"name":        "final_answer",
		"description": "Provide the final answer to the user. Use this when you have completed the task or gathered all needed information.",
		"parameters": map[string]interface{}{
			"answer": map[string]interface{}{
				"type":        "string",
				"description": "The final answer or response to give the user",
				"required":    true,
			},
		},
	})

	b, _ := json.MarshalIndent(toolDefs, "", "  ")
	return string(b)
}

// parseStep parses the LLM's JSON response into a Step
func (a *Agent) parseStep(response string) (*Step, error) {
	// Clean up response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Try to find JSON in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		response = response[start : end+1]
	}

	// Fix invalid JSON: escape literal newlines inside string values
	// LLMs sometimes output newlines in JSON strings which is invalid
	response = fixJSONNewlines(response)

	var step Step
	if err := json.Unmarshal([]byte(response), &step); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return &step, nil
}

// fixJSONNewlines escapes literal newlines inside JSON string values
func fixJSONNewlines(s string) string {
	var result strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if escaped {
			result.WriteByte(c)
			escaped = false
			continue
		}

		if c == '\\' && inString {
			result.WriteByte(c)
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			result.WriteByte(c)
			continue
		}

		if inString && c == '\n' {
			result.WriteString("\\n")
			continue
		}

		if inString && c == '\r' {
			result.WriteString("\\r")
			continue
		}

		if inString && c == '\t' {
			result.WriteString("\\t")
			continue
		}

		result.WriteByte(c)
	}

	return result.String()
}
