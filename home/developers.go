package home

import (
	"html"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/origin"
	"net/http"
	"strings"
)

func DevelopersHandler(w http.ResponseWriter, r *http.Request) {
	base := html.EscapeString(strings.TrimRight(origin.URL(r), "/"))
	var b strings.Builder
	b.WriteString(`<div class="page-col"><p>Use your Micro assistant from your own code. Same account, conversations and usage allowance.</p><p>Product operations live under /agent/api, /inbox/api and /work/api. Service operations remain at <a href="/api">/api/v1</a> and <a href="/tools">/mcp</a>, using Services tokens.</p><h2>Ask your assistant</h2><p><a href="/account/tokens?add=api#create-token-form">Create an API token</a>, leaving Agents and Allow actions selected. Set it as <code>MICRO_TOKEN</code> in your environment, then run:</p><pre>curl '` + base + `/agent/api/ask' \
  -H "Authorization: Bearer $MICRO_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Help me plan my week"}'</pre><p>The response contains <code>data.text</code> (the reply) and <code>data.thread</code> (the saved conversation ID). To continue, send another request to the same endpoint with:</p><pre>{"prompt":"Focus on Monday", "thread":"THREAD_ID"}</pre><p>Replace <code>THREAD_ID</code> with the returned ID. Omit it to start a new conversation. Keep your token on your server, outside browser code and source control.</p><p>Requests use your account’s included allowance, then prepaid credit. <a href="/account/usage">Usage</a> · <a href="/pricing#costs">Usage costs</a>.</p>`)
	b.WriteString(`<section id="mcp" class="section-stack"><h2>MCP</h2><p>Add a remote server using HTTP with this URL:</p><pre>` + base + `/agent/mcp</pre><p>Set its authorization header to <code>Authorization: Bearer YOUR_TOKEN</code>, using the same API token. The client can discover tools and call <code>agent_ask</code> with a <code>prompt</code>. <a href="#reference">Tool reference</a>.</p></section><section id="reference" class="section-stack"><h2>API and tool reference</h2><p>All calls use POST with the same authorization and JSON headers as the example above. Enable the corresponding capability on your token. Actions also require Allow actions.</p><p>Successful responses wrap the result in <code>{"data": ...}</code>. Errors return <code>{"error":{"code": ..., "message": ...}}</code>. A 401 means the credential is missing or invalid; 403 means access is not allowed; 402 means there is not enough credit.</p><p>For background work, submit a job once, then poll its ID with Work get. Do not automatically resubmit after a lost response: it could run twice.</p>`)
	for _, op := range api.Operations {
		b.WriteString(`<h3>` + html.EscapeString(op.Name) + `</h3><p>` + html.EscapeString(op.Description) + `</p><code>POST ` + html.EscapeString(api.ProductPath(op.Name)) + `</code>`)
		for _, param := range op.Params {
			b.WriteString(`<p><code>` + html.EscapeString(param.Name) + `</code> — ` + html.EscapeString(param.Description) + `</p>`)
		}
	}
	b.WriteString(`</section></div>`)
	app.Respond(w, r, app.Response{Title: "Developers", Description: "Call your Micro assistant from code", HTML: b.String()})
}
