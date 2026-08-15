package api

import (
	"encoding/json"
	"fmt"
	"html"
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
	b.WriteString(`<p class="card-desc">Every tool this instance serves, with its parameters and a playground to run it.</p>`)
	b.WriteString(`<p>Endpoint: <code>POST /mcp</code> &mdash; calls carry <code>Authorization: Bearer</code>.</p>`)
	b.WriteString(`<p class="card-meta">Not connected yet? <a href="/tools#connect">Connect your agent &rarr;</a> ` +
		`&middot; <a href="/help/mcp">Auth and protocol detail</a> &middot; <a href="/pricing">What calls cost</a></p>`)
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
	b.WriteString(`<select id="mcp-tool" onchange="mcpSelectTool()" style="width:100%;padding:8px;font-size:14px;border:1px solid #ddd;border-radius:4px;margin-bottom:12px">`)
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
	b.WriteString(`<p id="mcp-tool-desc" style="color:#666;font-size:13px;margin:0 0 12px 0;display:none"></p>`)

	// Dynamic params form
	b.WriteString(`<div id="mcp-params"></div>`)

	// Run button
	b.WriteString(`<button id="mcp-run" onclick="mcpRun()" disabled style="margin-top:8px;padding:8px 20px;font-size:14px">Run</button>`)

	// Output area
	b.WriteString(`<div id="mcp-output" style="display:none;margin-top:12px">`)
	b.WriteString(`<div style="display:flex;justify-content:space-between;align-items:center">`)
	b.WriteString(`<strong style="font-size:13px;color:#666">Response</strong>`)
	b.WriteString(`<span id="mcp-time" style="font-size:12px;color:#999"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<pre id="mcp-result" style="white-space:pre-wrap;word-break:break-all;background:#f5f5f5;padding:10px;margin-top:6px;font-size:12px;max-height:400px;overflow-y:auto;border-radius:4px"></pre>`)
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
	b.WriteString(`h+='<div style="margin-bottom:8px">';`)
	b.WriteString(`h+='<label style="display:block;font-size:13px;font-weight:500;margin-bottom:2px">'+p.name+(p.required?' <span style=\"color:#e55\">*</span>':'')+'</label>';`)
	b.WriteString(`h+='<div style="font-size:12px;color:#888;margin-bottom:4px">'+p.description+'</div>';`)
	b.WriteString(`if(p.type==='string'&&(p.name==='prompt'||p.name==='body'||p.name==='content'||p.name==='message'||p.name==='text')){`)
	b.WriteString(`h+='<textarea name="'+p.name+'" rows="3" style="width:100%;padding:6px 8px;font-size:13px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box;font-family:inherit;resize:vertical" placeholder="'+p.name+'"></textarea>';`)
	b.WriteString(`}else{`)
	b.WriteString(`h+='<input name="'+p.name+'" type="'+(p.type==='number'||p.type==='integer'?'number':'text')+'" style="width:100%;padding:6px 8px;font-size:13px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box" placeholder="'+p.name+'">';`)
	b.WriteString(`}`)
	b.WriteString(`h+='</div>';}`)
	b.WriteString(`}else{h='<p style="color:#999;font-size:13px">No parameters needed</p>';}`)
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
	b.WriteString(`<summary style="cursor:pointer;font-weight:500">Raw JSON-RPC</summary>`)
	b.WriteString(`<div style="margin-top:10px">`)
	b.WriteString(`<form id="mcp-test-form" onsubmit="return sendMCP(event)">`)
	b.WriteString(`<textarea id="mcp-input" rows="5" style="width:100%;font-family:monospace;font-size:13px;padding:8px;box-sizing:border-box;" placeholder='{"jsonrpc":"2.0","id":1,"method":"tools/list"}'></textarea>`)
	b.WriteString(`<button type="submit" style="margin-top:8px">Send</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`<pre id="mcp-raw-output" style="display:none;white-space:pre-wrap;word-break:break-all;background:#f5f5f5;padding:10px;margin-top:10px;font-size:12px;max-height:300px;overflow-y:auto;"></pre>`)
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
