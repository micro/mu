package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/internal/ai"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
)

// WorkspaceHandler serves the agent workspace page.
func WorkspaceHandler(w http.ResponseWriter, r *http.Request) {
	// SSE stream when prompt is provided
	if r.URL.Query().Get("prompt") != "" {
		handleWorkspaceQuery(w, r)
		return
	}

	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	var sb strings.Builder
	sb.WriteString(`<style>
#ws{display:flex;flex-direction:column;height:calc(100vh - 60px);max-width:900px;margin:0 auto}
#ws-tabs{display:flex;gap:0;border-bottom:1px solid #e0e0e0;margin-bottom:8px}
#ws-tabs button{padding:8px 16px;border:none;background:none;font-family:inherit;font-size:13px;cursor:pointer;color:#888;border-bottom:2px solid transparent}
#ws-tabs button.active{color:#1a1a1a;border-bottom-color:#1a1a1a}
#ws-chat{flex:1;overflow-y:auto;display:flex;flex-direction:column}
#ws-log{flex:1;overflow-y:auto;padding:8px 0}
#ws-app{flex:1;display:none;flex-direction:column}
#ws-preview{width:100%;flex:1;border:none;min-height:50vh;background:#fff}
#ws-save-bar{display:flex;gap:8px;padding:8px 0;align-items:center}
#ws-save-bar input{flex:1;padding:8px 12px;border:1px solid #e0e0e0;border-radius:6px;font-family:inherit;font-size:14px}
#ws-save-bar button{padding:8px 16px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;font-size:13px}
#ws-input{display:flex;gap:8px;padding:8px 0}
#ws-input input{flex:1;padding:10px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:15px;font-family:inherit}
#ws-input button{padding:10px 24px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;font-size:15px}
#ws-input button:disabled{background:#ccc;cursor:not-allowed}
.ws-msg{margin:4px 0;font-size:14px;line-height:1.5}
.ws-user{color:#1a1a1a;font-weight:600}
.ws-agent{color:#555}
.ws-step{color:#888;font-size:13px;padding:2px 0;border-left:2px solid #e0e0e0;padding-left:8px;margin:2px 0}
.ws-error{color:#c00;font-size:13px}
</style>
<div id="ws">
<div id="ws-tabs">
<button class="active" onclick="showTab('chat')">Chat</button>
<button onclick="showTab('app')">Preview</button>
</div>
<div id="ws-chat">
<div id="ws-log"></div>
</div>
<div id="ws-app">
<iframe id="ws-preview"></iframe>
<div id="ws-save-bar" style="display:none">
<input type="text" id="ws-app-name" placeholder="App name">
<button onclick="saveApp()">Save as App</button>
</div>
</div>
<div id="ws-input">
<input type="text" id="ws-prompt" placeholder="Tell the agent what to do..." autofocus>
<button id="ws-btn" onclick="send()">Go</button>
</div>
</div>
<script>
var log=document.getElementById('ws-log');
var preview=document.getElementById('ws-preview');
var input=document.getElementById('ws-prompt');
var btn=document.getElementById('ws-btn');
var chatTab=document.getElementById('ws-chat');
var appTab=document.getElementById('ws-app');
var tabs=document.querySelectorAll('#ws-tabs button');
var sessionId='';
var lastHTML='';

function showTab(name){
  tabs.forEach(function(t){t.className=''});
  if(name==='chat'){chatTab.style.display='flex';appTab.style.display='none';tabs[0].className='active'}
  else{chatTab.style.display='none';appTab.style.display='flex';tabs[1].className='active'}
}

function addMsg(cls,text){
  var d=document.createElement('div');d.className='ws-msg '+cls;d.textContent=text;
  log.appendChild(d);log.scrollTop=log.scrollHeight;
}
function addStep(text){
  var d=document.createElement('div');d.className='ws-step';d.textContent=text;
  log.appendChild(d);log.scrollTop=log.scrollHeight;
}
function addHTML(cls,html){
  var d=document.createElement('div');d.className='ws-msg '+cls;d.innerHTML=html;
  log.appendChild(d);log.scrollTop=log.scrollHeight;
}

input.addEventListener('keydown',function(e){if(e.key==='Enter')send()});

function send(){
  var prompt=input.value.trim();
  if(!prompt)return;
  addMsg('ws-user',prompt);
  input.value='';
  btn.disabled=true;
  addStep('Planning...');

  var es=new EventSource('/agent/workspace?prompt='+encodeURIComponent(prompt)+'&session_id='+sessionId);

  es.onmessage=function(e){
    try{
      var ev=JSON.parse(e.data);

      if(ev.type==='flow_id'){sessionId=ev.flow_id}
      else if(ev.type==='status'){addStep(ev.message)}
      else if(ev.type==='exec'){
        if(ev.html){
          lastHTML=ev.html;
          addStep('Rendering app...');
          showTab('app');
          var doc=preview.contentDocument||preview.contentWindow.document;
          doc.open();doc.write(ev.html);doc.close();
          document.getElementById('ws-save-bar').style.display='flex';
          setTimeout(function(){
            try{
              var errs=preview.contentWindow.mu&&preview.contentWindow.mu.errors;
              if(errs&&errs.length>0){
                addStep('Runtime error: '+errs[0].message);
                feedback(sessionId,false,'',errs[0].message,doc.body?doc.body.textContent.slice(0,500):'');
              } else {
                addStep('App rendered');
                feedback(sessionId,true,'rendered','',doc.body?doc.body.textContent.slice(0,500):'');
              }
            }catch(ex){feedback(sessionId,true,'rendered','','')}
          },1000);
        }
        if(ev.code){
          addStep('Executing code...');
          (async function(){
            try{
              var result=await eval('(async function(){'+ev.code+'})()');
              addStep('Code executed');
              feedback(sessionId,true,String(result||'ok'),'','');
            }catch(err){
              addMsg('ws-error','Error: '+err.message);
              feedback(sessionId,false,'',err.message,'');
            }
          })();
        }
        if(!ev.html&&!ev.code){feedback(sessionId,true,'ok','','')}
      }
      else if(ev.type==='response'){addHTML('ws-agent',ev.html||ev.message||'')}
      else if(ev.type==='error'){addMsg('ws-error',ev.message)}
      else if(ev.type==='done'){btn.disabled=false;es.close()}
    }catch(ex){console.error('parse error',ex)}
  };
  es.onerror=function(){btn.disabled=false;es.close()};
}

function saveApp(){
  if(!lastHTML){addMsg('ws-error','Nothing to save');return}
  var name=document.getElementById('ws-app-name').value.trim()||'Untitled App';
  addStep('Saving app...');
  fetch('/apps',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({name:name,html:lastHTML,public:true})
  }).then(function(r){return r.json()}).then(function(j){
    if(j.slug){
      addHTML('ws-agent','Saved: <a href="/apps/'+j.slug+'/run">'+j.name+'</a>');
    } else {
      addMsg('ws-error','Save failed: '+(j.error||'unknown error'));
    }
  }).catch(function(e){addMsg('ws-error','Save failed: '+e.message)});
}

function feedback(sid,ok,result,error,dom){
  fetch('/agent/exec/result',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({session_id:sid,ok:ok,result:result,error:error,dom:dom})
  }).catch(function(){});
}
</script>`)

	html := app.RenderHTMLForRequest("Agent", "Agent workspace", sb.String(), r)
	w.Write([]byte(html))
}

