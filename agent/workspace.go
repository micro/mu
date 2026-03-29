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
#ws-log{flex:1;overflow-y:auto;padding:8px 0}
#ws-preview{border:1px solid #e0e0e0;border-radius:6px;min-height:200px;display:none;margin-bottom:12px;padding:16px;background:#fff}
#ws-input{display:flex;gap:8px;padding:8px 0}
#ws-input input{flex:1;padding:10px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:15px;font-family:inherit}
#ws-input button{padding:10px 24px;background:#000;color:#fff;border:none;border-radius:6px;cursor:pointer;font-family:inherit;font-size:15px}
#ws-input button:disabled{background:#ccc;cursor:not-allowed}
.ws-msg{margin:4px 0;font-size:14px}
.ws-user{color:#1a1a1a;font-weight:600}
.ws-agent{color:#555}
.ws-status{color:#888;font-size:13px}
.ws-error{color:#c00;font-size:13px}
.ws-code{font-family:'SF Mono',monospace;font-size:12px;background:#f5f5f5;padding:8px;border-radius:4px;margin:4px 0;overflow-x:auto;white-space:pre-wrap}
</style>
<div id="ws">
<div id="ws-log"></div>
<div id="ws-preview"></div>
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
var flowId='';

function addMsg(cls,text){
  var d=document.createElement('div');
  d.className='ws-msg '+cls;
  d.textContent=text;
  log.appendChild(d);
  log.scrollTop=log.scrollHeight;
}

function addHTML(cls,html){
  var d=document.createElement('div');
  d.className='ws-msg '+cls;
  d.innerHTML=html;
  log.appendChild(d);
  log.scrollTop=log.scrollHeight;
}

function addCode(code){
  var d=document.createElement('pre');
  d.className='ws-code';
  d.textContent=code.length>500?code.slice(0,500)+'...':code;
  log.appendChild(d);
  log.scrollTop=log.scrollHeight;
}

input.addEventListener('keydown',function(e){if(e.key==='Enter')send()});

function send(){
  var prompt=input.value.trim();
  if(!prompt)return;
  addMsg('ws-user',prompt);
  input.value='';
  btn.disabled=true;

  var es=new EventSource('/agent/workspace?prompt='+encodeURIComponent(prompt)+'&flow_id='+flowId);

  es.onmessage=function(e){
    try{
      var ev=JSON.parse(e.data);

      if(ev.type==='flow_id'){
        flowId=ev.flow_id;
      }
      else if(ev.type==='status'){
        addMsg('ws-status',ev.message);
      }
      else if(ev.type==='exec'){
        // Execute code in the preview context
        addMsg('ws-status','Executing code...');
        if(ev.html){
          preview.style.display='block';
          preview.innerHTML=ev.html;
          // Execute any scripts in the injected HTML
          var scripts=preview.querySelectorAll('script');
          scripts.forEach(function(s){
            var ns=document.createElement('script');
            ns.textContent=s.textContent;
            preview.appendChild(ns);
            s.remove();
          });
        }
        if(ev.code){
          addCode(ev.code);
          // Wrap in async IIFE so await works
          (async function(){
            try{
              var result=await eval('(async function(){'+ev.code+'})()');
              feedback(flowId,true,String(result||'ok'),'',preview.textContent.slice(0,500));
            }catch(err){
              addMsg('ws-error','Error: '+err.message);
              feedback(flowId,false,'',err.message,preview.textContent.slice(0,500));
            }
          })();
        } else {
          // HTML only, no code to eval
          feedback(flowId,true,'rendered','',preview.textContent.slice(0,500));
        }
      }
      else if(ev.type==='response'){
        addHTML('ws-agent',ev.html||ev.message||'');
      }
      else if(ev.type==='error'){
        addMsg('ws-error',ev.message);
      }
      else if(ev.type==='done'){
        btn.disabled=false;
        es.close();
      }
      else if(ev.type==='save'){
        addHTML('ws-agent','<a href="/apps/'+ev.slug+'/run" style="font-weight:600">Open App: '+ev.name+'</a>');
      }
    }catch(ex){console.error('parse error',ex)}
  };

  es.onerror=function(){
    btn.disabled=false;
    es.close();
  };
}

function feedback(fid,ok,result,error,dom){
  fetch('/agent/feedback',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({flow_id:fid,ok:ok,result:result,error:error,dom:dom})
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

	flow := &Flow{
		ID:        newFlowID(),
		AccountID: acc.ID,
		Prompt:    prompt,
		Status:    "running",
		CreatedAt: time.Now().UTC(),
	}
	saveFlow(flow)

	sseSend := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	sseSend(map[string]any{"type": "flow_id", "flow_id": flow.ID})
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

	// Step 2: Execute steps
	var lastExecResult *ExecFeedback
	var toolResults []string
	responded := false

	for _, s := range steps {
		switch s.Type {
		case "exec":
			code := stripCodeFences(s.Code)
			sseSend(map[string]any{"type": "exec", "code": code, "html": s.HTML})
			// Wait for browser feedback
			fb := waitForFeedback(flow.ID, 15*time.Second)
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
						fb2 := waitForFeedback(flow.ID, 15*time.Second)
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

	updateFlow(flow.ID, func(f *Flow) {
		f.Status = "done"
		f.Answer = prompt
	})

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
