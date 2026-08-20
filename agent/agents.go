package agent

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/agent/micro"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/mail"
)

// AgentsHandler is the JSON API behind the chat's agent picker, at /agents/data.
//
// It was /agent/agents, which nested agents under agent and read as two
// different words that are the same word. /agents is the page where agents are
// created and scoped; this is the data behind the picker on the chat, so it
// belongs under that path rather than under the thing doing the talking.
//
//	GET  → { agents: [user agents], builtins: [{id,name,description}] }
//	POST action=save   (name, prompt, description, id?, fork?) → saved agent
//
// ?fork= copies one of *your own* agents into a new one. It is the last thing
// called fork here: the Fork that copied somebody else's published agent went
// with the directory, and the ⑂ on the rail went with it — offering to copy an
// agent is noise on a page where most people have not made one yet. The query
// parameter keeps its name because links to it exist.
//
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
			_ = RemoveAgent(acc.ID, r.FormValue("id"))
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
			// Into the roster, the one store — and this is now the only place
			// an agent is made.
			desc := strings.TrimSpace(r.FormValue("description"))
			var saved *Agent
			var secret string
			var err error
			if id := r.FormValue("id"); id != "" && For(acc.ID, id) != nil {
				saved, err = UpdateAgent(acc.ID, id, name, prompt, desc, r.Form["tools"])
			} else {
				// No token at creation. An agent is something you talk to; a
				// token is what you additionally hand to a program outside, and
				// the Connect page is where you ask for one.
				saved, secret, err = CreateAgent(acc.ID, name, Hosted, prompt, desc, r.Form["tools"], false)
			}
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			out := map[string]any{"id": saved.ID, "name": saved.Name, "kind": saved.Kind}
			// The secret goes back exactly once, and only far enough to reach
			// the page that shows it. It is stored hashed and cannot be read
			// again — the same one-shot handoff /agents already did.
			if secret != "" {
				out["secret"] = secret
			}
			_ = json.NewEncoder(w).Encode(out)
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

		// Where it runs. The management list wants every agent; the chat
		// picker wants only the ones that can actually answer here, and could
		// not tell them apart without this.
		Kind string `json:"kind,omitempty"`

		// Where to write to this one. Picking an agent changed which name
		// answered and nothing else on the screen — the address underneath went
		// on naming the default, so the page said "answering as Test" directly
		// above "write to it at agent@". The first question anybody has about an
		// agent is where to reach it, and it was the one fact the picker could
		// not change.
		Address string `json:"address,omitempty"`
	}
	// One list, one store. This used to read agent/micro's own store while
	// /agents wrote to the roster, so "my agents" depended on which page you
	// asked — and an agent made here had no scope and no token.
	var mine []lite
	for _, a := range Agents(acc.ID) {
		m := a.AsMicro()
		// AsMicro falls back to the system prompt when there is no description,
		// which is right for the router — it needs a sentence to route on — and
		// wrong here, where this becomes a tooltip and a subtitle. A nine-line
		// prompt rendered as an agent's one-line description is not a
		// description, it is the whole agent spilled onto the list.
		mine = append(mine, lite{m.ID, m.Name, firstLine(a.Description, m.Description),
			m.SystemPrompt, m.Tools, a.Kind, a.Address()})
	}
	// The default's address too, so the picker can put it back when the reader
	// returns to Micro. Without it the only way back to the shared address is a
	// page reload.
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agents":  mine,
		"tools":   AllAgentTools(),
		"address": mail.SharedAgentAddress(),
	})
}

