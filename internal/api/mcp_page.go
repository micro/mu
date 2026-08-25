package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/quota"
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

	// A bound on the request, before anything reads it.
	//
	// Nothing capped this. serveMCP buffers the body to see whether the call is
	// a tools/list, and that read is the first thing that happens — before the
	// gateway authenticates the caller and before metering charges anybody. So
	// an unauthenticated POST of an arbitrarily large body was an allocation an
	// anonymous caller could ask this process to make, repeatedly, for free.
	//
	// The door next to it already had one: /api caps a call's arguments at 1 MB
	// (rest.go), serving the same tools to the same kind of client. This is that
	// limit, at the other door, so the pair cannot disagree about how big a
	// request may be — and it is deliberately the same constant rather than a
	// second opinion about it.
	//
	// MaxBytesReader rather than a length check, because Content-Length is a
	// claim the sender makes and a chunked request does not make it at all. It
	// bounds what is actually read.
	//
	// Read here rather than deeper, and the overflow answered as a refusal. Left
	// to serveMCP the oversized body would be truncated at the limit and handed
	// on, and the gateway would answer whatever a JSON document cut in half
	// parses as — a syntax error, blamed on the caller's JSON rather than on its
	// size, which is the kind of message somebody debugs for an hour.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		json.NewEncoder(w).Encode(jsonrpcResponse{
			JSONRPC: "2.0",
			Error: &rpcError{Code: -32600, Message: "Request too large: /mcp accepts at most " +
				strconv.Itoa(maxRequestBytes/1024) + " KB"},
		})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Protocol served by go-micro's gateway/mcp; tools + metering stay mu's.
	serveMCP(w, r)
}

