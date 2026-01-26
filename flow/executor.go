package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mu/app"
	"mu/tools"
)

// ExecutionResult holds the result of running a flow
type ExecutionResult struct {
	Success   bool          `json:"success"`
	Steps     []*StepResult `json:"steps"`
	FinalData interface{}   `json:"final_data,omitempty"`
	Error     string        `json:"error,omitempty"`
	Duration  string        `json:"duration"`
}

// StepResult holds the result of a single step
type StepResult struct {
	Tool    string      `json:"tool"`
	Args    interface{} `json:"args,omitempty"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Execute runs a flow and returns the result
func Execute(f *Flow, userID string) *ExecutionResult {
	start := time.Now()
	result := &ExecutionResult{
		Steps: []*StepResult{},
	}

	// Parse the flow
	parsed, err := Parse(f.Source)
	if err != nil {
		result.Error = fmt.Sprintf("parse error: %v", err)
		result.Duration = time.Since(start).String()
		return result
	}

	// Create context with user
	ctx := tools.WithUser(context.Background(), userID)

	// Track data flowing between steps
	var lastData interface{}

	// Variables store
	vars := make(map[string]interface{})

	// Execute each step
	for _, step := range parsed.Steps {
		stepResult := &StepResult{
			Tool: step.Tool,
			Args: step.Args,
		}

		// Special handling for "summarize" - it needs the previous result
		if step.Tool == "summarize" {
			if lastData != nil {
				// For now, just pass through - real summarize would call AI
				stepResult.Success = true
				stepResult.Data = lastData
				app.Log("flow", "Summarize step (pass-through for now)")
			} else {
				stepResult.Error = "nothing to summarize"
			}
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		// Special handling for "var.save" - store last result in variable
		if step.Tool == "var.save" {
			if lastData != nil && step.SaveAs != "" {
				vars[step.SaveAs] = lastData
				stepResult.Success = true
				stepResult.Data = lastData
				app.Log("flow", "Saved to variable: %s", step.SaveAs)
			} else {
				stepResult.Error = "nothing to save"
			}
			result.Steps = append(result.Steps, stepResult)
			continue
		}

		// Convert args to map[string]any
		params := make(map[string]any)
		for k, v := range step.Args {
			// Special handling for "to: me" - resolve to user's email
			if k == "to" && v == "me" {
				params[k] = userID + "@mu.xyz" // TODO: get user's actual email
			} else {
				params[k] = v
			}
		}

		// If this is mail.send and we have previous data, include it as body
		if step.Tool == "mail.send" && lastData != nil {
			body, _ := json.MarshalIndent(lastData, "", "  ")
			params["body"] = string(body)
		}

		// Call the tool
		data, err := tools.Call(ctx, step.Tool, params)
		if err != nil {
			stepResult.Error = err.Error()
			result.Steps = append(result.Steps, stepResult)
			result.Error = fmt.Sprintf("step '%s' failed: %v", step.Tool, err)
			result.Duration = time.Since(start).String()
			updateFlowRun(f, result.Error, result.Duration)
			return result
		}

		stepResult.Success = true
		stepResult.Data = data
		lastData = data

		// Save to variable if specified
		if step.SaveAs != "" {
			vars[step.SaveAs] = data
			app.Log("flow", "Saved result to variable: %s", step.SaveAs)
		}

		result.Steps = append(result.Steps, stepResult)

		app.Log("flow", "Executed step %s successfully", step.Tool)
	}

	result.Success = true
	result.FinalData = lastData
	result.Duration = time.Since(start).String()

	updateFlowRun(f, "", result.Duration)
	return result
}

func updateFlowRun(f *Flow, errMsg string, duration string) {
	now := time.Now()
	f.LastRun = now
	f.LastError = errMsg
	f.RunCount++

	// Add to history (keep last 10 runs)
	log := RunLog{
		Time:     now,
		Success:  errMsg == "",
		Duration: duration,
		Error:    errMsg,
	}
	f.History = append(f.History, log)
	if len(f.History) > 10 {
		f.History = f.History[len(f.History)-10:]
	}

	f.Save()
}

// ExecuteSource parses and executes flow source directly (without saving)
func ExecuteSource(source, userID string) *ExecutionResult {
	tempFlow := &Flow{
		ID:     "temp",
		Source: source,
	}
	return Execute(tempFlow, userID)
}
