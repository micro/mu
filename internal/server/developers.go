package server

import (
	"html"
	"mu/internal/app"
	"mu/internal/origin"
	"net/http"
	"strings"
)

func DevelopersHandler(w http.ResponseWriter, r *http.Request) {
	base := html.EscapeString(strings.TrimRight(origin.URL(r), "/"))
	body := `<div class="page-col"><p>Use the same resources from your browser or your own code. Send JSON with Content-Type: application/json and request JSON with Accept: application/json.</p>
 <h2>Assistant</h2><p><a href="/account/tokens?add=api#create-token-form">Create a token</a> with Agents and Allow actions. Set it as MICRO_TOKEN, then run:</p><pre>curl '` + base + `/agent' \
  -H "Authorization: Bearer $MICRO_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"prompt":"Help me plan my week"}'</pre>
 <p>The response contains <code>text</code> and <code>thread</code>. Send <code>{"prompt":"Focus on Monday","thread":"THREAD_ID"}</code> to continue. Use <code>/agent/name</code> for a named agent. Keep your token outside browser code and source control.</p>
 <h2>Inbox and Work</h2><p>Use the same authorization header and enable the corresponding capability on your token. Writes also require Allow actions.</p>
 <table class="data-table stacked"><thead><tr><th>Request</th><th>Purpose</th></tr></thead><tbody>
 <tr><td>GET /inbox</td><td>Read your inbox as JSON.</td></tr>
 <tr><td>GET /inbox?id=THREAD_ID</td><td>Read a thread.</td></tr>
 <tr><td>POST /inbox</td><td>Send {"action":"mark_read","id":"THREAD_ID"} or mark_unread.</td></tr>
 <tr><td>GET /work</td><td>List delegated work; filter with ?status=failed.</td></tr>
 <tr><td>GET /work?id=WORK_ID</td><td>Read progress in the work field.</td></tr>
 <tr><td>POST /work</td><td>Send {"prompt":"…"} to submit work, or {"action":"retry","id":"WORK_ID"} to retry reviewed work.</td></tr>
 </tbody></table><p>Do not automatically retry a submission after a lost response: it may already have started. Usage draws from the same account allowance and balance.</p>
 <h2>Service tools</h2><p><a href="/tools">Tools</a> documents the runtime services available through <code>/mcp</code> and <a href="/api">/api/v1</a>. Use a Services token for these. MCP supplies tools to your client; it does not run your Micro assistant.</p></div>`
	app.Respond(w, r, app.Response{Title: "Developers", HTML: body})
}
