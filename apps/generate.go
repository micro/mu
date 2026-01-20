package apps

import (
	"fmt"
	"strings"

	"mu/ai"
)

// generateAppCode uses AI to generate HTML/CSS/JS from a prompt
func generateAppCode(prompt string) (string, error) {
	systemPrompt := `You are an expert web developer. Generate a complete, self-contained single-page web application based on the user's description.

Rules:
1. Output ONLY valid HTML - no markdown, no code fences, no explanations
2. Include all CSS in a <style> tag in the <head>
3. Include all JavaScript in a <script> tag before </body>
4. Use modern, clean design with good typography
5. Make it mobile-responsive
6. Use system-ui font stack
7. The app must be fully functional and self-contained
8. Do not use any external dependencies or CDNs
9. Start with <!DOCTYPE html> and end with </html>
10. NEVER use placeholder comments like "// ..." or "/* ... */" - write ALL the actual code
11. Output the COMPLETE application - no shortcuts, no omissions

Mu SDK (automatically available as window.mu):
- mu.db.get(key) - retrieve stored value from server (async, syncs across devices)
- mu.db.set(key, value) - store value persistently on server (async)
- mu.db.delete(key) - delete a key (async)
- mu.db.list() - list all keys (async)
- mu.cache.get(key) - get cached value from localStorage (async, returns null if expired)
- mu.cache.set(key, value, {ttl: seconds}) - cache value locally with optional TTL (async)
- mu.cache.delete(key) - delete cached item (async)
- mu.cache.clear() - clear all cached items for this app (async)
- mu.fetch(url) - fetch any URL (server-side proxy, bypasses CORS) - returns {ok, status, text(), json()}
- mu.user.id - current user's ID (null if not logged in)
- mu.user.name - current user's name
- mu.user.loggedIn - boolean
- mu.app.id - this app's ID
- mu.app.name - this app's name
- mu.theme.get(name) - get CSS variable value (e.g., mu.theme.get('accent-color'))

IMPORTANT: For fetching external URLs, ALWAYS use mu.fetch() instead of fetch() to avoid CORS issues.
IMPORTANT: Use mu.cache for API responses (fast, local). Use mu.db for user data (persistent, syncs across devices).

Theme CSS variables are automatically available (use var(--mu-*)):
--mu-text-primary, --mu-text-secondary, --mu-text-muted
--mu-accent-color, --mu-accent-blue
--mu-card-background, --mu-card-border, --mu-hover-background
--mu-spacing-xs/sm/md/lg/xl, --mu-border-radius
--mu-shadow-sm, --mu-shadow-md, --mu-transition-fast
--mu-font-family

Use mu.cache for API responses to avoid fetching on every page load. Use mu.db for user data that should sync across devices.

Generate the complete HTML file now:`

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
