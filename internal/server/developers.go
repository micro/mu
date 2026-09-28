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
	body := `<div class="document-content"><p>Ask an agent, assign work and retrieve the result through HTTP or the Mu CLI.</p>
 <p>Micro runs the agents and tools for you. Use your account and credits; no model key or server setup is needed. If your own agent only needs tools, start with <a href="/tools">MCP tools</a> or the <a href="/api">services HTTP API</a>.</p>
 <h2>Start with Micro</h2><p><a href="/account/tokens?add=api#create-token-form">Create a token</a>: choose Agents / Account, enable Agents and Background jobs, and Allow actions. Enable Inbox too if you want to read conversations. Keep the token in your environment or CLI configuration, outside browser code and source control.</p>
 <p>Build the current CLI from <a href="https://github.com/micro/mu">Mu</a>, then sign in:</p><pre>git clone https://github.com/micro/mu.git
cd mu
go build -o mu .
./mu login ` + base + `
./mu ask "Compare SQLite and PostgreSQL for a small personal server"</pre>
 <p>Micro returns an answer and saves the conversation. Use <code>./mu ask --raw "…"</code> for JSON containing <code>text</code> and <code>thread</code>; continue with <code>./mu ask --thread THREAD_ID "…"</code>.</p>
 <h2>Assign work</h2><p>For a task that should keep running after your request returns:</p><pre>./mu work submit --prompt "Compare SQLite and PostgreSQL for a small personal server. Cite sources and recommend one."
./mu work get --id WORK_ID</pre>
 <p>Submission returns an <code>id</code>. Reading it returns a <code>work</code> object with its <code>status</code>, <code>result</code>, steps and attempts. Check periodically: todo is queued, doing is running, done is complete; failed, blocked or canceled need review. The same work appears in <a href="/work">Work</a>.</p>
 <h2>Create an agent</h2><p>Give an agent reusable instructions and an explicit set of services it may use:</p><pre>./mu agent create researcher \
  --prompt "Research questions using sources. Cite URLs and distinguish facts from uncertainty." \
  --tools web,news
./mu agent list
./mu work submit --agent researcher --prompt "Compare SQLite and PostgreSQL for a small personal server"</pre>
 <p>Creation returns its <code>agent</code> name. Use that returned name when assigning work or with <code>./mu ask --agent NAME</code>. The tools flag takes service names from the <a href="/services">services directory</a>. Creating an agent does not issue another token; your plan's agent limit applies.</p>
 <h2>Use HTTP</h2><p>The CLI uses these same resources. Set your token as <code>MU_TOKEN</code>, then ask Micro:</p><pre>curl '` + base + `/agent' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"prompt":"Compare SQLite and PostgreSQL for a small personal server"}'</pre>
 <p>To create an agent:</p><pre>curl '` + base + `/agents' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"name":"researcher","prompt":"Research using sources and cite URLs.","services":["web","news"]}'</pre>
 <p>To assign work, use the returned agent name:</p><pre>curl '` + base + `/work' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -d '{"agent":"researcher","prompt":"Compare SQLite and PostgreSQL for a small personal server"}'

curl '` + base + `/work?id=WORK_ID' \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Accept: application/json'</pre>
 <table class="data-table stacked"><thead><tr><th>Request</th><th>Purpose</th></tr></thead><tbody>
 <tr><td>GET /agents</td><td>List your agents.</td></tr>
 <tr><td>POST /agents</td><td>Create with name, prompt and a nonempty services array.</td></tr>
 <tr><td>POST /agent or /agent/NAME</td><td>Ask with prompt; pass thread to continue a conversation.</td></tr>
 <tr><td>POST /work</td><td>Submit prompt, optional agent and optional thread for context and delivery.</td></tr>
 <tr><td>GET /work</td><td>List work; optionally filter with ?status=failed.</td></tr>
 <tr><td>GET /work?id=WORK_ID</td><td>Read progress and result in the work field.</td></tr>
 <tr><td>GET /inbox?id=THREAD_ID</td><td>Read the conversation, including delivered results.</td></tr>
 </tbody></table>
 <p>All calls require your token and Accept: application/json; POST requests also require Content-Type: application/json. Usage draws from the same account allowance and balance. After a lost creation or submission response, inspect your agents or work before retrying. To retry reviewed work explicitly, use <code>./mu work retry --id WORK_ID</code> or POST /work with <code>{"action":"retry","id":"WORK_ID"}</code>; previous actions may be repeated.</p>
 <h2>Hosted or self-hosted</h2><p>The CLI defaults to Micro. Run <code>./mu login https://your-server.example</code> to use your own Mu server, or set <code>MU_URL</code> and <code>MU_TOKEN</code>. The HTTP resources stay the same; a self-hosted server needs its own model and service configuration. See <a href="/install">self-hosting</a>.</p>
 <h2>Tools and x402</h2><p>Bring your own agent to <a href="/tools">/mcp</a> or the <a href="/api">services API</a> to call individual tools with a Services token. For wallet-paid public service calls, use <a href="/x402">x402</a>: m3o.com is the machine-readable endpoint. Hosted agent execution uses your Micro account and credits.</p></div>`
	app.Respond(w, r, app.Response{Title: "Developers", HTML: body})
}