// firstLine returns the stored description if there is one, otherwise the
// opening line of the fallback, trimmed to something that fits on a row.
func firstLine(stored, fallback string) string {
	if s := strings.TrimSpace(stored); s != "" {
		return s
	}
	s := strings.TrimSpace(fallback)
	if i := strings.IndexAny(s, ".\n"); i > 0 {
		s = s[:i+1]
	}
	if len(s) > 140 {
		s = strings.TrimSpace(s[:140]) + "…"
	}
	return s
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
	// The tool list comes from the registry, so a generated agent can only be
	// scoped to services that exist. Naming them in the prompt by hand is how
	// you get an agent told to "lean on social" on an instance where social is
	// not registered — which then reports that source unavailable on every
	// answer.
	available := strings.Join(AllAgentTools(), ", ")
	sys := `You design AI agent personas for Mu, a personal assistant whose tools are grouped by service. The services available on this instance are: ` + available + `.

Given a brief, output ONLY minified JSON with exactly these keys:
"name": a short label, <=40 chars, no emoji;
"description": one line, <=120 chars, saying what it does for the user — not a list of the data it reads;
"prompt": a system prompt of 2-4 sentences in second person ("You are ...") defining the persona, tone and priorities. Say what to do with the data, not which tools to call: the tools are chosen for it and naming them adds nothing. Never instruct it to "lean on" a source;
"tools": an array of service names taken ONLY from the list above — the smallest set the brief actually needs. Omit anything it would not use. An empty array means every tool, so avoid it unless the brief is genuinely general.
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
	var raw struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Prompt      string   `json:"prompt"`
		Tools       []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil || strings.TrimSpace(raw.Prompt) == "" {
		// Model didn't return clean JSON — fall back to using the text as the prompt.
		return map[string]string{"name": "", "description": "", "prompt": out}, nil
	}
	// validServices drops anything the model invented, so a hallucinated
	// service name cannot end up as a scope that matches nothing.
	return map[string]string{
		"name":        raw.Name,
		"description": raw.Description,
		"prompt":      raw.Prompt,
		"tools":       strings.Join(validServices(raw.Tools), ","),
	}, nil
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
.agents-new{color:var(--btn-primary,#000);text-decoration:none;font-size:13px;font-weight:600}
.agents-list{padding:6px}
.agents-list>div{display:flex;justify-content:space-between;align-items:center;gap:6px;padding:7px 8px;border-radius:6px;cursor:pointer;font-size:13px;color:#333}
.agents-list>div:hover{background:#f4f4f5}
.agents-list>div.on{background:var(--hover-background,#f5f5f5);color:var(--text-primary,#111);font-weight:600}
.agents-def{color:#aaa;font-size:11px;font-weight:400}
.agents-actions{display:flex;gap:2px;opacity:.55}
.agents-actions a,.agents-actions button{border:0;background:none;cursor:pointer;font-size:12px;padding:0 2px;color:inherit;text-decoration:none}
</style>
<script>
var MUAKEY='mu_active_agent';
// Restore the in-tab agent selection so a reload keeps answering as the same
// agent. A reopened session or ?agent= link overrides this via muSeedAgent.
window.muActiveAgent=window.muActiveAgent||(function(){try{return sessionStorage.getItem(MUAKEY)||'';}catch(e){return '';}})();
function muAgentCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
// Resolve an agent id to its display name from the loaded list ('' = default).
function muAgentName(id){if(!id)return 'Micro';var d=document.querySelector('#agents-list>div[data-id="'+id+'"]');if(d){var s=d.querySelector('span');if(s&&s.textContent)return s.textContent;}return 'Micro';}
function muAgentChip(){var c=document.getElementById('active-agent-chip');if(c)c.textContent='Agent: '+muAgentName(window.muActiveAgent);}
// Picking an agent is a navigation, not a highlight.
//
// It used to set a variable, move the highlight and update the chip, and stop
// there. Everything else on the page — the conversation in the middle and the
// rail of past conversations beside it — is rendered by the server for one
// agent, so switching left you looking at the previous agent's history with a
// chip claiming you were talking to the new one. Worse for a brand-new agent:
// the rail should be empty and instead showed somebody else's conversations,
// so an agent that had never been used looked well used.
//
// The id goes in the URL, which is the same thing clicking an agent on /agents
// does. One door, one behaviour, and a reload keeps the agent.
//
// It stays where you are. The picker is beside the conversation and beside the
// page saying how to reach an agent, and sending every pick back to the
// conversation meant switching agents while reading how to connect one threw
// away what you were doing.
function muAgentPick(id){
  var p=window.location.pathname,to;
  if(p.indexOf('/agent/connect')===0){to=id?'/agent/connect?id='+encodeURIComponent(id):'/agent/connect';}
  else{to=id?'/agent?id='+encodeURIComponent(id):'/agent';}
  if(window.location.pathname+window.location.search===to){return;}
  window.location=to;
}
// Every agent is one you can talk to, so clicking one picks it. The second
// argument is a leftover from when an agent could be declared external and had
// no conversation here; callers pass false.
function muAgentOpen(id,external){
  muAgentPick(id);
}
// Set the active agent from the server (session reopen / deep link) and persist
// it; the list highlight + chip refresh once the agents finish loading.
window.muSeedAgent=function(id){window.muActiveAgent=id||'';try{sessionStorage.setItem(MUAKEY,window.muActiveAgent);}catch(e){}document.querySelectorAll('#agents-list>div').forEach(function(d){d.classList.toggle('on',d.getAttribute('data-id')===window.muActiveAgent);});muAgentChip();};
function muAgentEsc(s){return (s||'').replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
function muAgentDelete(id,ev){ev.stopPropagation();ev.preventDefault();if(!confirm('Delete this agent?'))return;
  var b=new URLSearchParams();b.append('action','delete');b.append('id',id);
  fetch('/agents/data',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':muAgentCsrf()},body:b.toString()})
    .then(function(){if(window.muActiveAgent===id)muAgentPick('');muAgentsLoad();}).catch(function(){});}
function muAgentsLoad(){
  fetch('/agents/data',{headers:{'Accept':'application/json'}}).then(function(r){return r.json();}).then(function(d){
    var list=document.getElementById('agents-list');if(!list)return;
    var h='<div class="'+(window.muActiveAgent?'':'on')+'" data-id="" onclick="muAgentPick(\'\')">Micro <span class="agents-def">default</span></div>';
    (d.agents||[]).forEach(function(a){var id=muAgentEsc(a.id);
      h+='<div class="'+(window.muActiveAgent===a.id?'on':'')+'" data-id="'+id+'" onclick="muAgentOpen(\''+id+'\',false)" title="'+muAgentEsc(a.description||'')+'"><span>'+muAgentEsc(a.name)+'</span><span class="agents-actions"><a title="Edit" href="/agent/new?id='+id+'" onclick="event.stopPropagation()">✎</a><button type="button" title="Delete" onclick="muAgentDelete(\''+id+'\',event)">✕</button></span></div>';
    });
    list.innerHTML=h;
    muAgentChip();
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
		if a := For(acc.ID, id); a != nil {
			cur, editID = a.AsMicro(), id
		}
	} else if fid := r.URL.Query().Get("fork"); fid != "" {
		if a := For(acc.ID, fid); a != nil {
			cur, forkFrom = a.AsMicro(), fid
		}
	}

	// At the cap, the builder is a form you cannot submit — so it is not shown.
	//
	// The check lived only where an agent is made, so the way to discover the
	// limit was to fill the whole thing in and have an alert say no. Editing is
	// exempt because editing adds nothing; copying is not, because a copy is a
	// new agent with a head start.
	if editID == "" {
		if full, have, max := AtAgentLimit(acc.ID); full {
			app.Respond(w, r, app.Response{
				Title:       "Agent limit",
				Description: "Build a custom agent",
				HTML: fmt.Sprintf(`<div class="card" style="max-width:600px">`+
					`<h3 class="m-0 mb-2">You are running %d of %d agent%s</h3>`+
					`<p style="color:#666;font-size:15px;margin:0 0 16px">Your plan runs %d. `+
					`Change your plan to add more, or delete one you are not using.</p>`+
					`<a href="/account/topup" class="btn">Add credit</a> `+
					`<a href="/agents" class="btn" style="background:#fff;color:#111;border:1px solid #ddd">Your agents</a>`+
					`</div>`, have, max, plural(max), max),
			})
			return
		}
	}

	name, desc, prompt := "", "", ""
	var selTools []string
	title := "New agent"
	if cur != nil {
		name, desc, prompt, selTools = cur.Name, cur.Description, cur.SystemPrompt, cur.Tools
		if forkFrom != "" {
			name = "Copy of " + name
			title = "Copy agent"
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
		toolsHTML.WriteString(`<label class="chip"><input type="checkbox" name="tool" value="` + t + `"` + chk +
			`><span>` + html.EscapeString(ToolLabel(t)) + `</span></label>`)
	}

	// Nothing here asks where it runs.
	//
	// It used to: Here, or Elsewhere — Claude, Cursor or your own program,
	// calling in with its token. The question never belonged on this form,
	// which is the form where you write the standing instruction: an agent
	// running in Cursor does not use the prompt you type here, so half the page
	// asked you to configure something the other half had declared irrelevant.
	//
	// Every agent runs here and answers at POST /agent/<name>. Pointing an
	// outside program at it is a token — something you ask for on the Connect
	// page when you want one, not a second species of agent chosen before you
	// have written a word.

	// What it has actually done. An agent is a scope — a name, a system prompt
	// and the tools it may reach — and the page where you set that scope said
	// nothing about whether any of it worked. Its runs are the only evidence
	// the scope is right, and they were a page away with no filter to get here.
	runsHTML := ""
	if editID != "" {
		runsHTML = agentRunsSummary(acc.ID, editID)
	}

	b := `<div class="builder">
  <p class="builder-sub">Describe an agent and Mu will draft it, or write the system prompt yourself. Pick what it may reach — it will be refused everything else, even though it is your account behind it.</p>
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
    <label class="b-label">What may it reach? <span class="b-hint">— none selected means everything you can</span></label>
    <div class="b-tools">` + toolsHTML.String() + `</div>
    ` + runsHTML + `
    <div class="b-actions">
      <button type="submit" class="b-save">Save agent</button>
      <a class="b-cancel" href="/agents">Cancel</a>
    </div>
  </form>
</div>
<style>
.builder{max-width:720px}
.builder-sub{color:#666;margin:0 0 18px}
.b-label{display:block;font-size:13px;font-weight:600;color:#374151;margin:14px 0 6px}
.b-state{font-size:13px;color:#666;line-height:1.5;margin:0 0 4px}
.b-state strong{color:var(--text-primary,#111)}
.b-hint{font-weight:400;color:#9ca3af}
#bform input,#bform textarea{width:100%;box-sizing:border-box;padding:9px 11px;font-size:14px;border:1px solid #d1d5db;border-radius:6px;font-family:inherit}
#bform textarea{line-height:1.5;resize:vertical}
.b-gen{display:flex;gap:8px}
.b-gen input{flex:1}
.b-gen button{white-space:nowrap;padding:9px 14px;font-size:14px;border:1px solid var(--border-color,#ddd);border-radius:var(--border-radius,6px);cursor:pointer;background:#fff;color:var(--text-primary,#111);font-weight:500}
.b-gen button[disabled]{opacity:.6;cursor:default}
.b-tools{display:flex;flex-wrap:wrap;gap:6px;margin-top:2px}
.b-actions{display:flex;align-items:center;gap:12px;margin-top:22px}
.b-save{padding:10px 22px;font-size:14px;font-weight:600;border:0;border-radius:var(--border-radius,6px);background:var(--btn-primary,#000);color:#fff;cursor:pointer}
.b-save:hover{background:var(--btn-primary-hover,#333)}
.b-cancel{color:#6b7280;text-decoration:none;font-size:14px}
.b-cancel:hover{color:#111}
</style>` + chipCSS + `
<script>
function bCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function bGen(){var brief=document.getElementById('b-brief').value.trim();if(!brief)return;
  var btn=document.getElementById('b-genbtn');btn.disabled=true;btn.textContent='Generating…';
  var b=new URLSearchParams();b.append('action','generate');b.append('brief',brief);
  fetch('/agents/data',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':bCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(d){btn.disabled=false;btn.textContent='✨ Generate';
      if(d.error){alert(d.error);return;}
      if(d.name)document.getElementById('b-name').value=d.name;
      if(d.description)document.getElementById('b-desc').value=d.description;
      if(d.prompt)document.getElementById('b-prompt').value=d.prompt;
      // Tick the scope it chose. Picking the tools by hand was the one step the
      // generator left to you, and it is the step you have least information
      // for: you have not read the prompt it just wrote.
      if(typeof d.tools==='string'){
        var want={};d.tools.split(',').forEach(function(t){if(t)want[t]=true;});
        document.querySelectorAll('.b-tools input[name="tool"]').forEach(function(c){c.checked=!!want[c.value];});
      }
    }).catch(function(){btn.disabled=false;btn.textContent='✨ Generate';});}
function bSave(e){e.preventDefault();
  var b=new URLSearchParams();b.append('action','save');
  b.append('id',document.getElementById('b-id').value);
  b.append('fork',document.getElementById('b-fork').value);
  b.append('name',document.getElementById('b-name').value);
  b.append('description',document.getElementById('b-desc').value);
  b.append('prompt',document.getElementById('b-prompt').value);
  document.querySelectorAll('.b-tools input:checked').forEach(function(el){b.append('tools',el.value);});
  fetch('/agents/data',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':bCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(a){if(a.error){alert(a.error);return;}
      if(!a.id){location.href='/agents';return;}
      // Creation mints no token now, so there is normally nothing to copy and
      // the next screen is the agent itself. The branch stays because /agents
      // is still where a secret is shown once, if one ever comes back.
      if(a.secret){location.href='/agents?created='+encodeURIComponent(a.id)+'&secret='+encodeURIComponent(a.secret);return;}
      location.href='/agent?id='+encodeURIComponent(a.id);}).catch(function(){});
  return false;}
</script>`

	app.Respond(w, r, app.Response{Title: title, Description: "Build a custom agent", HTML: b})
}
