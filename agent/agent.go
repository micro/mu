package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mu/ai"
	"mu/app"
)

// StepCallback is called when a step completes (for streaming)
type StepCallback func(step *Step, final bool)

// Tool represents a capability the agent can use
type Tool struct {
	Name        string                                                   `json:"name"`
	Description string                                                   `json:"description"`
	Parameters  map[string]ToolParam                                     `json:"parameters"`
	Execute     func(params map[string]interface{}) (*ToolResult, error) `json:"-"`
}

// ToolParam describes a parameter for a tool
type ToolParam struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolResult is what a tool returns
type ToolResult struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	// For UI rendering
	HTML   string `json:"html,omitempty"`
	Action string `json:"action,omitempty"` // "play", "navigate", "display", etc.
	URL    string `json:"url,omitempty"`
}

// Agent orchestrates tasks across mu services
type Agent struct {
	tools    map[string]*Tool
	userID   string
	maxSteps int
}

// Step represents one step in the agent's execution
type Step struct {
	Thought    string      `json:"thought"`
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
	a := &Agent{
		tools:    make(map[string]*Tool),
		userID:   userID,
		maxSteps: 5,
	}

	// Register all tools
	a.registerTools()

	return a
}

// registerTools sets up all available tools
func (a *Agent) registerTools() {
	// Video tools
	a.tools["video_search"] = &Tool{
		Name:        "video_search",
		Description: "Search for videos on YouTube. Use this to find videos by topic, channel name, or content.",
		Parameters: map[string]ToolParam{
			"query": {Type: "string", Description: "Search query (e.g., 'bingo songs', 'cooking tutorial')", Required: true},
		},
		Execute: a.videoSearch,
	}

	a.tools["video_play"] = &Tool{
		Name:        "video_play",
		Description: "Play a specific video by its ID. Returns a URL to play the video.",
		Parameters: map[string]ToolParam{
			"video_id": {Type: "string", Description: "YouTube video ID", Required: true},
		},
		Execute: a.videoPlay,
	}

	// News tools
	a.tools["news_search"] = &Tool{
		Name:        "news_search",
		Description: "Search news articles by topic or keywords. Returns headlines and summaries.",
		Parameters: map[string]ToolParam{
			"query": {Type: "string", Description: "Search query (e.g., 'technology', 'bitcoin', 'sports')", Required: true},
		},
		Execute: a.newsSearch,
	}

	a.tools["news_read"] = &Tool{
		Name:        "news_read",
		Description: "Get the full content of a news article by its URL or ID.",
		Parameters: map[string]ToolParam{
			"url": {Type: "string", Description: "Article URL", Required: true},
		},
		Execute: a.newsRead,
	}

	// Apps tools
	a.tools["app_create"] = &Tool{
		Name:        "app_create",
		Description: "Create a new micro app from a description. The app will be generated with HTML/CSS/JS.",
		Parameters: map[string]ToolParam{
			"name":        {Type: "string", Description: "Name for the app", Required: true},
			"description": {Type: "string", Description: "Detailed description of what the app should do", Required: true},
		},
		Execute: a.appCreate,
	}

	a.tools["app_modify"] = &Tool{
		Name:        "app_modify",
		Description: "Modify an existing app with new instructions.",
		Parameters: map[string]ToolParam{
			"app_id":      {Type: "string", Description: "ID of the app to modify", Required: true},
			"instruction": {Type: "string", Description: "What changes to make", Required: true},
		},
		Execute: a.appModify,
	}

	a.tools["app_list"] = &Tool{
		Name:        "app_list",
		Description: "List user's apps or search public apps.",
		Parameters: map[string]ToolParam{
			"query": {Type: "string", Description: "Optional search query", Required: false},
		},
		Execute: a.appList,
	}

	// Market tools
	a.tools["market_price"] = &Tool{
		Name:        "market_price",
		Description: "Get current market prices for stocks, crypto, or commodities.",
		Parameters: map[string]ToolParam{
			"symbol": {Type: "string", Description: "Ticker symbol (e.g., 'BTC', 'AAPL', 'gold')", Required: true},
		},
		Execute: a.marketPrice,
	}

	// Notes tools
	a.tools["save_note"] = &Tool{
		Name:        "save_note",
		Description: "Save a note for the user. Use for quick capture of ideas, reminders, or any text the user wants to save.",
		Parameters: map[string]ToolParam{
			"content": {Type: "string", Description: "The note content to save", Required: true},
			"title":   {Type: "string", Description: "Optional title for the note", Required: false},
			"tags":    {Type: "string", Description: "Optional comma-separated tags", Required: false},
		},
		Execute: a.saveNote,
	}

	a.tools["search_notes"] = &Tool{
		Name:        "search_notes",
		Description: "Search the user's notes by keyword.",
		Parameters: map[string]ToolParam{
			"query": {Type: "string", Description: "Search query", Required: true},
		},
		Execute: a.searchNotes,
	}

	a.tools["list_notes"] = &Tool{
		Name:        "list_notes",
		Description: "List the user's recent notes.",
		Parameters: map[string]ToolParam{
			"tag": {Type: "string", Description: "Optional tag to filter by", Required: false},
		},
		Execute: a.listNotes,
	}

	// Email tools
	a.tools["send_email"] = &Tool{
		Name:        "send_email",
		Description: "Send an email or message to another user. For internal users, use their username. For external email, use their email address.",
		Parameters: map[string]ToolParam{
			"to":      {Type: "string", Description: "Recipient username or email address", Required: true},
			"subject": {Type: "string", Description: "Email subject", Required: true},
			"body":    {Type: "string", Description: "Email body content", Required: true},
		},
		Execute: a.sendEmail,
	}

	a.tools["check_inbox"] = &Tool{
		Name:        "check_inbox",
		Description: "Check the user's inbox for unread messages and recent mail.",
		Parameters:  map[string]ToolParam{},
		Execute:     a.checkInbox,
	}

	// Final answer tool
	a.tools["final_answer"] = &Tool{
		Name:        "final_answer",
		Description: "Provide the final answer to the user. Use this when you have completed the task or gathered all needed information.",
		Parameters: map[string]ToolParam{
			"answer": {Type: "string", Description: "The final answer or response to give the user", Required: true},
		},
		Execute: a.finalAnswer,
	}
}

