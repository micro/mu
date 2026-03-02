package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/app"
)

// MCPHandler handles both GET (HTML page) and POST (JSON-RPC) at /mcp
func MCPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		mcpPageHandler(w, r)
		return
	}

	if r.Method != "POST" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(jsonrpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32600, Message: "Only POST method is supported"},
		})
		return
	}

	mcpPostHandler(w, r)
}

// mcpPageHandler renders the HTML page listing MCP tools
func mcpPageHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Model Context Protocol</h2>`)
	b.WriteString(`<p class="card-desc">Connect AI clients (e.g. Claude Desktop) to this MCP server.</p>`)
	b.WriteString(`<p>Endpoint: <code>/mcp</code> &mdash; <a href="/api">API Docs</a></p>`)
	b.WriteString(`</div>`)

	// Authentication section
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Authentication</h3>`)
	b.WriteString(`<p>Pass a token in the <code>Authorization</code> header with each request:</p>`)
	b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto">Authorization: Bearer YOUR_TOKEN</pre>`)
	b.WriteString(`<p>Two ways to obtain a token:</p>`)
	b.WriteString(`<ol>`)
	b.WriteString(`<li><strong>Personal Access Token (PAT)</strong> &mdash; create one at <a href="/token">/token</a> after logging in.</li>`)
	b.WriteString(`<li><strong>Signup / Login</strong> &mdash; the agent can call the <code>signup</code> or <code>login</code> tool to obtain a session token programmatically.</li>`)
	b.WriteString(`</ol>`)
	b.WriteString(`</div>`)

	// Test panel
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Test</h3>`)
	b.WriteString(`<form id="mcp-test-form" onsubmit="return sendMCP(event)">`)
	b.WriteString(`<textarea id="mcp-input" rows="5" style="width:100%;font-family:monospace;font-size:13px;padding:8px;box-sizing:border-box;" placeholder='{"jsonrpc":"2.0","id":1,"method":"tools/list"}'></textarea>`)
	b.WriteString(`<button type="submit" style="margin-top:8px">Send</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<pre id="mcp-output" style="display:none;white-space:pre-wrap;word-break:break-all;background:#f5f5f5;padding:10px;margin-top:10px;font-size:12px;max-height:300px;overflow-y:auto;"></pre>`)
	b.WriteString(`<script>`)
	b.WriteString(`function sendMCP(e){e.preventDefault();var inp=document.getElementById('mcp-input');var out=document.getElementById('mcp-output');out.style.display='block';out.textContent='Sending...';fetch('/mcp',{method:'POST',headers:{'Content-Type':'application/json'},body:inp.value}).then(function(r){return r.text();}).then(function(t){try{out.textContent=JSON.stringify(JSON.parse(t),null,2);}catch(ex){out.textContent=t;}}).catch(function(err){out.textContent='Error: '+err;});return false;}`)
	b.WriteString(`function fillAndSend(json){document.getElementById('mcp-input').value=json;}`)
	b.WriteString(`</script>`)
	b.WriteString(`</div>`)

	// Tools list
	b.WriteString(app.List(mcpToolsHTML()))

	app.Respond(w, r, app.Response{
		Title:       "MCP",
		Description: "Model Context Protocol server for AI tool integration",
		HTML:        b.String(),
	})
}

// mcpToolsHTML generates HTML listing all registered MCP tools
func mcpToolsHTML() string {
	var b strings.Builder
	for _, t := range tools {
		b.WriteString(`<div class="card">`)
		b.WriteString(`<span class="card-title">` + html.EscapeString(t.Name) + `</span>`)
		b.WriteString(app.Desc(t.Description))
		if t.WalletOp != "" {
			b.WriteString(`<p class="card-meta">Metered &mdash; requires credits</p>`)
		}
		if len(t.Params) > 0 {
			b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;margin:8px 0">`)
			b.WriteString(`<tr><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Param</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Type</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Description</th></tr>`)
			for _, p := range t.Params {
				req := ""
				if p.Required {
					req = " <span style=\"color:#e55\">*</span>"
				}
				b.WriteString(fmt.Sprintf(`<tr><td style="padding:4px 8px">%s%s</td><td style="padding:4px 8px;color:#888">%s</td><td style="padding:4px 8px">%s</td></tr>`,
					html.EscapeString(p.Name), req,
					html.EscapeString(p.Type),
					html.EscapeString(p.Description),
				))
			}
			b.WriteString(`</table>`)
		}
		// Example JSON-RPC request - use data attribute to avoid JS escaping issues
		example := exampleRequest(t)
		exampleEscaped := html.EscapeString(example)
		b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;cursor:pointer" data-json="` + exampleEscaped + `" onclick="fillAndSend(this.dataset.json)">` + exampleEscaped + `</pre>`)
		b.WriteString(`</div>`)
	}
	return b.String()
}

// exampleRequest generates an example JSON-RPC tools/call request for a tool
func exampleRequest(t Tool) string {
	args := map[string]any{}
	for _, p := range t.Params {
		if p.Required {
			switch p.Type {
			case "number", "integer":
				args[p.Name] = 1
			default:
				args[p.Name] = p.Name
			}
		}
	}
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      t.Name,
			"arguments": args,
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + t.Name + `","arguments":{}}}`
	}
	return string(b)
}
