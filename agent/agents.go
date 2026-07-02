package agent

import (
	"encoding/json"
	"html"
	"net/http"
	"strings"

	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/app"
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

// renderAgentsPanel renders the lean "Agents" card for the rail: pick the
// default or one of your agents. Creating/editing happens on /agent/new.
func renderAgentsPanel() string {
	return `<div class="agents-panel">
  <div class="agents-head"><span>Agents</span><a class="agents-new" href="/agent/new">+ New</a></div>
  <div class="agents-list" id="agents-list"><div class="on" data-id="" onclick="muAgentPick('')">Micro <span class="agents-def">default</span></div></div>
</div>
<style>
.agents-panel{border:1px solid var(--card-border,#e8e8e8);border-radius:8px;margin-bottom:12px;background:var(--card-background,#fff);overflow:hidden}
.agents-head{display:flex;justify-content:space-between;align-items:center;padding:10px 12px;font-size:14px;font-weight:600;border-bottom:1px solid var(--card-border,#eee)}
.agents-new{color:#4f46e5;text-decoration:none;font-size:13px;font-weight:600}
.agents-list{padding:6px}
.agents-list>div{display:flex;justify-content:space-between;align-items:center;gap:6px;padding:7px 8px;border-radius:6px;cursor:pointer;font-size:13px;color:#333}
.agents-list>div:hover{background:#f4f4f5}
.agents-list>div.on{background:#eef2ff;color:#3730a3;font-weight:600}
.agents-def{color:#aaa;font-size:11px;font-weight:400}
.agents-actions{display:flex;gap:2px;opacity:.55}
.agents-actions a,.agents-actions button{border:0;background:none;cursor:pointer;font-size:12px;padding:0 2px;color:inherit;text-decoration:none}
</style>
<script>
window.muActiveAgent=window.muActiveAgent||'';
function muAgentCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function muAgentPick(id){window.muActiveAgent=id;document.querySelectorAll('#agents-list>div').forEach(function(d){d.classList.toggle('on',d.getAttribute('data-id')===id);});}
function muAgentEsc(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
function muAgentDelete(id,ev){ev.stopPropagation();ev.preventDefault();if(!confirm('Delete this agent?'))return;
  var b=new URLSearchParams();b.append('action','delete');b.append('id',id);
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muAgentCsrf()},body:b.toString()})
    .then(function(){if(window.muActiveAgent===id)muAgentPick('');muAgentsLoad();}).catch(function(){});}
function muAgentsLoad(){
  fetch('/agent/agents',{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    var list=document.getElementById('agents-list');if(!list)return;
    var h='<div class="'+(window.muActiveAgent?'':'on')+'" data-id="" onclick="muAgentPick(\'\')">Micro <span class="agents-def">default</span></div>';
    (d.agents||[]).forEach(function(a){var id=muAgentEsc(a.id);
      h+='<div class="'+(window.muActiveAgent===a.id?'on':'')+'" data-id="'+id+'" onclick="muAgentPick(\''+id+'\')" title="'+muAgentEsc(a.description||'')+'"><span>'+muAgentEsc(a.name)+'</span><span class="agents-actions"><a title="Edit" href="/agent/new?id='+id+'" onclick="event.stopPropagation()">✎</a><a title="Fork" href="/agent/new?fork='+id+'" onclick="event.stopPropagation()">⑂</a><button type="button" title="Delete" onclick="muAgentDelete(\''+id+'\',event)">✕</button></span></div>';
    });
    list.innerHTML=h;
  }).catch(function(){});
}
muAgentsLoad();
</script>`
}

// NewAgentHandler renders the full-page agent builder at /agent/new, separate
// from the chat. It handles new agents, ?id= (edit) and ?fork= (copy).
func NewAgentHandler(w http.ResponseWriter, r *http.Request) {
	_, acc := auth.TrySession(r)
	if acc == nil {
		http.Redirect(w, r, "/login?next=/agent/new", http.StatusSeeOther)
		return
	}

	var cur *micro.Agent
	editID := ""
	forkFrom := ""
	if id := r.URL.Query().Get("id"); id != "" {
		if a := micro.GetUserAgentFor(acc.ID, id); a != nil {
			cur, editID = a, id
		}
	} else if fid := r.URL.Query().Get("fork"); fid != "" {
		if a := micro.GetUserAgentFor(acc.ID, fid); a != nil {
			cur, forkFrom = a, fid
		}
	}

	name, desc, prompt := "", "", ""
	var selTools []string
	title := "New agent"
	if cur != nil {
		name, desc, prompt, selTools = cur.Name, cur.Description, cur.SystemPrompt, cur.Tools
		if forkFrom != "" {
			name = "Copy of " + name
			title = "Fork agent"
		} else {
			title = "Edit agent"
		}
	}

	selected := map[string]bool{}
	for _, t := range selTools {
		selected[t] = true
	}
	var toolsHTML strings.Builder
	for _, t := range AllAgentTools() {
		chk := ""
		if selected[t] {
			chk = " checked"
		}
		toolsHTML.WriteString(`<label><input type="checkbox" name="tool" value="` + t + `"` + chk + `> ` + t + `</label>`)
	}

	b := `<div class="builder">
  <p class="builder-sub">Describe an agent and Mu will draft it, or write the system prompt yourself. Pick which tools it may use.</p>
  <form id="bform" onsubmit="return bSave(event)">
    <input type="hidden" id="b-id" value="` + html.EscapeString(editID) + `">
    <input type="hidden" id="b-fork" value="` + html.EscapeString(forkFrom) + `">
    <label class="b-label">Describe it (optional)</label>
    <div class="b-gen">
      <input id="b-brief" placeholder="e.g. a meticulous crypto research analyst that always cites sources">
      <button type="button" id="b-genbtn" onclick="bGen()">✨ Generate</button>
    </div>
    <label class="b-label">Name</label>
    <input id="b-name" maxlength="60" required value="` + html.EscapeString(name) + `">
    <label class="b-label">Description</label>
    <input id="b-desc" maxlength="140" value="` + html.EscapeString(desc) + `">
    <label class="b-label">System prompt</label>
    <textarea id="b-prompt" rows="9" required>` + html.EscapeString(prompt) + `</textarea>
    <label class="b-label">Tools <span class="b-hint">— none selected means all tools</span></label>
    <div class="b-tools">` + toolsHTML.String() + `</div>
    <div class="b-actions">
      <button type="submit" class="b-save">Save agent</button>
      <a class="b-cancel" href="/agent">Cancel</a>
    </div>
  </form>
</div>
<style>
.builder{max-width:720px}
.builder-sub{color:#666;margin:0 0 18px}
.b-label{display:block;font-size:13px;font-weight:600;color:#374151;margin:14px 0 6px}
.b-hint{font-weight:400;color:#9ca3af}
#bform input,#bform textarea{width:100%;box-sizing:border-box;padding:9px 11px;font-size:14px;border:1px solid #d1d5db;border-radius:6px;font-family:inherit}
#bform textarea{line-height:1.5;resize:vertical}
.b-gen{display:flex;gap:8px}
.b-gen input{flex:1}
.b-gen button{white-space:nowrap;padding:9px 14px;font-size:14px;border:1px solid #c7d2fe;border-radius:6px;cursor:pointer;background:#eef2ff;color:#4338ca;font-weight:500}
.b-gen button[disabled]{opacity:.6;cursor:default}
.b-tools{display:flex;flex-wrap:wrap;gap:8px 16px;margin-top:2px}
.b-tools label{display:flex;align-items:center;gap:6px;font-size:13px;color:#374151;cursor:pointer}
.b-actions{display:flex;align-items:center;gap:12px;margin-top:22px}
.b-save{padding:10px 22px;font-size:14px;font-weight:600;border:0;border-radius:6px;background:#4f46e5;color:#fff;cursor:pointer}
.b-save:hover{background:#4338ca}
.b-cancel{color:#6b7280;text-decoration:none;font-size:14px}
.b-cancel:hover{color:#111}
</style>
<script>
function bCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function bGen(){var brief=document.getElementById('b-brief').value.trim();if(!brief)return;
  var btn=document.getElementById('b-genbtn');btn.disabled=true;btn.textContent='Generating…';
  var b=new URLSearchParams();b.append('action','generate');b.append('brief',brief);
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':bCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(d){btn.disabled=false;btn.textContent='✨ Generate';
      if(d.error){alert(d.error);return;}
      if(d.name)document.getElementById('b-name').value=d.name;
      if(d.description)document.getElementById('b-desc').value=d.description;
      if(d.prompt)document.getElementById('b-prompt').value=d.prompt;
    }).catch(function(){btn.disabled=false;btn.textContent='✨ Generate';});}
function bSave(e){e.preventDefault();
  var b=new URLSearchParams();b.append('action','save');
  b.append('id',document.getElementById('b-id').value);
  b.append('fork',document.getElementById('b-fork').value);
  b.append('name',document.getElementById('b-name').value);
  b.append('description',document.getElementById('b-desc').value);
  b.append('prompt',document.getElementById('b-prompt').value);
  document.querySelectorAll('.b-tools input:checked').forEach(function(el){b.append('tools',el.value);});
  fetch('/agent/agents',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':bCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(a){if(a.error){alert(a.error);return;}
      location.href='/agent'+(a.id?('?agent='+encodeURIComponent(a.id)):'');}).catch(function(){});
  return false;}
</script>`

	app.Respond(w, r, app.Response{Title: title, Description: "Build a custom agent", HTML: b})
}