// Run executes the agent on a task
func (a *Agent) Run(task string) *Result {
	start := time.Now()

	result := &Result{
		Steps: []*Step{},
	}

	// Build tool descriptions for the prompt
	toolsJSON := a.buildToolsPrompt()

	// Create the system prompt for the agent
	systemPrompt := fmt.Sprintf(`You are an AI agent for the Mu platform. You help users accomplish tasks by using the available tools.

Available tools:
%s

You must respond with valid JSON in this exact format:
{
  "thought": "Your reasoning about what to do next",
  "tool": "tool_name",
  "parameters": { "param1": "value1" }
}

Rules:
1. Think step by step about what you need to do
2. Use one tool at a time
3. After each tool result, decide if you need more steps or can provide final_answer
4. Use video_search to find videos, then video_play to play a specific result
5. Use news_search to find articles, then news_read to get full content
6. Always end with final_answer when done
7. Be concise and efficient - minimize steps

Current user: %s`, toolsJSON, a.userID)

	// Conversation context for multi-step reasoning
	var context ai.History
	currentPrompt := task

	for i := 0; i < a.maxSteps; i++ {
		// Ask LLM for next step
		llmPrompt := &ai.Prompt{
			System:   systemPrompt,
			Question: currentPrompt,
			Context:  context,
			Priority: ai.PriorityHigh,
		}

		response, err := ai.Ask(llmPrompt)
		if err != nil {
			result.Success = false
			result.Answer = fmt.Sprintf("Error: %v", err)
			break
		}

		// Parse the agent's response
		step, err := a.parseStep(response)
		if err != nil {
			app.Log("agent", "Failed to parse step: %v, response: %s", err, response)
			// Try to extract useful info anyway
			result.Answer = response
			result.Success = true
			break
		}

		result.Steps = append(result.Steps, step)

		// Execute the tool
		if tool, ok := a.tools[step.Tool]; ok {
			params, _ := step.Parameters.(map[string]interface{})
			toolResult, err := tool.Execute(params)
			if err != nil {
				step.Result = &ToolResult{Success: false, Error: err.Error()}
			} else {
				step.Result = toolResult
			}

			// Propagate action/URL from tool results (before checking final_answer)
			if step.Result != nil && step.Result.Action != "" {
				result.Action = step.Result.Action
				result.URL = step.Result.URL
				result.HTML = step.Result.HTML
			}

			// Check if this is the final answer
			if step.Tool == "final_answer" {
				result.Success = true
				if answer, ok := params["answer"].(string); ok {
					result.Answer = answer
				}
				break
			}

			// Add to context for next iteration
			resultJSON, _ := json.Marshal(step.Result)
			context = append(context, ai.Message{
				Prompt: currentPrompt,
				Answer: response,
			})
			currentPrompt = fmt.Sprintf("Tool result: %s\n\nWhat's next?", string(resultJSON))

		} else {
			step.Result = &ToolResult{Success: false, Error: fmt.Sprintf("Unknown tool: %s", step.Tool)}
			result.Answer = fmt.Sprintf("Unknown tool: %s", step.Tool)
			break
		}
	}

	result.Duration = time.Since(start).String()
	return result
}

