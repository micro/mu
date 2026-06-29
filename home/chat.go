package home

import "encoding/json"

// jsString returns s as a safely-quoted JavaScript string literal (with
// surrounding quotes) for embedding in inline scripts.
func jsString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// chatComponent returns the shared inline-streaming chat UI used on both the
// guest landing page (`/` logged out) and the assistant page (`/` logged in).
//
// It is intentionally self-contained: one <div id="mu-chat"> with the input,
// suggestion pills, and a conversation log, plus scoped styles and a small
// script. The script POSTs to /agent and renders the SSE stream inline, keeps
// the conversation in the DOM + sessionStorage so a reload restores it, and
// threads the most recent turns back to the server (the `history` field) so
// follow-up questions keep context — for guests too, with no server-side
// persistence required.
//
// When `guest` is true and the server returns 401 (free query limit reached),
// the chat shows an inline sign-up call-to-action instead of a generic error.
func chatComponent(guest bool) string {
	guestJS := "false"
	if guest {
		guestJS = "true"
	}

	html := `<div id="mu-chat">
  <form id="mu-chat-form">
    <textarea id="mu-chat-input" placeholder="Ask anything..." maxlength="1024" rows="1"
      onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();document.getElementById('mu-chat-form').dispatchEvent(new Event('submit'))}"
      oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,140)+'px'"></textarea>
    <button type="submit" aria-label="Send">&#x2192;</button>
  </form>
  <div id="mu-chat-suggest"></div>
  <div id="mu-chat-conv"></div>
</div>

<style>
#mu-chat{max-width:750px;margin:0 auto;width:100%}
#mu-chat-form{display:flex;align-items:center;gap:0;border:1px solid #ddd;border-radius:6px;background:#fff;padding:4px 4px 4px 12px;transition:border-color .2s}
#mu-chat-form:focus-within{border-color:#999}
#mu-chat-input{flex:1;padding:10px 0;border:none;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:transparent;outline:none}
#mu-chat-form button{flex-shrink:0;width:36px;height:36px;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:18px}
#mu-chat-suggest{margin-top:16px}
.mu-pills{display:flex;gap:8px;flex-wrap:wrap;justify-content:center}
.mu-pills a{padding:8px 14px;border:1px solid #e0e0e0;border-radius:6px;font-size:13px;color:#555;text-decoration:none;cursor:pointer}
.mu-pills a:hover{background:#f5f5f5}
#mu-chat-conv{margin-top:24px;font-size:15px;line-height:1.7;text-align:left}
#mu-chat-conv:empty{margin-top:0}
.mu-user{margin:0 0 12px;padding:10px 14px;background:#f5f5f5;border-radius:8px;font-size:14px;color:#333}
.mu-agent{margin-bottom:24px}
.mu-think{color:#888;font-size:14px}
.mu-err{color:#c00}
.mu-cta{padding:12px 14px;border:1px solid #e0e0e0;border-radius:8px;background:#fafafa;font-size:14px}
.mu-cta a{color:#111;font-weight:600;text-decoration:none}
.mu-cursor{display:inline-block;width:2px;height:1em;background:#000;vertical-align:text-bottom;animation:mublink .8s step-end infinite;margin-left:2px}
@keyframes mublink{0%,100%{opacity:1}50%{opacity:0}}
#mu-chat .card{max-width:100%;border:1px solid #e0e0e0;border-radius:8px;padding:16px 18px;margin-bottom:12px;background:#fff;box-shadow:0 1px 2px rgba(0,0,0,.04)}
#mu-chat .card h4{margin:0 0 8px;font-size:1em;font-weight:600}
#mu-chat .card a,#mu-chat .link,#mu-chat a.link{color:#111}
/* Self-contained typography so rendered answers look right even where mu.css
   isn't loaded (e.g. the guest landing page). */
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
var form=document.getElementById('mu-chat-form');
var input=document.getElementById('mu-chat-input');
var conv=document.getElementById('mu-chat-conv');
var sugDiv=document.getElementById('mu-chat-suggest');
if(!form)return;
var CKEY='mu_chat_conv';
var HKEY='mu_chat_hist';
var history=[];

try{
  var savedConv=sessionStorage.getItem(CKEY);
  if(savedConv)conv.innerHTML=savedConv;
  var savedHist=sessionStorage.getItem(HKEY);
  if(savedHist)history=JSON.parse(savedHist)||[];
}catch(e){}

function esc(s){return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}

var SUGGEST=['Today\'s news','Bitcoin price','Weather forecast','What is Mu?'];
function showSuggestions(){
  if(conv.innerHTML.trim()){sugDiv.innerHTML='';return;}
  var h='<div class="mu-pills">';
  SUGGEST.forEach(function(s){h+='<a href="#" data-q="'+esc(s)+'">'+esc(s)+'</a>';});
  h+='</div>';
  sugDiv.innerHTML=h;
  sugDiv.querySelectorAll('a').forEach(function(a){
    a.addEventListener('click',function(e){e.preventDefault();input.value=a.dataset.q;ask(a.dataset.q);});
  });
}

function save(){
  try{
    sessionStorage.setItem(CKEY,conv.innerHTML);
    sessionStorage.setItem(HKEY,JSON.stringify(history.slice(-6)));
  }catch(e){}
}

function ask(q){
  q=String(q||'').trim();
  if(!q)return;
  sugDiv.innerHTML='';
  var u=document.createElement('div');u.className='mu-user';u.textContent=q;conv.appendChild(u);
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
  input.value='';input.style.height='auto';input.focus();

  // Animated progress indicator: a spinner + label + elapsed time, ticking
  // until the answer renders. The response often arrives in one chunk rather
  // than token-by-token, so this is what tells the user something is happening.
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
  a.scrollIntoView({behavior:'smooth',block:'end'});
  var streamText='';
  var streaming=false;
  var body=JSON.stringify({prompt:q,model:'standard',history:history.slice(-6)});
  fetch('/agent',{method:'POST',headers:{'Content-Type':'application/json'},body:body,credentials:'same-origin'})
  .then(function(resp){
    if(resp.status===401){
      return resp.json().catch(function(){return {};}).then(function(j){
        stopWork();
        var msg=esc(j.error||'Sign up to keep using the AI agent.');
        a.innerHTML='<div class="mu-cta">'+msg+' <a href="/signup">Sign up free →</a></div>';
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
            if(ev.type==='thinking'){
              // Keep the animated indicator; just update its label.
              startWork(ev.message);
            }else if(ev.type==='stream_start'){
              // Don't blank to a bare cursor — keep the progress indicator
              // running until the first token actually arrives.
              streamText='';streaming=false;startWork('Composing');
            }else if(ev.type==='stream_token'){
              streamText+=ev.token;
              if(!streaming){
                streaming=true;stopWork();
                a.innerHTML='<div style="white-space:pre-wrap"><span id="mu-stream-out"></span><span class="mu-cursor"></span></div>';
              }
              var el=document.getElementById('mu-stream-out');
              if(el)el.textContent=streamText;
              a.scrollIntoView({behavior:'smooth',block:'end'});
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

// Exposed so server-rendered prefill (?q= / ?prompt=) can auto-submit.
window.muChatAsk=ask;
})();
</script>`

	return html
}
