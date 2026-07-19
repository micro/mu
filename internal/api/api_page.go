package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/wallet"
)

// APIPageHandler renders the API documentation page with interactive playground
func APIPageHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>API</h2>`)
	b.WriteString(`<p class="card-desc">The same services as the <a href="/mcp">MCP server</a>, over plain HTTP. Every tool is callable via <code>POST /mcp</code>; some also have a dedicated REST path. Metered tools show their per-call price below.</p>`)
	b.WriteString(`<p>Authentication: <code>Authorization: Bearer YOUR_TOKEN</code> &mdash; <a href="/token">Get a token</a>, or pay per call with x402.</p>`)
	b.WriteString(`</div>`)

	// Playground
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Playground</h3>`)
	b.WriteString(`<p class="card-desc">Try the dedicated REST endpoints below. Every tool (see the full reference underneath) is also callable via the <a href="/mcp">MCP playground</a>.</p>`)

	// Endpoint selector
	b.WriteString(`<select id="api-endpoint" onchange="apiSelectEndpoint()" style="width:100%;padding:8px;font-size:14px;border:1px solid #ddd;border-radius:4px;margin-bottom:12px">`)
	b.WriteString(`<option value="">Select an endpoint...</option>`)
	for i, ep := range sortedEndpoints() {
		b.WriteString(fmt.Sprintf(`<option value="%d">%s %s — %s</option>`,
			i,
			html.EscapeString(ep.Method),
			html.EscapeString(ep.Path),
			html.EscapeString(ep.Name),
		))
	}
	b.WriteString(`</select>`)

	// Endpoint description
	b.WriteString(`<p id="api-desc" style="color:#666;font-size:13px;margin:0 0 12px 0;display:none"></p>`)

	// Method + path display
	b.WriteString(`<div id="api-method-path" style="display:none;margin-bottom:12px">`)
	b.WriteString(`<span id="api-method-badge" style="display:inline-block;padding:2px 8px;border-radius:3px;font-size:12px;font-weight:600;color:#fff;margin-right:8px"></span>`)
	b.WriteString(`<code id="api-path-display" style="font-size:14px"></code>`)
	b.WriteString(`</div>`)

	// Path params
	b.WriteString(`<div id="api-path-params"></div>`)

	// Body params form
	b.WriteString(`<div id="api-params"></div>`)

	// curl preview
	b.WriteString(`<details id="api-curl-wrap" style="display:none;margin-top:8px">`)
	b.WriteString(`<summary style="cursor:pointer;font-size:12px;color:#888">curl command</summary>`)
	b.WriteString(`<pre id="api-curl" style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;margin-top:6px;border-radius:4px;cursor:pointer" onclick="navigator.clipboard&&navigator.clipboard.writeText(this.textContent)"></pre>`)
	b.WriteString(`</details>`)

	// Send button
	b.WriteString(`<button id="api-send" onclick="apiSend()" disabled style="margin-top:10px;padding:8px 20px;font-size:14px">Send Request</button>`)

	// Output
	b.WriteString(`<div id="api-output" style="display:none;margin-top:12px">`)
	b.WriteString(`<div style="display:flex;justify-content:space-between;align-items:center">`)
	b.WriteString(`<span><strong style="font-size:13px;color:#666">Response</strong> <span id="api-status" style="font-size:12px;margin-left:8px"></span></span>`)
	b.WriteString(`<span id="api-time" style="font-size:12px;color:#999"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<pre id="api-result" style="white-space:pre-wrap;word-break:break-all;background:#f5f5f5;padding:10px;margin-top:6px;font-size:12px;max-height:400px;overflow-y:auto;border-radius:4px"></pre>`)
	b.WriteString(`</div>`)

	// Endpoint metadata as JSON
	b.WriteString(`<script>`)
	epJSON := apiEndpointsJSON()
	b.WriteString(`var apiEndpoints=` + epJSON + `;`)

	b.WriteString(`function apiSelectEndpoint(){`)
	b.WriteString(`var sel=document.getElementById('api-endpoint');`)
	b.WriteString(`var idx=sel.value;`)
	b.WriteString(`var desc=document.getElementById('api-desc');`)
	b.WriteString(`var mp=document.getElementById('api-method-path');`)
	b.WriteString(`var pp=document.getElementById('api-path-params');`)
	b.WriteString(`var params=document.getElementById('api-params');`)
	b.WriteString(`var btn=document.getElementById('api-send');`)
	b.WriteString(`var curl=document.getElementById('api-curl-wrap');`)
	b.WriteString(`if(idx===''){desc.style.display='none';mp.style.display='none';pp.innerHTML='';params.innerHTML='';btn.disabled=true;curl.style.display='none';return;}`)
	b.WriteString(`var ep=apiEndpoints[idx];`)
	b.WriteString(`desc.textContent=ep.description;desc.style.display='block';`)
	b.WriteString(`mp.style.display='block';`)
	// Method badge color
	b.WriteString(`var badge=document.getElementById('api-method-badge');`)
	b.WriteString(`badge.textContent=ep.method;`)
	b.WriteString(`var colors={GET:'#1a7f37',POST:'#cf8e00',PATCH:'#8250df',DELETE:'#cf222e',PUT:'#cf8e00'};`)
	b.WriteString(`badge.style.background=colors[ep.method]||'#555';`)
	b.WriteString(`document.getElementById('api-path-display').textContent=ep.path;`)
	b.WriteString(`btn.disabled=false;curl.style.display='block';`)

	// Path params (e.g. {id}, {username})
	b.WriteString(`var pathParams=ep.path.match(/\{(\w+)\}/g)||[];`)
	b.WriteString(`var ph='';`)
	b.WriteString(`for(var i=0;i<pathParams.length;i++){`)
	b.WriteString(`var pn=pathParams[i].replace(/[{}]/g,'');`)
	b.WriteString(`ph+='<div style="margin-bottom:8px">';`)
	b.WriteString(`ph+='<label style="display:block;font-size:13px;font-weight:500;margin-bottom:2px">'+pn+' <span style=\"color:#e55\">*</span></label>';`)
	b.WriteString(`ph+='<div style="font-size:12px;color:#888;margin-bottom:4px">Path parameter</div>';`)
	b.WriteString(`ph+='<input name="path_'+pn+'" class="api-path-param" data-param="'+pn+'" style="width:100%;padding:6px 8px;font-size:13px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box" placeholder="'+pn+'">';`)
	b.WriteString(`ph+='</div>';}`)
	b.WriteString(`pp.innerHTML=ph;`)

	// Body params
	b.WriteString(`var h='';`)
	b.WriteString(`if(ep.params&&ep.params.length){`)
	b.WriteString(`for(var i=0;i<ep.params.length;i++){var p=ep.params[i];`)
	b.WriteString(`h+='<div style="margin-bottom:8px">';`)
	b.WriteString(`h+='<label style="display:block;font-size:13px;font-weight:500;margin-bottom:2px">'+p.name+'</label>';`)
	b.WriteString(`h+='<div style="font-size:12px;color:#888;margin-bottom:4px">'+p.type+' — '+p.description+'</div>';`)
	b.WriteString(`if(p.name==='prompt'||p.name==='content'||p.name==='body'||p.name==='message'||p.name==='text'){`)
	b.WriteString(`h+='<textarea name="'+p.name+'" class="api-body-param" rows="3" style="width:100%;padding:6px 8px;font-size:13px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box;font-family:inherit;resize:vertical" placeholder="'+p.name+'"></textarea>';`)
	b.WriteString(`}else{`)
	b.WriteString(`h+='<input name="'+p.name+'" class="api-body-param" type="'+(p.type==='number'||p.type==='integer'?'number':'text')+'" style="width:100%;padding:6px 8px;font-size:13px;border:1px solid #ddd;border-radius:4px;box-sizing:border-box" placeholder="'+p.name+'">';`)
	b.WriteString(`}`)
	b.WriteString(`h+='</div>';}`)
	b.WriteString(`}`)
	b.WriteString(`params.innerHTML=h;`)
	b.WriteString(`apiUpdateCurl();`)
	// Update curl on input change
	b.WriteString(`document.querySelectorAll('.api-body-param,.api-path-param').forEach(function(el){el.addEventListener('input',apiUpdateCurl)});`)
	b.WriteString(`}`)

	// Update curl preview
	b.WriteString(`function apiUpdateCurl(){`)
	b.WriteString(`var sel=document.getElementById('api-endpoint').value;if(sel==='')return;`)
	b.WriteString(`var ep=apiEndpoints[sel];`)
	b.WriteString(`var path=ep.path;`)
	// Replace path params
	b.WriteString(`document.querySelectorAll('.api-path-param').forEach(function(el){`)
	b.WriteString(`if(el.value)path=path.replace('{'+el.dataset.param+'}',el.value);`)
	b.WriteString(`});`)
	b.WriteString(`var cmd='curl';`)
	b.WriteString(`if(ep.method!=='GET')cmd+=' -X '+ep.method;`)
	b.WriteString(`cmd+=' -H \"Authorization: Bearer YOUR_TOKEN\"';`)
	// Body
	b.WriteString(`var body={};var hasBody=false;`)
	b.WriteString(`document.querySelectorAll('.api-body-param').forEach(function(el){`)
	b.WriteString(`if(el.value){body[el.name]=el.value;hasBody=true}`)
	b.WriteString(`});`)
	b.WriteString(`if(hasBody){cmd+=' -H \"Content-Type: application/json\"';cmd+=' -d \''+JSON.stringify(body)+'\''}`)
	b.WriteString(`cmd+=' '+location.origin+path;`)
	b.WriteString(`document.getElementById('api-curl').textContent=cmd;`)
	b.WriteString(`}`)

	// Send request
	b.WriteString(`function apiSend(){`)
	b.WriteString(`var sel=document.getElementById('api-endpoint').value;if(sel==='')return;`)
	b.WriteString(`var ep=apiEndpoints[sel];`)
	b.WriteString(`var path=ep.path;`)
	b.WriteString(`document.querySelectorAll('.api-path-param').forEach(function(el){`)
	b.WriteString(`if(el.value)path=path.replace('{'+el.dataset.param+'}',el.value);`)
	b.WriteString(`});`)
	b.WriteString(`var body=null;`)
	b.WriteString(`var hasBody=false;var data={};`)
	b.WriteString(`document.querySelectorAll('.api-body-param').forEach(function(el){`)
	b.WriteString(`if(el.value){data[el.name]=el.value;hasBody=true}`)
	b.WriteString(`});`)
	b.WriteString(`if(hasBody)body=JSON.stringify(data);`)
	b.WriteString(`var out=document.getElementById('api-output');`)
	b.WriteString(`var res=document.getElementById('api-result');`)
	b.WriteString(`var tm=document.getElementById('api-time');`)
	b.WriteString(`var st=document.getElementById('api-status');`)
	b.WriteString(`var btn=document.getElementById('api-send');`)
	b.WriteString(`out.style.display='block';res.textContent='Sending...';res.style.color='';tm.textContent='';st.textContent='';btn.disabled=true;`)
	b.WriteString(`var t0=Date.now();`)
	b.WriteString(`var opts={method:ep.method,headers:{'Accept':'application/json'}};`)
	b.WriteString(`if(body){opts.headers['Content-Type']='application/json';opts.body=body;}`)
	b.WriteString(`fetch(path,opts)`)
	b.WriteString(`.then(function(r){`)
	b.WriteString(`var ms=Date.now()-t0;tm.textContent=ms+'ms';btn.disabled=false;`)
	b.WriteString(`var code=r.status;`)
	b.WriteString(`st.textContent=code+' '+r.statusText;`)
	b.WriteString(`st.style.color=code>=200&&code<300?'#1a7f37':code>=400?'#cf222e':'#cf8e00';`)
	b.WriteString(`return r.text();`)
	b.WriteString(`}).then(function(t){`)
	b.WriteString(`try{res.textContent=JSON.stringify(JSON.parse(t),null,2)}catch(e){res.textContent=t}`)
	b.WriteString(`}).catch(function(err){btn.disabled=false;res.textContent='Error: '+err;res.style.color='#c00';});`)
	b.WriteString(`}`)
	b.WriteString(`</script>`)
	b.WriteString(`</div>`)

	// Endpoint reference list with a sticky endpoint index (desktop only).
	b.WriteString(apiEndpointsSection())

	app.Respond(w, r, app.Response{
		Title:       "API",
		Description: "API documentation",
		HTML:        b.String(),
	})
}

