package api

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
)

// APIPageHandler renders the API documentation page with interactive playground
func APIPageHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder

	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>API</h2>`)
	b.WriteString(`<p class="card-desc">REST API for programmatic access to Mu.</p>`)
	b.WriteString(`<p>Authentication: <code>Authorization: Bearer YOUR_TOKEN</code> &mdash; <a href="/token">Get a token</a> | <a href="/mcp">MCP Server</a></p>`)
	b.WriteString(`</div>`)

	// Playground
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h3>Playground</h3>`)
	b.WriteString(`<p class="card-desc">Pick an endpoint, fill in parameters, and send the request.</p>`)

	// Endpoint selector
	b.WriteString(`<select id="api-endpoint" onchange="apiSelectEndpoint()" style="width:100%;padding:8px;font-size:14px;border:1px solid #ddd;border-radius:4px;margin-bottom:12px">`)
	b.WriteString(`<option value="">Select an endpoint...</option>`)
	for i, ep := range Endpoints {
		methodColor := "#1a7f37"
		switch ep.Method {
		case "POST":
			methodColor = "#cf8e00"
		case "PATCH":
			methodColor = "#8250df"
		case "DELETE":
			methodColor = "#cf222e"
		}
		b.WriteString(fmt.Sprintf(`<option value="%d" data-method="%s">%s %s — %s</option>`,
			i,
			html.EscapeString(ep.Method),
			html.EscapeString(ep.Method),
			html.EscapeString(ep.Path),
			html.EscapeString(ep.Name),
		))
		_ = methodColor
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

	// Endpoint reference list
	b.WriteString(app.List(apiEndpointsHTML()))

	app.Respond(w, r, app.Response{
		Title:       "API",
		Description: "API documentation",
		HTML:        b.String(),
	})
}

// apiEndpointsJSON returns endpoint metadata as JSON for the playground JS
func apiEndpointsJSON() string {
	var eps []map[string]any
	for _, ep := range Endpoints {
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

// apiEndpointsHTML generates the reference listing of all API endpoints
func apiEndpointsHTML() string {
	var b strings.Builder
	for _, ep := range Endpoints {
		methodColor := "#1a7f37"
		switch ep.Method {
		case "POST":
			methodColor = "#cf8e00"
		case "PATCH":
			methodColor = "#8250df"
		case "DELETE":
			methodColor = "#cf222e"
		}

		b.WriteString(`<div class="card">`)
		b.WriteString(fmt.Sprintf(`<span class="card-title"><span style="display:inline-block;padding:1px 6px;border-radius:3px;font-size:11px;font-weight:600;color:#fff;background:%s;margin-right:6px">%s</span> %s</span>`,
			methodColor,
			html.EscapeString(ep.Method),
			html.EscapeString(ep.Path),
		))
		b.WriteString(app.Desc(ep.Name + " — " + ep.Description))

		if len(ep.Params) > 0 {
			b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;margin:8px 0">`)
			b.WriteString(`<tr><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Param</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Type</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Description</th></tr>`)
			for _, p := range ep.Params {
				b.WriteString(fmt.Sprintf(`<tr><td style="padding:4px 8px">%s</td><td style="padding:4px 8px;color:#888">%s</td><td style="padding:4px 8px">%s</td></tr>`,
					html.EscapeString(p.Name),
					html.EscapeString(p.Value),
					html.EscapeString(p.Description),
				))
			}
			b.WriteString(`</table>`)
		}

		if len(ep.Response) > 0 {
			for _, resp := range ep.Response {
				if len(resp.Params) > 0 {
					b.WriteString(`<p style="font-size:12px;color:#888;margin:8px 0 4px 0">Response: ` + html.EscapeString(resp.Type) + `</p>`)
					b.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:13px;margin:0 0 8px 0">`)
					b.WriteString(`<tr><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Field</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Type</th><th style="text-align:left;padding:4px 8px;border-bottom:1px solid #eee">Description</th></tr>`)
					for _, p := range resp.Params {
						b.WriteString(fmt.Sprintf(`<tr><td style="padding:4px 8px">%s</td><td style="padding:4px 8px;color:#888">%s</td><td style="padding:4px 8px">%s</td></tr>`,
							html.EscapeString(p.Name),
							html.EscapeString(p.Value),
							html.EscapeString(p.Description),
						))
					}
					b.WriteString(`</table>`)
				}
			}
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}