// mcpPageHandler renders the HTML page listing MCP tools
func mcpPageHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	// One header, then the reference.
	//
	// This page used to open with a config block and an authentication section —
	// which is /tools' connect step, written again in different words. Two pages
	// explaining how to connect means two places to keep true, and the reader
	// who has landed here has usually already connected. /tools owns "connect",
	// this page owns "call". Setup is a link.
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Model Context Protocol</h2>`)
	b.WriteString(`<p class="card-desc">Every tool this instance serves, with its parameters and a playground to run it. ` +
		`The inbox comes with them — <code>recall_list</code>, <code>recall_conversation</code> and ` +
		`<code>recall_search</code> read what arrived here, so a client on the other end sees the ` +
		`conversations as well as the capabilities.</p>`)
	b.WriteString(`<p>Endpoint: <code>POST /mcp</code> &mdash; calls carry <code>Authorization: Bearer</code>.</p>`)
	// Or pay instead of signing up, which is the point of the rail and was
	// stated on /api and nowhere else — including here, the door an agent
	// actually uses. Every tool below carries its price; this says how a caller
	// with no account settles it.
	b.WriteString(`<p>Or no token: a priced tool answers an unauthenticated call ` +
		`with <code>402</code> and an <a href="https://x402.org">x402</a> challenge. ` +
		`Pay it in USDC on Base and the same request succeeds. No signup. ` +
		`<a href="/pricing">Pricing</a>.</p>`)
	b.WriteString(`<p class="card-meta">Not connected yet? <a href="/tools#connect">Connect your agent &rarr;</a> ` +
		`&middot; <a href="/help/mcp">Auth and protocol detail</a> &middot; <a href="/tools">What calls cost</a></p>`)
	// Not everybody arriving here is building an agent. Somebody writing a
	// client wants one URL per method, and being handed a tool-calling protocol
	// because it was the only door documented is how they conclude this is not
	// for them.
	b.WriteString(`<p class="card-meta">Not building an agent? <a href="/api">The HTTP API</a> ` +
		`serves the same methods as plain <code>GET</code> and <code>POST</code>.</p>`)
	b.WriteString(`</div>`)

	// Interactive playground
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Playground</h3>`)
	b.WriteString(`<p class="card-desc">Pick a tool, fill in parameters, and run it.</p>`)

	// Tool selector
	b.WriteString(`<select id="mcp-tool" onchange="mcpSelectTool()" class="form-input w-full text-base mb-3">`)
	b.WriteString(`<option value="">Select a tool...</option>`)
	for _, t := range mcpTools() {
		metered := ""
		if t.WalletOp != "" {
			metered = " (metered)"
		}
		b.WriteString(`<option value="` + html.EscapeString(t.Name) + `">` + html.EscapeString(t.Name) + metered + `</option>`)
	}
	b.WriteString(`</select>`)

	// Tool description
	b.WriteString(`<p id="mcp-tool-desc" class="text-secondary text-sm m-0 mb-3 d-none"></p>`)

	// Dynamic params form
	b.WriteString(`<div id="mcp-params"></div>`)

	// Run button
	b.WriteString(`<button id="mcp-run" onclick="mcpRun()" disabled class="mt-2">Run</button>`)

	// Output area
	b.WriteString(`<div id="mcp-output" class="d-none mt-3">`)
	b.WriteString(`<div class="d-flex between items-center">`)
	b.WriteString(`<strong class="text-sm text-secondary">Response</strong>`)
	b.WriteString(`<span id="mcp-time" class="text-xs text-muted"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<pre id="mcp-result" class="output"></pre>`)
	b.WriteString(`</div>`)

	// Tool metadata as JSON for JS
	b.WriteString(`<script>`)
	toolsJSON := mcpToolsJSON()
	b.WriteString(`var mcpTools=` + toolsJSON + `;`)
	b.WriteString(`function mcpSelectTool(){`)
	b.WriteString(`var sel=document.getElementById('mcp-tool').value;`)
	b.WriteString(`var desc=document.getElementById('mcp-tool-desc');`)
	b.WriteString(`var params=document.getElementById('mcp-params');`)
	b.WriteString(`var btn=document.getElementById('mcp-run');`)
	b.WriteString(`if(!sel){desc.style.display='none';params.innerHTML='';btn.disabled=true;return;}`)
	b.WriteString(`var t=mcpTools[sel];`)
	b.WriteString(`desc.textContent=t.description;desc.style.display='block';`)
	b.WriteString(`btn.disabled=false;`)
	b.WriteString(`var h='';`)
	b.WriteString(`if(t.params&&t.params.length){`)
	b.WriteString(`for(var i=0;i<t.params.length;i++){var p=t.params[i];`)
	b.WriteString(`h+='<div class="mb-2">';`)
	b.WriteString(`h+='<label class="d-block text-sm mb-1 field-label">'+p.name+(p.required?' <span style=\"color:#e55\">*</span>':'')+'</label>';`)
	b.WriteString(`h+='<div class="text-xs text-muted mb-1">'+p.description+'</div>';`)
	b.WriteString(`if(p.type==='string'&&(p.name==='prompt'||p.name==='body'||p.name==='content'||p.name==='message'||p.name==='text')){`)
	b.WriteString(`h+='<textarea name="'+p.name+'" rows="3" class="form-area text-sm" placeholder="'+p.name+'"></textarea>';`)
	b.WriteString(`}else{`)
	b.WriteString(`h+='<input name="'+p.name+'" type="'+(p.type==='number'||p.type==='integer'?'number':'text')+'" class="form-input w-full text-sm" placeholder="'+p.name+'">';`)
	b.WriteString(`}`)
	b.WriteString(`h+='</div>';}`)
	b.WriteString(`}else{h='<p class="text-muted text-sm">No parameters needed</p>';}`)
	b.WriteString(`params.innerHTML=h;`)
	b.WriteString(`}`)

	b.WriteString(`function mcpRun(){`)
	b.WriteString(`var name=document.getElementById('mcp-tool').value;if(!name)return;`)
	b.WriteString(`var args={};`)
	b.WriteString(`var inputs=document.getElementById('mcp-params').querySelectorAll('input,textarea');`)
	b.WriteString(`for(var i=0;i<inputs.length;i++){var v=inputs[i].value;if(v){`)
	b.WriteString(`var t=mcpTools[name].params.find(function(p){return p.name===inputs[i].name});`)
	b.WriteString(`if(t&&(t.type==='number'||t.type==='integer')){args[inputs[i].name]=Number(v)}else{args[inputs[i].name]=v}`)
	b.WriteString(`}}`)
	b.WriteString(`var body=JSON.stringify({jsonrpc:'2.0',id:1,method:'tools/call',params:{name:name,arguments:args}});`)
	b.WriteString(`var out=document.getElementById('mcp-output');`)
	b.WriteString(`var res=document.getElementById('mcp-result');`)
	b.WriteString(`var tm=document.getElementById('mcp-time');`)
	b.WriteString(`var btn=document.getElementById('mcp-run');`)
	b.WriteString(`out.style.display='block';res.textContent='Running...';tm.textContent='';btn.disabled=true;`)
	b.WriteString(`var t0=Date.now();`)
	b.WriteString(`fetch('/mcp',{method:'POST',headers:{'Content-Type':'application/json'},body:body})`)
	b.WriteString(`.then(function(r){return r.json()})`)
	b.WriteString(`.then(function(j){`)
	b.WriteString(`var ms=Date.now()-t0;tm.textContent=ms+'ms';btn.disabled=false;`)
	b.WriteString(`if(j.error){res.textContent='Error: '+j.error.message;res.style.color='#c00';return;}`)
	b.WriteString(`res.style.color='';`)
	b.WriteString(`if(j.result&&j.result.content&&j.result.content.length){`)
	b.WriteString(`var txt=j.result.content.map(function(c){return c.text||JSON.stringify(c)}).join('\n');`)
	b.WriteString(`try{var parsed=JSON.parse(txt);res.textContent=JSON.stringify(parsed,null,2)}catch(e){res.textContent=txt}`)
	b.WriteString(`}else{res.textContent=JSON.stringify(j.result||j,null,2)}`)
	b.WriteString(`}).catch(function(err){btn.disabled=false;res.textContent='Error: '+err;res.style.color='#c00';});`)
	b.WriteString(`}`)
	b.WriteString(`</script>`)
	b.WriteString(`</div>`)

	// Raw JSON-RPC panel (collapsed)
	b.WriteString(`<div class="card">`)
	b.WriteString(`<details>`)
	b.WriteString(`<summary class="clickable medium">Raw JSON-RPC</summary>`)
	b.WriteString(`<div class="mt-3">`)
	b.WriteString(`<form id="mcp-test-form" onsubmit="return sendMCP(event)">`)
	b.WriteString(`<textarea id="mcp-input" rows="5" class="form-input w-full text-sm mono" placeholder='{"jsonrpc":"2.0","id":1,"method":"tools/list"}'></textarea>`)
	b.WriteString(`<button type="submit" class="mt-2">Send</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<pre id="mcp-raw-output" class="output d-none"></pre>`)
	b.WriteString(`</div>`)
	b.WriteString(`<script>`)
	b.WriteString(`function sendMCP(e){e.preventDefault();var inp=document.getElementById('mcp-input');var out=document.getElementById('mcp-raw-output');out.style.display='block';out.textContent='Sending...';fetch('/mcp',{method:'POST',headers:{'Content-Type':'application/json'},body:inp.value}).then(function(r){return r.text();}).then(function(t){try{out.textContent=JSON.stringify(JSON.parse(t),null,2);}catch(ex){out.textContent=t;}}).catch(function(err){out.textContent='Error: '+err;});return false;}`)
	b.WriteString(`function fillAndSend(json){document.getElementById('mcp-input').value=json;}`)
	b.WriteString(`</script>`)
	b.WriteString(`</details>`)
	b.WriteString(`</div>`)

	// No pricing card. Every tool below carries its own price, and /pricing is
	// linked from the header — a card restating the rule in prose was a third
	// place for it to be wrong.

	// Tools list with a sticky endpoint sidebar (desktop).
	b.WriteString(mcpToolsSection())

	app.Respond(w, r, app.Response{
		Title:       "MCP",
		Description: "Model Context Protocol server for AI tool integration",
		HTML:        b.String(),
	})
}

