package home

import (
	"html"
	"mu/account"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/origin"
	"net/http"
	"strings"
)

func DevelopersHandler(w http.ResponseWriter, r *http.Request) {
	base := html.EscapeString(strings.TrimRight(origin.URL(r), "/"))
	var b strings.Builder
	b.WriteString(`<div class="page-col"><p>Use your Micro assistant from your own code. Same account, conversations and usage allowance.</p><h2>Ask your assistant</h2><p><a href="/token?access=agent">Create an API token</a>, leaving Agents and Allow actions selected. Set it as <code>MICRO_TOKEN</code> in your environment, then run:</p><pre>curl '` + base + `/api/v1/agent/ask' \
  -H "Authorization: Bearer $MICRO_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Help me plan my week"}'</pre><p>The response contains <code>data.text</code> (the reply) and <code>data.thread</code> (the saved conversation ID). To continue, send another request to the same endpoint with:</p><pre>{"prompt":"Focus on Monday", "thread":"THREAD_ID"}</pre><p>Replace <code>THREAD_ID</code> with the returned ID. Omit it to start a new conversation. Keep your token on your server, outside browser code and source control.</p><p>Requests use your account’s included allowance, then prepaid credit. There is no separate API balance. <a href="/account/billing">View balance and usage</a>.</p>`)
	if !account.PaymentsEnabled() {
		b.WriteString(`<p>This instance does not charge for usage.</p>`)
	}
	b.WriteString(`<details id="mcp" class="disclosure"` + mcpOpen(r) + `><summary>Connect an MCP client</summary><p>Add a remote server using HTTP with this URL:</p><pre>` + base + `/mcp</pre><p>Set its authorization header to <code>Authorization: Bearer YOUR_TOKEN</code>, using the same API token. The client can discover tools and call <code>agent_ask</code> with a <code>prompt</code>. Opening the URL in a browser brings you to these instructions.</p></details><details class="disclosure"><summary>More API operations</summary><p>All calls use POST with the same authorization and JSON headers as the example above. Enable the corresponding capability on your token. Actions also require Allow actions.</p><p>Successful responses wrap the result in <code>{"data": ...}</code>. Errors return <code>{"error":{"code": ..., "message": ...}}</code>. A 401 means the credential is missing or invalid; 403 means access is not allowed; 402 means there is not enough credit.</p><p>For background work, submit a job once, then poll its ID with Work get. Do not automatically resubmit after a lost response: it could run twice.</p>`)
	for _, op := range api.Operations {
		b.WriteString(`<h3>` + html.EscapeString(strings.Replace(op.Name, "_", " / ", 1)) + `</h3><p>` + html.EscapeString(op.Description) + `</p><code>POST /api/v1/` + html.EscapeString(strings.Replace(op.Name, "_", "/", 1)) + `</code>`)
		for _, param := range op.Params {
			b.WriteString(`<p><code>` + html.EscapeString(param.Name) + `</code> — ` + html.EscapeString(param.Description) + `</p>`)
		}
	}
	b.WriteString(`</details><details class="disclosure"><summary>Usage costs</summary><p>One credit is one US cent. An assistant call and any paid tools it uses draw from the same account budget. Stored-data operations priced at zero are free. Messaging and rate limits still apply.</p>` + account.PricingTableHTML() + `</details><details class="disclosure"><summary>Calling individual services</summary><p>If you need specific services without assistant execution, create a token with Selected services access. Choose only the services your application needs. These tokens use the same HTTP and MCP endpoints and account balance; they do not grant assistant or Inbox API access.</p><p><a href="/services">Browse individual service methods</a></p></details></div>`)
	app.Respond(w, r, app.Response{Title: "Developers", Description: "Call your Micro assistant from code", HTML: b.String()})
}

// Show setup immediately when a browser follows the MCP endpoint.
func mcpOpen(r *http.Request) string {
	if r.URL.Query().Get("setup") == "mcp" {
		return " open"
	}
	return ""
}