// handleWorkspaceQuery handles the SSE stream for workspace queries.
func handleWorkspaceQuery(w http.ResponseWriter, r *http.Request) {
	prompt := r.URL.Query().Get("prompt")
	if prompt == "" {
		http.Error(w, "prompt required", 400)
		return
	}

	_, acc, err := auth.RequireSession(r)
	if err != nil {
		http.Error(w, "auth required", 401)
		return
	}

	model := Models[0]
	if QuotaCheck != nil {
		canProceed, _, err := QuotaCheck(r, model.WalletOp)
		if !canProceed {
			msg := "Insufficient credits"
			if err != nil {
				msg = err.Error()
			}
			http.Error(w, msg, 402)
			return
		}
	}

	_ = acc

	// SSE setup
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	// Use a session ID for the feedback channel (not a saved flow)
	sessionID := newFlowID()

	sseSend := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	sseSend(map[string]any{"type": "flow_id", "flow_id": sessionID})
	sseSend(map[string]any{"type": "status", "message": "Processing..."})

	// Step 1: Plan — ask AI what to do
	planSystem := `You are an AI agent on a browser-based app platform.

STEP TYPES:
1. {"type":"tool","name":"TOOL","args":{}} — fetch data server-side
2. {"type":"exec","html":"..."} — render HTML in browser preview
3. {"type":"exec","code":"..."} — run JS in browser (has window.mu SDK)
4. {"type":"respond","message":"markdown text"} — text answer

TOOLS (for fetching data):
` + agentToolsDesc + `

RULES:
- Questions about data → use TOOLS then RESPOND with a summary
- Building apps → use EXEC with complete HTML document
- Simple questions → just RESPOND
- NEVER use exec to fetch data. Tools are for data, exec is for rendering.

Output ONLY a JSON array. No other text.`

	planResult, err := ai.Ask(&ai.Prompt{
		System:   planSystem,
		Question: prompt,
		Priority: ai.PriorityHigh,
		Provider: model.Provider,
		Model:    model.Model,
		Caller:   "workspace-plan",
	})
	if err != nil {
		sseSend(map[string]any{"type": "error", "message": err.Error()})
		sseSend(map[string]any{"type": "done"})
		return
	}

	// Parse steps
	type step struct {
		Type    string         `json:"type"`
		Code    string         `json:"code,omitempty"`
		HTML    string         `json:"html,omitempty"`
		Name    string         `json:"name,omitempty"`
		Args    map[string]any `json:"args,omitempty"`
		Message string         `json:"message,omitempty"`
	}
	var steps []step
	stepsJSON := extractJSONArray(planResult)
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		// If parsing fails, treat the whole response as a text answer
		sseSend(map[string]any{"type": "response", "message": planResult})
		sseSend(map[string]any{"type": "done"})
		return
	}

	// Describe what we're about to do
	for _, s := range steps {
		switch s.Type {
		case "exec":
			if s.HTML != "" {
				sseSend(map[string]any{"type": "status", "message": "Building app..."})
			} else if s.Code != "" {
				sseSend(map[string]any{"type": "status", "message": "Running code..."})
			}
		case "tool":
			sseSend(map[string]any{"type": "status", "message": "Will fetch: " + s.Name})
		case "respond":
			// nothing to announce
		}
	}

	// Step 2: Execute steps
	var lastExecResult *ExecResult
	var toolResults []string
	responded := false

	for _, s := range steps {
		switch s.Type {
		case "exec":
			code := stripCodeFences(s.Code)
			sseSend(map[string]any{"type": "exec", "code": code, "html": s.HTML})
			// Wait for browser feedback
			fb := waitForExecResult(sessionID, 15*time.Second)
			if fb != nil {
				lastExecResult = fb
				if !fb.OK {
					sseSend(map[string]any{"type": "status", "message": "Error: " + fb.Error})
					// Try to fix
					fixResult, fixErr := ai.Ask(&ai.Prompt{
						System:   "Fix this JavaScript error. Output ONLY the corrected JavaScript code. No markdown, no fences, no explanation. Just the code.",
						Question: fmt.Sprintf("Error: %s\n\nCode that failed:\n%s", fb.Error, s.Code),
						Priority: ai.PriorityHigh,
						Caller:   "workspace-fix",
					})
					if fixErr == nil {
						sseSend(map[string]any{"type": "status", "message": "Fixing..."})
						sseSend(map[string]any{"type": "exec", "code": stripCodeFences(fixResult)})
						fb2 := waitForExecResult(sessionID, 15*time.Second)
						if fb2 != nil {
							lastExecResult = fb2
						}
					}
				}
			}

		case "tool":
			sseSend(map[string]any{"type": "status", "message": "Running " + s.Name + "..."})
			text, isErr, execErr := api.ExecuteTool(r, s.Name, s.Args)
			if execErr != nil || isErr {
				sseSend(map[string]any{"type": "status", "message": s.Name + " failed"})
				continue
			}
			if len(text) > 4000 {
				text = text[:4000]
			}
			toolResults = append(toolResults, fmt.Sprintf("### %s\n%s", s.Name, text))
			sseSend(map[string]any{"type": "status", "message": s.Name + " done"})

		case "respond":
			responded = true
			sseSend(map[string]any{"type": "response", "message": s.Message, "html": app.RenderString(s.Message)})
		}
	}

	// Always synthesise if we have tool results and haven't responded
	if len(toolResults) > 0 && !responded {
		sseSend(map[string]any{"type": "status", "message": "Composing answer..."})
		answer, err := ai.Ask(&ai.Prompt{
			System:   "Summarise the results. Use markdown.",
			Rag:      toolResults,
			Question: prompt,
			Priority: ai.PriorityHigh,
			Caller:   "workspace-synth",
		})
		if err == nil {
			answer = app.StripLatexDollars(answer)
			sseSend(map[string]any{"type": "response", "html": app.RenderString(answer)})
		}
	}

	// If we rendered HTML, offer to save as app
	if lastExecResult != nil && lastExecResult.OK && lastExecResult.DOM != "" {
		sseSend(map[string]any{"type": "status", "message": "App rendered successfully"})
	}

	sseSend(map[string]any{"type": "done"})
}

// stripCodeFences removes markdown code fences from AI output.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	// Remove opening fence: ```js, ```javascript, ```
	for _, prefix := range []string{"```javascript\n", "```js\n", "```html\n", "```\n"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}
	// Remove closing fence
	if strings.HasSuffix(s, "\n```") {
		s = s[:len(s)-4]
	} else if strings.HasSuffix(s, "```") {
		s = s[:len(s)-3]
	}
	return strings.TrimSpace(s)
}