// apiEndpointsJSON returns endpoint metadata as JSON for the playground JS
func apiEndpointsJSON() string {
	var eps []map[string]any
	for _, ep := range sortedEndpoints() {
		params := []map[string]any{}
		for _, p := range ep.Params {
			params = append(params, map[string]any{
				"name":        p.Name,
				"type":        p.Value,
				"description": p.Description,
			})
		}
		eps = append(eps, map[string]any{
			"name":        ep.Name,
			"path":        ep.Path,
			"method":      ep.Method,
			"description": ep.Description,
			"params":      params,
		})
	}
	b, _ := json.Marshal(eps)
	return string(b)
}

// apiEndpointsSection renders the reference from the same tool registry as the
// MCP page, so the two are always the same set of services. Every tool is
// callable over HTTP — via its dedicated REST path when it has one, otherwise
// via POST /mcp — and metered tools show their per-call price.
func apiEndpointsSection() string {
	var nav strings.Builder
	nav.WriteString(`<nav class="ep-nav"><div class="ep-nav-title">Endpoints</div>`)
	for _, t := range sortedTools() {
		price := ""
		if p := wallet.X402PriceFor(t.WalletOp); p != "" {
			price = `<span class="ep-price">` + p + `</span>`
		}
		nav.WriteString(`<a href="#api-` + html.EscapeString(t.Name) + `"><span class="ep-path">` + html.EscapeString(t.Name) + `</span>` + price + `</a>`)
	}
	nav.WriteString(`</nav>`)
	return `<div class="ep-layout">` + nav.String() + `<div class="ep-main">` + app.List(apiEndpointsHTML()) + `</div></div>`
}

