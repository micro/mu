package agent

import (
	"encoding/json"
	"net/http"
	"strings"

	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/auth"
)

// AgentsHandler is the CRUD API for user-defined agents at /agent/agents.
//
//	GET  → { agents: [user agents], builtins: [{id,name,description}] }
//	POST action=save   (name, prompt, description, id?, fork?) → saved agent
//	POST action=delete (id)                                    → { ok: true }
func AgentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, acc := auth.TrySession(r)
	if acc == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "login required"})
		return
	}

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "generate":
			spec, err := generateAgentSpec(strings.TrimSpace(r.FormValue("brief")))
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(spec)
			return
		case "delete":
			micro.DeleteUserAgentFor(acc.ID, r.FormValue("id"))
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		default: // save
			name := strings.TrimSpace(r.FormValue("name"))
			prompt := strings.TrimSpace(r.FormValue("prompt"))
			if name == "" || prompt == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "name and prompt are required"})
				return
			}
			saved := micro.SaveUserAgent(acc.ID, &micro.Agent{
				ID:           r.FormValue("id"),
				Name:         name,
				Description:  strings.TrimSpace(r.FormValue("description")),
				SystemPrompt: prompt,
				Tools:        r.Form["tools"], // empty = all tools
				ForkedFrom:   r.FormValue("fork"),
			})
			_ = json.NewEncoder(w).Encode(saved)
			return
		}
	}

	// GET: list
	type lite struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Prompt      string   `json:"prompt,omitempty"`
		Tools       []string `json:"tools,omitempty"`
	}
	var mine []lite
	for _, a := range micro.UserAgentsFor(acc.ID) {
		mine = append(mine, lite{a.ID, a.Name, a.Description, a.SystemPrompt, a.Tools})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"agents": mine, "tools": AllAgentTools()})
}

// generateAgentSpec turns a one-line brief into a full agent spec (name,
// description, system prompt) using the LLM — the "describe it and it becomes an
// agent" flow.
func generateAgentSpec(brief string) (map[string]string, error) {
	if brief == "" {
		return nil, errBadBrief
	}
	if !ai.Configured() {
		return nil, errNoAI
	}
	sys := `You design AI agent personas for Mu, a personal assistant with tools for news, markets, weather, mail, web search, places, video, and social. Given a brief, output ONLY minified JSON with exactly these keys:
"name": a short label, <=40 chars, no emoji;
"description": one line, <=120 chars;
"prompt": a system prompt of 2-4 sentences in second person ("You are ...") defining the persona, tone, priorities, and which kinds of tools/data to lean on.
No markdown, no code fences, no commentary — just the JSON object.`
	out, err := ai.Ask(&ai.Prompt{System: sys, Question: "Brief: " + brief, Caller: "agent_builder", MaxTokens: 500})
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	out = strings.TrimSpace(out)
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil || strings.TrimSpace(m["prompt"]) == "" {
		// Model didn't return clean JSON — fall back to using the text as the prompt.
		return map[string]string{"name": "", "description": "", "prompt": out}, nil
	}
	return m, nil
}

var (
	errBadBrief = &agentErr{"describe the agent in a sentence first"}
	errNoAI     = &agentErr{"AI is not configured on this instance"}
)

type agentErr struct{ s string }

func (e *agentErr) Error() string { return e.s }

