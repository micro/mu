package app

import "encoding/json"

// JSString returns s as a safely-quoted JavaScript string literal (with
// surrounding quotes) for embedding in inline scripts.
func JSString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// ChatConfig configures the shared chat component.
type ChatConfig struct {
	// Guest renders the sign-up CTA on the free-query limit (401).
	Guest bool
	// ContextID seeds the conversation's server-side thread id, so follow-up
	// messages continue the same session. Empty starts a new session.
	ContextID string
	// InitialConvHTML is pre-rendered conversation HTML (prior turns) injected
	// into the log when reopening a session. When set, the component does not
	// restore from sessionStorage.
	InitialConvHTML string
	// HideSuggestions suppresses the component's built-in suggestion pills — used
	// where the host page supplies its own (e.g. Home's personalised chips).
	HideSuggestions bool
	// OfferAgentPicker shows which agent is answering, and lets the reader
	// change it.
	//
	// The component has always sent window.muActiveAgent, and the server has
	// always honoured it — but the only thing that could set it was the rail on
	// /agent. So somebody could create an agent, go to the input on Home, and
	// have no way to reach it: the agent existed and could not be talked to
	// from the place you would naturally talk to it. That is most of why a new
	// agent reads as a toy.
	OfferAgentPicker bool
	// StorageNS namespaces the component's sessionStorage keys so different
	// surfaces (Home, /agent) keep separate in-tab conversations and never show
	// each other's. When empty the component is ephemeral: it neither restores
	// nor saves, so it always starts clean (used for Home's quick-ask box).
	StorageNS string
}