// apiEndpointsHTML lists every tool as an HTTP endpoint, with its price and a
// ready-to-run call (REST path if it has one, else the POST /mcp JSON-RPC body).
func apiEndpointsHTML() string {
	var b strings.Builder
	for _, t := range sortedTools() {
		b.WriteString(`<div class="card" id="api-` + html.EscapeString(t.Name) + `">`)

		if p := wallet.X402PriceFor(t.WalletOp); p != "" {
			b.WriteString(`<span class="tool-price"><b>` + p + `</b> <span>/ call</span></span>`)
		} else if t.WalletOp != "" {
			b.WriteString(`<span class="tool-price">credits</span>`)
		}

		// Title: the REST method+path when the tool has one, else the tool name.
		if t.Path != "" {
			method := t.Method
			if method == "" {
				method = "GET"
			}
			b.WriteString(`<span class="card-title"><span class="api-method">` + html.EscapeString(method) + `</span> <code>` + html.EscapeString(t.Path) + `</code></span>`)
		} else {
			b.WriteString(`<span class="card-title">` + html.EscapeString(t.Name) + `</span>`)
		}
		b.WriteString(app.Desc(t.Description))

		if len(t.Params) > 0 {
			b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;margin:8px 0">`)
			b.WriteString(`<tr><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Param</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Type</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Description</th></tr>`)
			for _, p := range t.Params {
				req := ""
				if p.Required {
					req = ` <span style="color:#e55">*</span>`
				}
				b.WriteString(fmt.Sprintf(`<tr><td style="padding:4px 8px">%s%s</td><td style="padding:4px 8px;color:#888">%s</td><td style="padding:4px 8px">%s</td></tr>`,
					html.EscapeString(p.Name), req, html.EscapeString(p.Type), html.EscapeString(p.Description)))
			}
			b.WriteString(`</table>`)
		}

		// Call example.
		var call string
		if t.Path != "" {
			method := t.Method
			if method == "" {
				method = "GET"
			}
			call = "curl -H 'Authorization: Bearer $TOKEN' -X " + method + " " + t.Path
		} else {
			call = "curl -X POST /mcp -H 'Content-Type: application/json' \\\n  -d '" + exampleRequest(t) + "'"
		}
		esc := html.EscapeString(call)
		b.WriteString(`<pre style="background:#f5f5f5;padding:8px;font-size:12px;overflow-x:auto;white-space:pre-wrap;word-break:break-word">` + esc + `</pre>`)
		b.WriteString(`</div>`)
	}
	return b.String()
}