// renderAgentsPanel renders the "Agents" card for the sessions rail: pick the
// default agent or a custom one, and create/fork/delete your own.
func renderAgentsPanel() string {
	return `<div class="agents-panel">
  <div class="agents-head"><span>Agents</span><button type="button" class="agents-new" onclick="muAgentForm()">+ New</button></div>
  <div class="agents-list" id="agents-list"><div class="agents-active" data-id="" onclick="muAgentPick('','Micro')">Micro <span class="agents-def">default</span></div></div>
  <form class="agents-form" id="agents-form" hidden onsubmit="return muAgentSave(event)">
    <input type="hidden" id="agf-id"><input type="hidden" id="agf-fork">
    <div class="agents-gen">
      <input id="agf-brief" placeholder="Describe your agent in a sentence…">
      <button type="button" id="agf-genbtn" onclick="muAgentGen()">✨ Generate</button>
    </div>
    <input id="agf-name" placeholder="Agent name (e.g. Crypto Researcher)" maxlength="60" required>
    <input id="agf-desc" placeholder="One-line description (optional)" maxlength="140">
    <textarea id="agf-prompt" rows="5" placeholder="System prompt — who this agent is and how it should behave. e.g. 'You are a meticulous crypto research analyst. Always cite sources and quote exact prices.'" required></textarea>
    <div class="agents-tools" id="agf-tools"></div>
    <div class="agents-formactions">
      <button type="submit">Save agent</button>
      <button type="button" onclick="muAgentCancel()">Cancel</button>
    </div>
  </form>
</div>
<style>
.agents-panel{border:1px solid var(--card-border,#e8e8e8);border-radius:8px;margin-bottom:12px;background:var(--card-background,#fff);overflow:hidden}
.agents-head{display:flex;justify-content:space-between;align-items:center;padding:10px 12px;font-size:14px;font-weight:600;border-bottom:1px solid var(--card-border,#eee)}
.agents-new{border:0;background:none;color:#4f46e5;cursor:pointer;font-size:13px;font-weight:600}
.agents-list{padding:6px}
.agents-list>div{display:flex;justify-content:space-between;align-items:center;gap:6px;padding:7px 8px;border-radius:6px;cursor:pointer;font-size:13px}
.agents-list>div:hover{background:#f4f4f5}
.agents-list>div.on{background:#eef2ff;color:#3730a3;font-weight:600}
.agents-def{color:#aaa;font-size:11px;font-weight:400}
.agents-actions{display:flex;gap:4px;opacity:.6}
.agents-actions button{border:0;background:none;cursor:pointer;font-size:12px;padding:0 2px}
.agents-form{padding:10px 12px;border-top:1px solid var(--card-border,#eee);display:flex;flex-direction:column;gap:8px}
.agents-form input,.agents-form textarea{width:100%;box-sizing:border-box;padding:7px 8px;font-size:13px;border:1px solid #ddd;border-radius:5px;font-family:inherit}
.agents-formactions{display:flex;gap:6px}
.agents-formactions button{flex:1;padding:7px;font-size:13px;border:1px solid #ddd;border-radius:5px;cursor:pointer;background:#fafafa}
.agents-formactions button[type=submit]{background:#4f46e5;color:#fff;border-color:#4f46e5}
.agents-gen{display:flex;gap:6px}
.agents-gen input{flex:1}
.agents-gen button{white-space:nowrap;padding:7px 10px;font-size:13px;border:1px solid #c7d2fe;border-radius:5px;cursor:pointer;background:#eef2ff;color:#4338ca}
.agents-gen button[disabled]{opacity:.6;cursor:default}
.agents-tools{display:flex;flex-wrap:wrap;gap:6px 10px}
.agents-toolslabel{width:100%;font-size:11px;color:#888;text-transform:uppercase;letter-spacing:.04em}
.agents-toolslabel span{text-transform:none;letter-spacing:0}
.agents-tools label{display:flex;align-items:center;gap:4px;font-size:12px;color:#444;cursor:pointer}
</style>
<script>
window.muActiveAgent=window.muActiveAgent||'';
function muAgentCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function muAgentPick(id,name){window.muActiveAgent=id;document.querySelectorAll('#agents-list>div').forEach(function(d){d.classList.toggle('on',d.getAttribute('data-id')===id);});}
function muAgentForm(a){var f=document.getElementById('agents-form');f.hidden=false;
  document.getElementById('agf-id').value=(a&&a.id&&!a._fork)?a.id:'';
  document.getElementById('agf-fork').value=(a&&a._fork)?a.id:'';
  document.getElementById('agf-name').value=a?(a._fork?('Copy of '+a.name):a.name):'';
  document.getElementById('agf-desc').value=a?(a.description||''):'';
  document.getElementById('agf-prompt').value=a?(a.prompt||''):'';
  muAgentRenderTools(a?a.tools:[]);
  document.getElementById('agf-name').focus();}
function muAgentRenderTools(sel){var c=document.getElementById('agf-tools');if(!c)return;var s=sel||[];
  var h='<div class="agents-toolslabel">Tools <span>(none selected = all)</span></div>';
  (window._muTools||[]).forEach(function(t){h+='<label><input type="checkbox" name="agtool" value="'+t+'"'+(s.indexOf(t)>=0?' checked':'')+'>'+t+'</label>';});
  c.innerHTML=h;}
function muAgentCancel(){document.getElementById('agents-form').hidden=true;}
function muAgentGen(){
  var brief=document.getElementById('agf-brief').value.trim();if(!brief)return;
  var btn=document.getElementById('agf-genbtn');btn.disabled=true;btn.textContent='Generating…';
  var b=new URLSearchParams();b.append('action','generate');b.append('brief',brief);
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muAgentCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(d){
      btn.disabled=false;btn.textContent='✨ Generate';
      if(d.error){alert(d.error);return;}
      if(d.name)document.getElementById('agf-name').value=d.name;
      if(d.description)document.getElementById('agf-desc').value=d.description;
      if(d.prompt)document.getElementById('agf-prompt').value=d.prompt;
    }).catch(function(){btn.disabled=false;btn.textContent='✨ Generate';});
}
function muAgentSave(e){e.preventDefault();
  var b=new URLSearchParams();b.append('action','save');
  b.append('id',document.getElementById('agf-id').value);
  b.append('fork',document.getElementById('agf-fork').value);
  b.append('name',document.getElementById('agf-name').value);
  b.append('description',document.getElementById('agf-desc').value);
  b.append('prompt',document.getElementById('agf-prompt').value);
  document.querySelectorAll('#agf-tools input:checked').forEach(function(el){b.append('tools',el.value);});
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muAgentCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(a){document.getElementById('agents-form').hidden=true;muAgentsLoad(function(){if(a&&a.id)muAgentPick(a.id,a.name);});}).catch(function(){});
  return false;}
function muAgentDelete(id,ev){ev.stopPropagation();if(!confirm('Delete this agent?'))return;
  var b=new URLSearchParams();b.append('action','delete');b.append('id',id);
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muAgentCsrf()},body:b.toString()})
    .then(function(){if(window.muActiveAgent===id)muAgentPick('','Micro');muAgentsLoad();}).catch(function(){});}
function muAgentEsc(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
function muAgentsLoad(cb){
  fetch('/agent/agents',{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    window._muTools=d.tools||window._muTools||[];
    var list=document.getElementById('agents-list');if(!list)return;
    var h='<div class="'+(window.muActiveAgent?'':'on')+'" data-id="" onclick="muAgentPick(\'\',\'Micro\')">Micro <span class="agents-def">default</span></div>';
    (d.agents||[]).forEach(function(a){
      var aj=muAgentEsc(JSON.stringify(a));
      h+='<div class="'+(window.muActiveAgent===a.id?'on':'')+'" data-id="'+a.id+'" onclick="muAgentPick(\''+a.id+'\',\''+muAgentEsc(a.name)+'\')" title="'+muAgentEsc(a.description||'')+'"><span>'+muAgentEsc(a.name)+'</span><span class="agents-actions"><button type="button" title="Fork" onclick=\'event.stopPropagation();muAgentForm(Object.assign(JSON.parse(this.closest("[data-id]").getAttribute("data-a")),{_fork:true}))\'>⑂</button><button type="button" title="Edit" onclick=\'event.stopPropagation();muAgentForm(JSON.parse(this.closest("[data-id]").getAttribute("data-a")))\'>✎</button><button type="button" title="Delete" onclick="muAgentDelete(\''+a.id+'\',event)">✕</button></span></div>';
      list.setAttribute&&0;
    });
    list.innerHTML=h;
    (d.agents||[]).forEach(function(a){var el=list.querySelector('[data-id="'+a.id+'"]');if(el)el.setAttribute('data-a',JSON.stringify(a));});
    if(cb)cb();
  }).catch(function(){});
}
muAgentsLoad();
</script>`
}
