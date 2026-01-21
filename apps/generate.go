package apps

import (
	"fmt"
	"strings"

	"mu/ai"
)

// Base app template - LLM modifies this instead of generating from scratch
const appTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{TITLE}}</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: var(--mu-font-family, system-ui, sans-serif); padding: 20px; max-width: 500px; margin: 0 auto; color: var(--mu-text-primary, #222); }
    h1 { font-size: 1.5em; margin-bottom: 16px; }
    .card { background: var(--mu-card-background, #fff); border: 1px solid var(--mu-card-border, #e0e0e0); border-radius: 8px; padding: 16px; margin-bottom: 12px; }
    .form { display: flex; gap: 8px; margin-bottom: 16px; }
    .form input, .form select { flex: 1; padding: 10px; border: 1px solid #ddd; border-radius: 6px; font-size: 16px; }
    .form button, button.primary { padding: 10px 16px; background: var(--mu-accent-color, #222); color: #fff; border: none; border-radius: 6px; cursor: pointer; font-size: 16px; }
    .list { display: flex; flex-direction: column; gap: 8px; }
    .item { display: flex; justify-content: space-between; align-items: center; }
    .item button { background: none; border: none; color: #999; cursor: pointer; font-size: 18px; }
    .empty { color: var(--mu-text-muted, #888); text-align: center; padding: 20px; }
    .meta { font-size: 0.85em; color: var(--mu-text-muted, #888); }
    .count { font-size: 3em; text-align: center; margin: 20px 0; font-weight: bold; }
    .stats { display: flex; gap: 16px; justify-content: center; margin-bottom: 16px; }
    .stat { text-align: center; }
    .stat-value { font-size: 1.5em; font-weight: bold; }
    .stat-label { font-size: 0.85em; color: var(--mu-text-muted, #888); }
{{EXTRA_CSS}}
  </style>
</head>
<body>
  <h1>{{TITLE}}</h1>
{{BODY_HTML}}
  <script>
    const DATA_KEY = '{{DATA_KEY}}';
    
    function timeAgo(ts) {
      const diff = Date.now() - ts;
      if (diff < 60000) return 'just now';
      if (diff < 3600000) return Math.floor(diff/60000) + 'm ago';
      if (diff < 86400000) return Math.floor(diff/3600000) + 'h ago';
      return Math.floor(diff/86400000) + 'd ago';
    }
    
    function today() { return new Date().toISOString().split('T')[0]; }
    
{{APP_JS}}
    
    load();
  </script>
</body>
</html>`

// generateAppCode uses AI to generate HTML/CSS/JS from a prompt
func generateAppCode(prompt string) (string, error) {
	systemPrompt := `You are an expert web developer. Modify the template below to create the requested app.

Rules:
1. Output ONLY the filled template - no markdown, no code fences, no explanations
2. Replace {{TITLE}} with the app title
3. Replace {{DATA_KEY}} with a unique storage key for this app
4. Replace {{BODY_HTML}} with the app's HTML body (forms, displays, etc)
5. Replace {{APP_JS}} with the app's JavaScript (load, render, action functions)
6. Replace {{EXTRA_CSS}} with any additional CSS needed (or leave empty)
7. Use mu.db.get/set for persistent storage, mu.fetch for external APIs
8. Keep the existing CSS classes and structure where possible
9. NEVER use placeholder comments - write ALL actual code
10. Start with <!DOCTYPE html> exactly as shown

Available: mu.db.get(key), mu.db.set(key,val), mu.fetch(url), mu.user.name, mu.user.id, today(), timeAgo(ts)

TEMPLATE:
` + appTemplate + `

Generate the complete modified HTML file now:`

	llmPrompt := &ai.Prompt{
		System:   systemPrompt,
		Question: prompt,
		Priority: ai.PriorityHigh,
	}

	response, err := ai.Ask(llmPrompt)
	if err != nil {
		return "", err
	}

	// Clean up response - extract just the HTML portion
	response = cleanLLMResponse(response)

	// Validate the response is complete and valid
	if err := validateModifiedCode(response); err != nil {
		return "", fmt.Errorf("LLM returned invalid code: %v", err)
	}

	return response, nil
}

// generateAppSummary generates a short user-facing description from the prompt
func generateAppSummary(prompt string) (string, error) {
	systemPrompt := `Generate a brief, user-friendly description of this app in ONE sentence (max 80 characters). 
Output ONLY the description text, nothing else. No quotes, no prefix, no explanation.
Example: "Track daily tasks with categories and due dates"`

	llmPrompt := &ai.Prompt{
		System:   systemPrompt,
		Question: prompt,
		Priority: ai.PriorityLow,
	}

	response, err := ai.Ask(llmPrompt)
	if err != nil {
		return "", err
	}

	// Clean up - remove quotes and trim
	response = strings.TrimSpace(response)
	response = strings.Trim(response, `"'`)

	// Enforce max length
	if len(response) > 100 {
		response = response[:97] + "..."
	}

	return response, nil
}

// modifyAppCode uses AI to make targeted changes to existing code
func modifyAppCode(currentCode, instruction string) (string, error) {
	systemPrompt := `You are an expert web developer. You will receive existing HTML/CSS/JS code and an instruction for how to modify it.

Rules:
1. Output ONLY the complete modified HTML file - no markdown, no code fences, no explanations
2. Make targeted changes based on the instruction - don't rewrite everything unnecessarily
3. Preserve the existing structure and style unless asked to change it
4. Keep all existing functionality unless asked to change it
5. Start with <!DOCTYPE html> and end with </html>
6. NEVER use placeholder comments like "// ...existing code..." - always include the full actual code
7. Output must be complete, valid, runnable HTML

Mu SDK (automatically available as window.mu):
- mu.db.get(key) - retrieve stored value from server (async, syncs across devices)
- mu.db.set(key, value) - store value persistently on server (async)
- mu.db.delete(key) - delete a key (async) 
- mu.db.list() - list all keys (async)
- mu.cache.get(key) - get cached value from localStorage (async, returns null if expired)
- mu.cache.set(key, value, {ttl: seconds}) - cache value locally with optional TTL (async)
- mu.cache.delete(key) - delete cached item (async)
- mu.fetch(url) - fetch any URL (server-side proxy, bypasses CORS) - returns {ok, status, text(), json()}
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name
- mu.theme.get(name) - get CSS variable value

IMPORTANT: For fetching external URLs, ALWAYS use mu.fetch() instead of fetch() to avoid CORS issues.
IMPORTANT: Use mu.cache for API responses (fast, local). Use mu.db for user data (persistent, syncs across devices).

Current code:
` + currentCode + `

Apply this modification and output the complete updated HTML file:`

	llmPrompt := &ai.Prompt{
		System:   systemPrompt,
		Question: instruction,
		Priority: ai.PriorityHigh,
	}

	response, err := ai.Ask(llmPrompt)
	if err != nil {
		return "", err
	}

	// Clean up response - extract just the HTML portion
	response = cleanLLMResponse(response)

	// Validate the response is complete and valid
	if err := validateModifiedCode(response); err != nil {
		return "", fmt.Errorf("LLM returned invalid code: %v", err)
	}

	return response, nil
}

// cleanLLMResponse extracts clean HTML from LLM response, removing any markdown
// code fences, explanatory comments, or other text before/after the HTML
func cleanLLMResponse(response string) string {
	response = strings.TrimSpace(response)

	// Remove markdown code fences if present
	response = strings.TrimPrefix(response, "```html")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	lower := strings.ToLower(response)

	// Find where HTML starts (<!DOCTYPE or <html)
	start := strings.Index(lower, "<!doctype")
	if start == -1 {
		start = strings.Index(lower, "<html")
	}
	if start > 0 {
		response = response[start:]
		lower = strings.ToLower(response)
	}

	// Find where HTML ends (</html>) and truncate anything after
	end := strings.LastIndex(lower, "</html>")
	if end > 0 {
		response = response[:end+7] // +7 for len("</html>")
	}

	return strings.TrimSpace(response)
}

// validateModifiedCode checks if the LLM response is valid HTML
func validateModifiedCode(code string) error {
	lower := strings.ToLower(code)

	// Must have basic HTML structure
	if !strings.Contains(lower, "<html") {
		return fmt.Errorf("missing <html> tag")
	}
	if !strings.Contains(lower, "</html>") {
		return fmt.Errorf("missing </html> tag - response may be truncated")
	}
	if !strings.Contains(lower, "<body") {
		return fmt.Errorf("missing <body> tag")
	}
	if !strings.Contains(lower, "</body>") {
		return fmt.Errorf("missing </body> tag - response may be truncated")
	}

	// Check for obvious truncation markers
	if strings.Contains(lower, "...existing") || strings.Contains(lower, "// ...") || strings.Contains(lower, "/* ...") {
		return fmt.Errorf("response contains placeholder comments instead of actual code")
	}

	// Minimum reasonable size (avoid empty or stub responses)
	if len(code) < 100 {
		return fmt.Errorf("response too short - likely incomplete")
	}

	return nil
}
