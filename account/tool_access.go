package account

import (
	"encoding/json"
	"html"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/settings"
)

// toolAccess gives credit buyers a next step that remains visible after checkout.
func toolAccess(setup bool) string {
	host := strings.TrimSpace(settings.Get("X402_HOST"))
	if host == "" {
		return ""
	}
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	base := u.Scheme + "://" + u.Host
	label := html.EscapeString(u.Host)
	body := `<p>Your account and credits work with the tools at <a href="` + html.EscapeString(base+"/tools") + `">` + label + `</a>. Manage billing here; connect your agent to that host using a service token.</p>`
	if !setup {
		body += `<p>Create a service token, then add the endpoint and token to your agent. Authenticated tool calls use this account’s available credits.</p><div class="form-actions"><a class="btn" href="/account/tokens?access=services#tool-connection">Connect your agent</a><a href="` + html.EscapeString(base+"/tools") + `">Browse tools</a></div>`
	} else {
		body += `<ol><li>Create a token above with <strong>Services API / MCP</strong> access. Select the services your agent needs.</li><li>In your agent’s MCP settings, add the remote HTTP endpoint below and set the Authorization header to <code>Bearer YOUR_TOKEN</code>.</li><li>Ask your agent to list the available tools, then try one. Paid calls use your account credits; no wallet payment is needed while credits are available.</li></ol><p>MCP endpoint: <code>` + html.EscapeString(base+"/mcp") + `</code></p>`
		config, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"tools": map[string]any{"url": base + "/mcp", "headers": map[string]string{"Authorization": "Bearer YOUR_TOKEN"}}}}, "", "  ")
		body += `<details class="disclosure"><summary>MCP configuration example</summary><p>For clients that accept this configuration format. Replace YOUR_TOKEN locally with the token you just copied.</p><pre>` + html.EscapeString(string(config)) + `</pre></details><p><a href="` + html.EscapeString(base+"/api") + `">HTTP API reference</a> · <a href="` + html.EscapeString(base+"/tools") + `">Browse tools</a> · <a href="/account/usage">View usage</a></p>`
	}
	return app.SectionID("tool-connection", "Use your credits with "+label, body)
}
