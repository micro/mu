package home

import (
	"html"
	"mu/internal/app"
	"mu/internal/origin"
	"net/http"
	"strings"
)

func DevelopersHandler(w http.ResponseWriter, r *http.Request) {
	base := html.EscapeString(strings.TrimRight(origin.URL(r), "/"))
	body := `<div class="page-col"><p>Build with Micro through HTTP or MCP. Use its assistant to carry out a goal, or call selected services from your own application.</p><section class="section-stack"><h2>Choose the access your application needs</h2><table class="data-table"><thead><tr><th>Access</th><th>What it gives you</th><th>Start here</th></tr></thead><tbody><tr><td>Assistant</td><td>Ask an agent, submit background work, and read or reply to Inbox conversations.</td><td><a href="/token?access=agent">Create an assistant token</a></td></tr><tr><td>Services</td><td>Call only the services you select. This does not grant assistant execution or Inbox API access.</td><td><a href="/token?access=services">Create a services token</a></td></tr></tbody></table><p>Choose the smallest set of permissions your application needs. Assistant access applies across your account; it does not create an isolated application account.</p></section><section class="section-stack"><h2>Make your first request</h2><p>Create an assistant token with Agent access and actions enabled. Keep it on your server, outside source control and browser code.</p><pre>curl -X POST '` + base + `/api/v1/agent/ask' \
  -H "Authorization: Bearer $MICRO_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What can you help me with?"}'</pre><p>The answer is saved as a conversation. Use its thread ID to continue. For longer jobs, submit Work and check its status.</p><p>For MCP, use <code>` + base + `/mcp</code> with the same credential. The token determines which tools are available. HTTP and MCP provide the same operations.</p><div class="form-actions"><a href="/api">Assistant, Work and Inbox reference</a><a href="/services">Service reference</a><a href="/api/v1">Operation schemas</a></div></section><section class="section-stack"><h2>Usage and credits</h2><p>Tokens grant permission; they do not contain a balance. Billable calls use the token owner’s account budget. The current daily allowance is used first, followed by prepaid credits. Paid tools can add to the cost of an assistant request. A credit is one US cent.</p><p>Read-only access to stored conversations and other operations priced at zero do not consume credit. Buying credit does not remove messaging and abuse limits.</p><div class="form-actions"><a href="/account/developer">Balance and usage</a><a href="/account/topup">Add credit</a><a href="/tools">Tool costs</a></div></section><section class="section-stack"><h2>Connecting a mail or chat app?</h2><p>You do not need an API token. <a href="/account/connections">Create an app password in Connections</a> and copy the server settings into your app.</p></section></div>`
	app.Respond(w, r, app.Response{Title: "Developers", Description: "Build with the Micro API and MCP", HTML: body})
}