// mcpToolsJSON returns a JSON object keyed by tool name with description and
// params, for the playground's form builder.
//
// It reads mcpTools(), not the raw slice. Reading the raw slice put the two
// RESTOnly entries in here but not in the selector that indexes into it — two
// lists over the same data, one filtered and one not, which is the same way the
// old /api page came to document twenty-five endpoints and offer twenty.
func mcpToolsJSON() string {
	m := map[string]any{}
	for _, t := range mcpTools() {
		params := []map[string]any{}
		for _, p := range t.Params {
			params = append(params, map[string]any{
				"name":        p.Name,
				"type":        p.Type,
				"description": p.Description,
				"required":    p.Required,
			})
		}
		m[t.Name] = map[string]any{
			"description": t.Description,
			"params":      params,
			"metered":     t.WalletOp != "",
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// mcpToolsSection renders the tool reference as a two-column layout: a sticky
// endpoint index on the left (desktop only) anchoring to each tool card on the
// right. On mobile the index is hidden and the cards stack normally.
func mcpToolsSection() string {
	var nav strings.Builder
	nav.WriteString(`<nav class="ep-nav"><div class="ep-nav-title">Tools</div>`)
	for _, t := range mcpTools() {
		price := ""
		if n := quota.OperationCost(t.WalletOp); t.WalletOp != "" && n > 0 {
			price = `<span class="ep-price">` + strconv.Itoa(n) + `</span>`
		}
		nav.WriteString(`<a href="#tool-` + html.EscapeString(t.Name) + `">` + html.EscapeString(t.Name) + price + `</a>`)
	}
	nav.WriteString(`</nav>`)
	return `<div class="ep-layout">` + nav.String() + `<div class="ep-main">` + app.List(mcpToolsHTML()) + `</div></div>`
}

// mcpToolsHTML generates HTML listing all registered MCP tools
func mcpToolsHTML() string {
	var b strings.Builder
	for _, t := range mcpTools() {
		cost := 0
		if t.WalletOp != "" {
			cost = quota.OperationCost(t.WalletOp)
		}
		b.WriteString(`<div class="card" id="tool-` + html.EscapeString(t.Name) + `">`)
		if cost > 0 {
			b.WriteString(`<span class="tool-price"><b>` + strconv.Itoa(cost) + `</b> <span>` + creditWord(cost) + `</span></span>`)
		}
		b.WriteString(`<span class="card-title">` + html.EscapeString(t.Name) + `</span>`)
		b.WriteString(app.Desc(t.Description))
		if cost > 0 {
			b.WriteString(`<p class="card-meta">Draws ` + strconv.Itoa(cost) + ` ` + creditWord(cost) + ` from your balance per call.</p>`)
		}
		if len(t.Params) > 0 {
			b.WriteString(`<table class="rule-table">`)
			b.WriteString(`<tr><th >Param</th><th >Type</th><th >Description</th></tr>`)
			for _, p := range t.Params {
				req := ""
				if p.Required {
					req = " <span style=\"color:#e55\">*</span>"
				}
				b.WriteString(fmt.Sprintf(`<tr><td >%s%s</td><td class="text-muted">%s</td><td >%s</td></tr>`,
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
		b.WriteString(`<pre class="bg-soft p-2 text-xs scroll-x clickable" data-json="` + exampleEscaped + `" onclick="fillAndSend(this.dataset.json)">` + exampleEscaped + `</pre>`)
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
