package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/apps"
	"mu/internal/app"
)

// looksLikeExecRequest checks if the prompt is asking to build, create, or make something.
func looksLikeExecRequest(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, keyword := range []string{
		"build me", "build a", "build an", "create a", "create an",
		"make me", "make a", "make an", "write a", "write an",
		"build app", "create app", "make app",
		"calculator", "timer", "converter", "generator", "tracker",
		"editor", "viewer", "dashboard", "tool",
	} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

// ExecResult is the result of executing code in the browser.
type ExecResult struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	DOM       string `json:"dom,omitempty"`
}

var (
	execMu    sync.Mutex
	execStore = map[string]chan *ExecResult{}
)

// waitForExecResult waits for the browser to send back the result of an exec.
func waitForExecResult(sessionID string, timeout time.Duration) *ExecResult {
	ch := make(chan *ExecResult, 1)

	execMu.Lock()
	execStore[sessionID] = ch
	execMu.Unlock()

	defer func() {
		execMu.Lock()
		delete(execStore, sessionID)
		execMu.Unlock()
	}()

	select {
	case r := <-ch:
		return r
	case <-time.After(timeout):
		return nil
	}
}

// ExecResultHandler receives POST /agent/exec from the browser.
func ExecResultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var result ExecResult
	if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
		app.RespondJSON(w, map[string]string{"error": "invalid request"})
		return
	}

	execMu.Lock()
	ch, ok := execStore[result.SessionID]
	execMu.Unlock()

	if ok {
		select {
		case ch <- &result:
		default:
		}
	}

	app.RespondJSON(w, map[string]string{"status": "ok"})
}

const maxIterations = 3

// runWorkspaceFlow handles build/exec requests within the agent SSE stream.
func runWorkspaceFlow(w http.ResponseWriter, r *http.Request, flow *Flow, prompt string, model Model) {
	sseSend := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	sseSend(map[string]any{"type": "thinking", "message": "Building…"})

	// Build the app directly
	a, err := apps.BuildAndSave(prompt, flow.AccountID, "Agent")
	if err != nil {
		errMsg := err.Error()
		if len(errMsg) > 200 {
			errMsg = errMsg[:200]
		}
		sseSend(map[string]any{"type": "error", "message": "Build failed: " + errMsg})
		sseSend(map[string]any{"type": "done"})
		return
	}

	slug := a.Slug
	name := a.Name

	// Iterate: render → check errors → fix → re-render
	for attempt := 0; attempt <= maxIterations; attempt++ {
		current := apps.GetApp(slug)
		if current == nil || current.HTML == "" {
			sseSend(map[string]any{"type": "error", "message": "App has no HTML"})
			sseSend(map[string]any{"type": "done"})
			return
		}

		// Inject the native SDK into the HTML before sending to preview
		html := current.HTML
		sdk := apps.NativeSDK(slug)
		html = apps.InjectSDK(html, sdk)

		sseSend(map[string]any{"type": "exec", "html": html})

		// Wait for browser to render and report back
		result := waitForExecResult(flow.ID, 8*time.Second)

		if result == nil {
			// No response from browser — can't iterate, just finish
			break
		}

		if result.OK && result.Error == "" {
			// Success — no errors
			break
		}

		// There are errors — can we iterate?
		if attempt >= maxIterations {
			sseSend(map[string]any{"type": "thinking", "message": fmt.Sprintf("App has errors after %d attempts, saving as-is", maxIterations)})
			break
		}

		// Tell the user we're fixing
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		sseSend(map[string]any{"type": "thinking", "message": fmt.Sprintf("Fixing: %s", truncateExec(errMsg, 100))})

		// Edit the app to fix the error
		fixPrompt := fmt.Sprintf("The app has a runtime error: %s\n\nFix this error. The app must work without errors.", errMsg)
		if result.DOM != "" {
			fixPrompt += fmt.Sprintf("\n\nCurrent DOM state: %s", truncateExec(result.DOM, 500))
		}

		_, editErr := apps.EditApp(slug, fixPrompt, flow.AccountID)
		if editErr != nil {
			sseSend(map[string]any{"type": "thinking", "message": "Could not auto-fix, saving as-is"})
			break
		}
		// Loop back to re-render the fixed version
	}

	// Send response with link
	rendered := app.RenderString(fmt.Sprintf("Built **%s** — [Open App](/apps/%s) · [Edit](/apps/%s/edit)", name, slug, slug))
	sseSend(map[string]any{"type": "response", "html": rendered, "flow_id": flow.ID})
	updateFlow(flow.ID, func(f *Flow) {
		f.Status = "done"
		f.Answer = fmt.Sprintf("Built %s — /apps/%s", name, slug)
		f.HTML = rendered
	})

	sseSend(map[string]any{"type": "done"})
}

// truncateExec shortens a string to max length.
func truncateExec(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// stripCodeFences removes markdown code fences from AI output.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"```javascript\n", "```js\n", "```html\n", "```\n"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	if strings.HasSuffix(s, "\n```") {
		s = s[:len(s)-4]
	} else if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