// RunStreaming executes the agent and calls the callback after each step
func (a *Agent) RunStreaming(task string, onStep StepCallback) *Result {
	start := time.Now()

	result := &Result{
		Steps: []*Step{},
	}

	toolsJSON := a.buildToolsPrompt()

	systemPrompt := fmt.Sprintf(`You are an AI agent for the Mu platform. You help users accomplish tasks by using the available tools.

Available tools:
%s

You must respond with valid JSON in this exact format:
{
  "thought": "Your reasoning about what to do next",
  "tool": "tool_name",
  "parameters": { "param1": "value1" }
}

Rules:
1. Think step by step about what you need to do
2. Use one tool at a time
3. After each tool result, decide if you need more steps or can provide final_answer
4. Use video_search to find videos, then video_play to play a specific result
5. Use news_search to find articles, then news_read to get full content
6. Always end with final_answer when done
7. Be concise and efficient - minimize steps

Current user: %s`, toolsJSON, a.userID)

	var context ai.History
	currentPrompt := task

	for i := 0; i < a.maxSteps; i++ {
		llmPrompt := &ai.Prompt{
			System:   systemPrompt,
			Question: currentPrompt,
			Context:  context,
			Priority: ai.PriorityHigh,
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

		if tool, ok := a.tools[step.Tool]; ok {
			params, _ := step.Parameters.(map[string]interface{})
			toolResult, err := tool.Execute(params)
			if err != nil {
				step.Result = &ToolResult{Success: false, Error: err.Error()}
			} else {
				step.Result = toolResult
			}

			if step.Result != nil && step.Result.Action != "" {
				result.Action = step.Result.Action
				result.URL = step.Result.URL
				result.HTML = step.Result.HTML
			}

			// Stream the step to client
			isFinal := step.Tool == "final_answer"
			if onStep != nil {
				onStep(step, isFinal)
			}

			if isFinal {
				result.Success = true
				if answer, ok := params["answer"].(string); ok {
					result.Answer = answer
				}
				break
			}

			resultJSON, _ := json.Marshal(step.Result)
			context = append(context, ai.Message{
				Prompt: currentPrompt,
				Answer: response,
			})
			currentPrompt = fmt.Sprintf("Tool result: %s\n\nWhat's next?", string(resultJSON))

		} else {
			step.Result = &ToolResult{Success: false, Error: fmt.Sprintf("Unknown tool: %s", step.Tool)}
			result.Answer = fmt.Sprintf("Unknown tool: %s", step.Tool)
			if onStep != nil {
				onStep(step, true)
			}
			break
		}
	}

	result.Duration = time.Since(start).String()
	return result
}

// buildToolsPrompt creates the tools description for the LLM
func (a *Agent) buildToolsPrompt() string {
	var tools []map[string]interface{}
	for _, tool := range a.tools {
		t := map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Parameters,
		}
		tools = append(tools, t)
	}
	b, _ := json.MarshalIndent(tools, "", "  ")
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

	var step Step
	if err := json.Unmarshal([]byte(response), &step); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	return &step, nil
}

// finalAnswer is a special tool that ends the agent loop
func (a *Agent) finalAnswer(params map[string]interface{}) (*ToolResult, error) {
	answer, _ := params["answer"].(string)
	return &ToolResult{
		Success: true,
		Data:    answer,
	}, nil
}
