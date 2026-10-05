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
	body := `<div class="document-content"><p>Ask the agent, assign work or call services from your own applications.</p>
 <nav class="form-actions" aria-label="Developer resources"><a href="/tools">Tools &amp; MCP</a><a href="/api">Services API</a><a href="/x402">x402</a><a href="/account/tokens">Access tokens</a></nav>
 <h2>CLI</h2><p>Install Mu and connect to your account:</p><pre>curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu login ` + base + `
mu ask "What needs my attention?"</pre>
 <p>No model key is needed to use the hosted assistant. To continue a conversation, use <code>mu ask --thread THREAD_ID "…"</code>. Add <code>--raw</code> for JSON.</p>
 <pre>mu work submit --prompt "Research the options and recommend one"
mu work get --id WORK_ID
mu inbox list
mu tools
mu help</pre>
 <p>Work runs in the background; use its returned ID to read progress and the result. Use <code>mu help SERVICE METHOD</code> to see how to call a service.</p>
 <h2>HTTP</h2><p><a href="/account/tokens">Create a token</a> and set it as <code>MU_TOKEN</code>. Choose Agents / Account for the agent, work and inbox; enable the permissions you need, including Allow actions to submit work.</p>
 <pre>curl '` + base + `/agent' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'</pre>
 <p>The response includes <code>text</code> and <code>thread</code>. Send <code>thread</code> with your next prompt to continue.</p>
 <pre>curl '` + base + `/work' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Research the options and recommend one"}'

curl '` + base + `/work/WORK_ID' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Accept: application/json'</pre>
 <p>Submission returns an <code>id</code>. Reading it returns <code>work.status</code> and <code>work.result</code>. Agent replies and work use your account's credits.</p>
 <table class="data-table stacked"><thead><tr><th>Resource</th><th>Use</th></tr></thead><tbody>
 <tr><td><code>POST /agent</code></td><td>Ask Micro with a prompt and optional thread.</td></tr>
 <tr><td><code>POST /agent/NAME</code></td><td>Ask a specific agent.</td></tr>
 <tr><td><code>GET /agents</code></td><td>List your agents.</td></tr>
 <tr><td><code>POST /work</code></td><td>Assign a prompt, with an optional agent and thread.</td></tr>
 <tr><td><code>GET /work</code></td><td>List your work.</td></tr>
 <tr><td><code>GET /work/WORK_ID</code></td><td>Read progress and results.</td></tr>
 <tr><td><code>GET /inbox/THREAD_ID</code></td><td>Read a conversation.</td></tr>
 </tbody></table>
 <h2>Services and tools</h2><p>Call services directly with a Services token. The <a href="/api">API reference</a> lists HTTP endpoints, parameters and examples. The <a href="/tools">Tools page</a> lists MCP tools and connection instructions for your own agent. In the CLI, use <code>mu tools</code> to discover them and <code>mu SERVICE METHOD --argument value</code> to call one.</p>
 <h2>Pay per call</h2><p><a href="/x402">x402</a> lets your applications pay for public service calls with USDC. See the payment and connection details there.</p>
 <h2>Self-hosting</h2><p>The same CLI works with your own server: <code>mu login https://your-server.example</code>. See the <a href="/install">installation guide</a>, <a href="/help">documentation</a> and <a href="https://github.com/micro/mu">source code</a>.</p></div>`
	app.Respond(w, r, app.Response{Title: "Developers", HTML: body})
}