// ChatComponent returns the single, shared chat UI used everywhere Mu talks to
// the agent — the home assistant, the guest landing page, and the /agent chat
// surface. It is self-contained (one <div id="mu-chat"> with input, suggestion
// pills and a conversation log, plus scoped styles and a small script). The
// script POSTs to /agent and renders the SSE stream inline, tracks the server
// thread id (context_id) so a session continues across turns, and keeps the
// conversation in the DOM + sessionStorage for guests. window.muChatAsk(text)
// submits a query; window.muChatNew() starts a fresh session.
func ChatComponent(cfg ChatConfig) string {
	guestJS := "false"
	if cfg.Guest {
		guestJS = "true"
	}
	// The cards go with the question, always.
	//
	// This was a checkbox — "use live context" — off by default, remembered in
	// localStorage. Asking somebody to opt in to the product working properly
	// is asking a question they have no way to answer: the cost is tokens they
	// cannot see and the benefit is an answer they have not read yet. On Home
	// the cards are on the screen, so an answer that ignores them is the wrong
	// answer, and there is nothing to decide.
	// The same sessionStorage key the rail on /agent uses, so a choice made in
	// one place holds in the other. Two pickers disagreeing about who is
	// answering would be worse than one picker.
	agentPicker := ""
	if cfg.OfferAgentPicker {
		agentPicker = `<label id="mu-chat-agent" title="Which of your agents answers. Each has its own instructions and its own scope">` +
			`answering as <select id="mu-chat-agent-pick"><option value="">Micro (default)</option></select></label>`
	}

	initialConv := ""
	if cfg.InitialConvHTML != "" {
		initialConv = cfg.InitialConvHTML
	}

	html := `<div id="mu-chat">
  <form id="mu-chat-form">
    <textarea id="mu-chat-input" placeholder="Try: give me a morning brief" maxlength="1024" rows="1"
      onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();document.getElementById('mu-chat-form').dispatchEvent(new Event('submit'))}"
      oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,140)+'px'"></textarea>
    <button type="submit" aria-label="Send">&#x2192;</button>
  </form>
  <div id="mu-chat-opts">` + agentPicker + `</div>
  <div id="mu-chat-suggest"></div>
  <div id="mu-chat-conv">` + initialConv + `</div>
</div>

<style>
#mu-chat{max-width:760px;margin:0 auto;width:100%}
#mu-chat-form{display:flex;align-items:center;gap:0;border:1px solid #ddd;border-radius:6px;background:#fff;padding:4px 4px 4px 12px;transition:border-color .2s;position:sticky;top:8px;z-index:5}
#mu-chat-form:focus-within{border-color:#999}
#mu-chat-opts{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin:6px 0 0}
#mu-chat-opts:empty{margin:0}
#mu-chat-agent{display:flex;align-items:center;gap:6px;font-size:12px;color:#999;cursor:pointer;user-select:none}
#mu-chat-agent select{width:auto;padding:2px 4px;font-size:12px;font-family:inherit;color:#555;border:1px solid #e0e0e0;border-radius:4px;background:#fff}
#mu-chat-input{flex:1;padding:10px 0;border:none;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:transparent;outline:none}
#mu-chat-form button{flex-shrink:0;width:36px;height:36px;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:18px}
#mu-chat-suggest{margin-top:16px}
.mu-pills{display:flex;gap:8px;flex-wrap:wrap;justify-content:center}
.mu-pills a{padding:8px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:13px;color:#555;text-decoration:none;cursor:pointer}
.mu-pills a:hover{background:#f5f5f5}
#mu-chat-conv{margin-top:24px;font-size:15px;line-height:1.7;text-align:left}
#mu-chat-conv:empty{margin-top:0}
.mu-user{margin:0 0 12px;padding:10px 14px;background:#f5f5f5;border-radius:8px;font-size:14px;color:#333;scroll-margin-top:64px}
.mu-agent{margin-bottom:24px;scroll-margin-top:64px}
.mu-think{color:#888;font-size:14px}
.mu-err{color:#c00}
.mu-cta{padding:12px 14px;border:1px solid #e0e0e0;border-radius:8px;background:#fafafa;font-size:14px}
.mu-cta a{color:#111;font-weight:600;text-decoration:none}
.mu-cursor{display:inline-block;width:2px;height:1em;background:#000;vertical-align:text-bottom;animation:mublink .8s step-end infinite;margin-left:2px}
@keyframes mublink{0%,100%{opacity:1}50%{opacity:0}}
/* Answer content must never widen the chat column. Without this a long code
   block, an unbroken URL or a wide table stretches the flex item, which drags
   the input off-screen on mobile — the column ends up wider than the viewport.
   Each kind of wide content is contained in its own block instead. */
#mu-chat,#mu-chat-conv,#mu-chat-form{min-width:0;max-width:100%}
#mu-chat-conv,.mu-user{overflow-wrap:break-word}
#mu-chat-conv pre{overflow-x:auto;max-width:100%}
#mu-chat-conv table{display:block;overflow-x:auto;max-width:100%}
#mu-chat-conv img{max-width:100%;height:auto}
#mu-chat .card{max-width:100%;border:1px solid #e0e0e0;border-radius:8px;padding:16px 18px;margin-bottom:12px;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.04)}
#mu-chat .card h4{margin:0 0 8px;font-size:1em;font-weight:600}
#mu-chat .card a,#mu-chat .link,#mu-chat a.link{color:#111}
/* Self-contained typography so rendered answers look right anywhere. */
#mu-chat .card h1,#mu-chat .card h2,#mu-chat .card h3{margin:16px 0 8px;line-height:1.3;font-weight:700}
#mu-chat .card h1{font-size:1.3em}
#mu-chat .card h2{font-size:1.15em}
#mu-chat .card h3{font-size:1.02em}
#mu-chat .card>h1:first-child,#mu-chat .card>h2:first-child,#mu-chat .card>h3:first-child,#mu-chat .card>p:first-child{margin-top:0}
#mu-chat .card p{margin:0 0 10px}
#mu-chat .card ul,#mu-chat .card ol{margin:0 0 12px;padding-left:22px}
#mu-chat .card li{margin:0 0 5px}
#mu-chat .card hr{border:none;border-top:1px solid #eee;margin:14px 0}
#mu-chat .card>*:last-child{margin-bottom:0}
.mu-think{display:flex;align-items:center}
.mu-spin{display:inline-block;width:11px;height:11px;border:2px solid #ddd;border-top-color:#666;border-radius:50%;animation:muspin .7s linear infinite;margin-right:8px;flex-shrink:0}
@keyframes muspin{to{transform:rotate(360deg)}}
.mu-think-t{color:#bbb;font-size:12px;margin-left:6px}
</style>

<script>
(function(){
var GUEST=` + guestJS + `;
var contextId=` + JSString(cfg.ContextID) + `;
var SESSION=` + boolJS(cfg.InitialConvHTML != "") + `;
var HIDE_SUGGEST=` + boolJS(cfg.HideSuggestions) + `;
var form=document.getElementById('mu-chat-form');
var input=document.getElementById('mu-chat-input');
var conv=document.getElementById('mu-chat-conv');
var sugDiv=document.getElementById('mu-chat-suggest');
if(!form)return;
var NS=` + JSString(cfg.StorageNS) + `;
// Persistence is per-surface and opt-in. With no namespace the component is
// ephemeral (Home's quick-ask box) so it never restores or leaks a
// conversation — in particular it must not show the /agent app's thread.
var PERSIST=!!NS;
var CKEY='mu_chat_conv:'+NS;
var HKEY='mu_chat_hist:'+NS;
var TKEY='mu_chat_ctx:'+NS;
var history=[];

// A reopened server session is authoritative; otherwise restore this surface's
// own in-tab conversation so a reload doesn't lose it — including the server
// thread id (context_id), so a follow-up after a reload continues the same
// conversation instead of starting a new one.
if(!SESSION && PERSIST){
  try{
    var savedConv=sessionStorage.getItem(CKEY);
    if(savedConv)conv.innerHTML=savedConv;
    var savedHist=sessionStorage.getItem(HKEY);
    if(savedHist)history=JSON.parse(savedHist)||[];
    var savedCtx=sessionStorage.getItem(TKEY);
    if(savedCtx)contextId=savedCtx;
  }catch(e){}
}

function esc(s){return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function protectCurrencyDollars(s){return String(s||'').replace(/\$(?=\d)/g,'$\u2060');}

var SUGGEST=['Give me a morning brief','What is moving in markets?','Weather in San Francisco','Find today\'s AI news'];
function showSuggestions(){
  if(HIDE_SUGGEST){sugDiv.innerHTML='';return;}
  if(conv.innerHTML.trim()){sugDiv.innerHTML='';return;}
  // No guest note here. The page that renders a guest chat says it once, in
  // its own markup, under the box — and this said it a second time directly
  // above that, so the landing page told a visitor the same thing twice in two
  // stacked paragraphs. One offer, stated once, by whoever owns the page.
  var h='<div class="mu-pills">';
  SUGGEST.forEach(function(s){h+='<a href="#" data-q="'+esc(s)+'">'+esc(s)+'</a>';});
  h+='</div>';
  sugDiv.innerHTML=h;
  sugDiv.querySelectorAll('a').forEach(function(a){
    a.addEventListener('click',function(e){e.preventDefault();input.value=a.dataset.q;ask(a.dataset.q);});
  });
}

function save(){
  if(SESSION||!PERSIST)return; // server owns reopened sessions; ephemeral surfaces don't save
  try{
    sessionStorage.setItem(CKEY,conv.innerHTML);
    sessionStorage.setItem(HKEY,JSON.stringify(history.slice(-6)));
    sessionStorage.setItem(TKEY,contextId||'');
  }catch(e){}
}

function ask(q){
  q=String(q||'').trim();
  if(!q)return;
  sugDiv.innerHTML='';
  var u=document.createElement('div');u.className='mu-user';u.textContent=q;conv.appendChild(u);
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
  input.value='';input.style.height='auto';input.focus();

  var workLabel='Processing';
  var t0=Date.now();
  var timer=null;
  function renderWork(){
    var dots=['.','..','...'][Math.floor((Date.now()-t0)/450)%3];
    var secs=Math.round((Date.now()-t0)/1000);
    a.innerHTML='<div class="mu-think"><span class="mu-spin"></span><span>'+esc(workLabel)+dots+'</span>'+(secs>=1?'<span class="mu-think-t">'+secs+'s</span>':'')+'</div>';
  }
  function startWork(label){if(label)workLabel=label;renderWork();if(!timer)timer=setInterval(renderWork,450);}
  function stopWork(){if(timer){clearInterval(timer);timer=null;}}
  startWork('Processing');

  save();
  // Anchor the new exchange just under the sticky input so it reads top-down;
  // don't chase the bottom as the answer streams in.
  u.scrollIntoView({behavior:'smooth',block:'start'});
  var streamText='';
  var streaming=false;
  var body=JSON.stringify({prompt:q,model:'standard',history:history.slice(-6),context_id:contextId||'',agent:(window.muActiveAgent||''),cards:true});
  fetch('/agent',{method:'POST',headers:{'Content-Type':'application/json'},body:body,credentials:'same-origin'})
  .then(function(resp){
    if(resp.status===401){
      return resp.json().catch(function(){return {};}).then(function(j){
        stopWork();
        var msg=esc(j.error||'Sign up to keep using the AI agent.');
        a.innerHTML='<div class="mu-cta">'+msg+' <a href="/signup">Sign up →</a> <a href="/login?redirect=/agent" style="margin-left:10px">Log in</a></div>';
        save();
        throw 'handled';
      });
    }
    if(!resp.ok||!resp.body){stopWork();a.innerHTML='<div class="mu-err">Something went wrong. Please try again.</div>';save();throw 'handled';}
    var reader=resp.body.getReader();
    var decoder=new TextDecoder();
    var buf='';
    function read(){
      return reader.read().then(function(chunk){
        if(chunk.done){stopWork();save();return;}
        buf+=decoder.decode(chunk.value,{stream:true});
        var lines=buf.split('\n');
        buf=lines.pop();
        lines.forEach(function(line){
          if(line.indexOf('data: ')!==0)return;
          try{
            var ev=JSON.parse(line.slice(6));
            if(ev.type==='flow_id'){
              // Continue this server session on the next message. Persist it now
              // (it arrives before the answer) so a reload mid-stream still
              // threads the follow-up onto this same conversation.
              // A conversation that did not exist a moment ago is one the page
              // beside this can now list. Without this the rail only learned
              // about a conversation on the next reload, so saying hello and
              // then refreshing made a row appear that had not been there while
              // you were talking — the list looked like it was lagging reality,
              // because it was. Optional: the same component runs on Home and
              // the landing page, where there is no rail to tell.
              // The conversation, where there is one — a signed-in caller. A
              // guest is not recorded, so the run id carries the thread for the
              // length of the page and no further.
              var id=ev.thread||ev.flow_id;
              if(id){
                var fresh=!contextId;
                contextId=id;save();
                if(fresh&&window.muSessionStarted)window.muSessionStarted(id,q);
              }
            }else if(ev.type==='thinking'){
              startWork(ev.message);
            }else if(ev.type==='stream_start'){
              streamText='';streaming=false;startWork('Composing');
            }else if(ev.type==='stream_token'){
              streamText+=ev.token;
              if(!streaming){
                streaming=true;stopWork();
                a.innerHTML='<div style="white-space:pre-wrap"><span id="mu-stream-out"></span><span class="mu-cursor"></span></div>';
              }
              var el=document.getElementById('mu-stream-out');
              if(el)el.textContent=protectCurrencyDollars(streamText);
            }else if(ev.type==='response'){
              stopWork();
              a.innerHTML=ev.html;
              history.push({prompt:q,answer:streamText});
              save();
            }else if(ev.type==='error'){
              stopWork();
              a.innerHTML='<div class="mu-err">'+esc(ev.message)+'</div>';
              save();
            }
          }catch(ex){}
        });
        return read();
      });
    }
    return read();
  })
  .catch(function(err){
    stopWork();
    if(err==='handled')return;
    a.innerHTML='<div class="mu-err">Error: '+esc(err&&err.message||err)+'</div>';
    save();
  });
}

form.addEventListener('submit',function(e){e.preventDefault();ask(input.value);});
showSuggestions();

// Start a fresh session (clears the log + thread id).
window.muChatNew=function(){
  conv.innerHTML='';history=[];contextId='';
  try{sessionStorage.removeItem(CKEY);sessionStorage.removeItem(HKEY);sessionStorage.removeItem(TKEY);}catch(e){}
  showSuggestions();input.focus();
};
// Exposed so server-rendered prefill (?q= / ?prompt=) can auto-submit.

// Which agent answers. The component has always sent window.muActiveAgent and
// the server has always honoured it; until now the only thing that could set it
// was the rail on /agent, so an agent created on /agents could not be reached
// from the input on Home.
//
// Same sessionStorage key as that rail, so the choice holds across both.
(function(){
  var sel=document.getElementById('mu-chat-agent-pick');
  if(!sel) return;
  var KEY='mu_active_agent';
  try{ window.muActiveAgent=window.muActiveAgent||sessionStorage.getItem(KEY)||''; }catch(e){}

  // Where to write to whichever agent is answering.
  //
  // The host page renders the address; the id is the contract between them.
  // Picking an agent changed which name answered and nothing else on the
  // screen, so the page read "answering as Test" directly above "write to it
  // at agent@" — the first question anybody has about an agent, answered
  // wrongly by the one control that should have changed it.
  var addrs={};
  function showAddr(){
    var el=document.getElementById('mu-agent-addr');
    if(!el) return;
    var a=addrs[sel.value]||addrs[''];
    if(a) el.textContent=a;
  }

  sel.addEventListener('change',function(){
    window.muActiveAgent=sel.value;
    try{ sessionStorage.setItem(KEY, sel.value); }catch(e){}
    showAddr();
  });

  fetch('/agents/data',{headers:{'Accept':'application/json'}})
    .then(function(r){return r.json();})
    .then(function(d){
      // Only agents that run here. An external one is a credential and a scope
      // for something that calls in from outside; nothing here can hand a
      // question to it, so "answering as" it meant Micro answering with that
      // agent's scope and an empty prompt — near enough the default, silently.
      var list=((d&&d.agents)||[]).filter(function(a){return a.kind!=='external';});
      // No agents, nothing to choose between: a picker with one option is a
      // control that only takes up room.
      if(!list.length){ var l=document.getElementById('mu-chat-agent'); if(l) l.remove(); return; }
      // Rebuilt, not appended to. This ran twice on one page — a soft
      // navigation back to Home re-runs it against a select that already has
      // its options — and every agent appeared once per run, so the list read
      // Micro, Test, Test. Building the whole list is right however many times
      // it happens.
      sel.innerHTML='';
      var def=document.createElement('option');
      def.value=''; def.textContent='Micro (default)';
      sel.appendChild(def);
      addrs={'':(d&&d.address)||''};
      list.forEach(function(a){
        var o=document.createElement('option');
        o.value=a.id; o.textContent=a.name;
        if(a.description) o.title=a.description;
        sel.appendChild(o);
        if(a.address) addrs[a.id]=a.address;
      });
      // Restore the choice, unless that agent has since been removed.
      if(window.muActiveAgent){
        sel.value=window.muActiveAgent;
        if(sel.value!==window.muActiveAgent){ window.muActiveAgent=''; sel.value=''; }
      }
      showAddr();
    })
    .catch(function(){ var l=document.getElementById('mu-chat-agent'); if(l) l.remove(); });
})();
window.muChatAsk=ask;
})();
</script>`

	return html
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
