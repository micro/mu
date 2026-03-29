package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/apps"
	"mu/internal/api"
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

	// Skip the planner — go straight to apps_build
	text, isErr, execErr := api.ExecuteToolAs(flow.AccountID, "apps_build", map[string]any{
		"prompt": prompt,
	})
	if execErr != nil || isErr {
		errMsg := text
		if execErr != nil {
			errMsg = execErr.Error()
		}
		if len(errMsg) > 200 {
			errMsg = errMsg[:200]
		}
		sseSend(map[string]any{"type": "error", "message": "Build failed: " + errMsg})
		sseSend(map[string]any{"type": "done"})
		return
	}

	// Parse the result to get the app slug
	var appResult struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
		Run  string `json:"run"`
	}
	json.Unmarshal([]byte(text), &appResult)

	if appResult.Slug == "" {
		sseSend(map[string]any{"type": "error", "message": "Build returned no app"})
		sseSend(map[string]any{"type": "done"})
		return
	}

	// Read the app HTML and send it to the preview
	a := apps.GetApp(appResult.Slug)
	if a != nil && a.HTML != "" {
		sseSend(map[string]any{"type": "exec", "html": a.HTML})
		// Wait briefly for exec result
		waitForExecResult(flow.ID, 5*time.Second)
	}

	// Send response with link
	rendered := app.RenderString(fmt.Sprintf("Built **%s** — [Open App](/apps/%s/run) · [Edit](/apps/%s/edit)", appResult.Name, appResult.Slug, appResult.Slug))
	sseSend(map[string]any{"type": "response", "html": rendered, "flow_id": flow.ID})
	updateFlow(flow.ID, func(f *Flow) {
		f.Status = "done"
		f.Answer = fmt.Sprintf("Built %s — /apps/%s/run", appResult.Name, appResult.Slug)
		f.HTML = rendered
	})

	sseSend(map[string]any{"type": "done"})
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
