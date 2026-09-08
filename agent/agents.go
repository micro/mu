package agent

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"sort"
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
		if action := r.FormValue("action"); action != "" && action != "save" && action != "delete" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "Unknown action"})
			return
		}
		switch r.FormValue("action") {
		case "delete":
			_ = RemoveAgent(acc.ID, r.FormValue("id"))
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		case "save", "":
			name := strings.TrimSpace(r.FormValue("name"))
			// The body only. This is the agent's system prompt — what it is for,
			// in the owner's words — and FormValue would take it from ?prompt=
			// just as happily, writing it into the reverse proxy's log.
			prompt := strings.TrimSpace(r.PostFormValue("prompt"))
			if name == "" || prompt == "" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "name and prompt are required"})
				return
			}
			// Into the roster, the one store — and this is now the only place
			// an agent is made.

			mode := r.FormValue("scope_mode")
			selected := r.Form["tools"]
			if mode == "all" {
				selected = nil
			}
			if (mode != "" && mode != "all" && mode != "select") || (mode == "select" && len(validServices(selected)) == 0) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "Select at least one valid service"})
				return
			}
			desc := strings.TrimSpace(r.FormValue("description"))
			var saved *Agent
			var secret string
			var err error
			if id := r.FormValue("id"); id != "" && For(acc.ID, id) != nil {
				saved, err = UpdateAgent(acc.ID, id, name, prompt, desc, selected)
			} else {
				// No token at creation. An agent is something you talk to; a
				// token is what you additionally hand to a program outside, and
				// the Connect page is where you ask for one.
				saved, secret, err = CreateAgent(acc.ID, name, Hosted, prompt, desc, selected, false)
			}
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
				return
			}
			// Which model it answers with, if the form offered a choice. A
			// second write rather than a seventh argument — see SetModel.
			//
			// Reported rather than swallowed: an agent saved with the model
			// quietly dropped is the failure this whole field exists to stop,
			// one level up. The agent is saved either way, which is why this
			// says what did not take rather than claiming nothing did.
			if err := SetModel(acc.ID, saved.ID, r.FormValue("model")); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": "Saved, but the model was not: " + err.Error(),
					"id":    saved.ID,
				})
				return
			}
			out := map[string]any{"id": saved.ID, "name": saved.Name, "kind": saved.Kind}
			// The secret goes back exactly once, and only far enough to reach
			// the page that shows it. It is stored hashed and cannot be read
			// again — the same one-shot handoff /agents already did.
			//
			// In this response body, and also stashed for the page: the browser
			// used to take it from here and put it in location.href, which is
			// how a bearer token ended up in the URL bar and in the reverse
			// proxy's log. The page collects it now. A caller that is not the
			// builder still gets it here, which is a body and the right place
			// for it. See secret.go.
			if secret != "" {
				out["secret"] = secret
				stashSecret(acc.ID, saved.ID, secret)
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

		// Which model it answers with, so the edit form can show what is set
		// rather than resetting it to the default every time it opens.
		Model string `json:"model,omitempty"`
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
			m.SystemPrompt, m.Tools, a.Kind, a.Address(), a.Model})
	}
	// The default's address too, so the picker can put it back when the reader
	// returns to Micro. Without it the only way back to the shared address is a
	// page reload.
	// And what this instance can actually run, so the form offers models
	// rather than asking somebody to type an id. Derived from which providers
	// have keys — see ai.Choices — so an instance with one key offers one
	// choice and an instance with none offers no menu at all, which is the
	// honest answer rather than a select nobody can satisfy.
	type model struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	models := []model{}
	for _, c := range ai.Choices() {
		models = append(models, model{c.ID, c.Label})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agents":  mine,
		"tools":   AllAgentTools(),
		"address": mail.SharedAgentAddress(),
		"models":  models,
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

// renderAgentsPanel renders the lean "Agents" card for the rail: pick the
// default or one of your agents. Creating/editing happens on /agent/new.
func renderAgentsPanel() string {
	return `<div class="agents-panel">
  <div class="agents-head"><span>Agents</span><a class="agents-new" href="/agent/new">New</a></div>
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
window.muActiveAgent='';
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
// Set the page agent from the server (session reopen / deep link); the list highlight + chip refresh once the agents finish loading.
window.muSeedAgent=function(id){window.muActiveAgent=id||'';document.querySelectorAll('#agents-list>div').forEach(function(d){d.classList.toggle('on',d.getAttribute('data-id')===window.muActiveAgent);});muAgentChip();};
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
				HTML: fmt.Sprintf(`<div class="card col-narrow">`+
					`<h3 class="m-0 mb-2">You are running %d of %d agent%s</h3>`+
					`<p class="text-secondary lead-15 m-0 mb-4">Your plan runs %d. `+
					`Change your plan to add more, or delete one you are not using.</p>`+
					`<a href="/account/topup" class="btn">Top up</a> `+
					`<a href="/agents" class="btn btn-plain">Your agents</a>`+
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
	var scopeChoices []app.Option
	for _, t := range AllAgentTools() {
		scopeChoices = append(scopeChoices, app.Option{Value: t, Label: ToolLabel(t), On: selected[t]})
	}

	// Which model, when there is more than one to pick from.
	//
	// Absent entirely on an instance with one provider: a select with a single
	// option is a control that cannot be used, and it would sit there implying
	// this instance can do something it cannot. Two providers is where the
	// question becomes real, which is also the moment somebody self-hosting
	// wires a second key.
	//
	// Options are derived, never listed — see ai.Choices. Model ids change
	// often enough that a hand-written menu is wrong within a month, and the
	// place it is written down is not the place anybody updates.
	modelHTML := ""
	if choices := ai.Choices(); len(choices) > 1 {
		var opts strings.Builder
		cur := ""
		if a := For(acc.ID, editID); a != nil {
			cur = a.Model
		}
		// Alphabetical, because a menu is scanned rather than read. The order
		// ai.Choices returns is best-of-each-provider-first, which is a
		// judgement about quality and tells somebody looking for Sonnet
		// nothing about where to find it.
		sort.Slice(choices, func(i, j int) bool { return choices[i].Label < choices[j].Label })

		// The default, named. It said "Instance default", which is a menu entry
		// that does not say what it selects — on the one screen whose whole
		// point is knowing which model is running.
		opts.WriteString(`<option value="">` + html.EscapeString(ai.DefaultLabel()) + `</option>`)
		for _, c := range choices {
			sel := ""
			if strings.EqualFold(c.ID, cur) {
				sel = " selected"
			}
			opts.WriteString(`<option value="` + html.EscapeString(c.ID) + `"` + sel + `>` +
				html.EscapeString(c.Label) + `</option>`)
		}
		modelHTML = `<label class="field-label">Model<select class="field field-wide" id="b-model">` + opts.String() + `</select></label>`

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

	// No run list here.
	//
	// The builder showed the last three workflow records for this agent, which
	// was the last thing left of /agent/runs after the page went. It answered
	// "is this scope right" with a list of prompts, which is the question but
	// not an answer to it — and what an answer actually called is now beside
	// the answer in the conversation, where somebody looking at an odd one is.
	b := `<div class="page-col">
 <form id="bform" class="form" onsubmit="return bSave(event)">
 <input type="hidden" id="b-id" value="` + html.EscapeString(editID) + `">
 <input type="hidden" id="b-fork" value="` + html.EscapeString(forkFrom) + `">` +
		app.Field{ID: "b-name", Label: "Name", Max: 60, Required: true, Wide: true, Value: name}.HTML() +
		app.Field{ID: "b-desc", Label: "Description", Max: 140, Wide: true, Value: desc}.HTML() +
		app.Field{ID: "b-prompt", Label: "System prompt", Rows: 9, Required: true, Wide: true, Value: prompt}.HTML() +
		app.ServiceSelect("b-scope", "b-service-list", "tool", scopeChoices) + modelHTML + `
 <div class="form-actions"><button type="submit" class="btn">Save agent</button><a href="/agents">Cancel</a></div>
 </form></div>

<script>
function bCsrf(){var m=document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);return m?decodeURIComponent(m[1]):'';}
function bSave(e){e.preventDefault();
  var b=new URLSearchParams();b.append('action','save');
  b.append('id',document.getElementById('b-id').value);
  b.append('fork',document.getElementById('b-fork').value);
  b.append('name',document.getElementById('b-name').value);
  b.append('description',document.getElementById('b-desc').value);
  b.append('prompt',document.getElementById('b-prompt').value);
  var mode=document.getElementById('b-scope').value;
  var selected=Array.from(document.querySelectorAll('#b-service-list input:checked'));
  if(mode==='select'&&!selected.length){alert('Select at least one service');return false;}
  b.append('scope_mode',mode);
  if(mode==='select')selected.forEach(function(el){b.append('tools',el.value);});
  // Only when the select is on the page. One provider means no menu, and
  // sending an empty model then would be sending a choice nobody made — which
  // is the same value it already has, but says so on every save.
  var bm=document.getElementById('b-model');if(bm)b.append('model',bm.value);
  fetch('/agents/data',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':bCsrf()},body:b.toString()})
    .then(function(r){return r.json();}).then(function(a){if(a.error){alert(a.error);return;}
      if(!a.id){location.href='/agents';return;}
      // Creation mints no token now, so there is normally nothing to copy and
      // the next screen is the agent itself. The branch stays because /agents
      // is still where a secret is shown once, if one ever comes back.
      // The id, never the secret. The server holds the token for the one render
      // that shows it — putting it here would write a bearer credential into the
      // URL bar, the history, and the proxy log. See agent/secret.go.
      if(a.secret){location.href='/agents?created='+encodeURIComponent(a.id);return;}
      location.href='/agent?id='+encodeURIComponent(a.id);}).catch(function(){});
  return false;}
</script>`

	app.Respond(w, r, app.Response{Title: title, Description: "Build a custom agent", HTML: b})
}
