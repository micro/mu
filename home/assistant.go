package home

import (
	"net/http"

	"mu/internal/app"
)

func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	// Pick up query from sidebar input
	prefill := r.URL.Query().Get("q")

	content := `<div>
<div style="margin-bottom:24px">
  <form id="ask-form" style="display:flex;align-items:center;gap:0;border:1px solid #ddd;border-radius:6px;padding:4px 4px 4px 12px">
    <textarea id="ask-input" placeholder="Ask anything..." maxlength="1024" rows="1"
      style="flex:1;padding:10px 0;border:none;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:transparent;outline:none"
      onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();document.getElementById('ask-form').dispatchEvent(new Event('submit'))}"
      oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px'"></textarea>
    <button type="submit" style="flex-shrink:0;width:36px;height:36px;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:18px">&#x2192;</button>
  </form>
</div>
<div id="conversation" style="font-size:15px;line-height:1.7"></div>
</div>

<style>
.msg-user{margin-bottom:16px;padding:10px 14px;background:#f5f5f5;border-radius:8px;font-size:14px;color:#333}
.msg-agent{margin-bottom:24px}
.msg-agent .card{margin:0}
.stream-cursor{display:inline-block;width:2px;height:1em;background:#000;vertical-align:text-bottom;animation:blink 0.8s step-end infinite;margin-left:2px}
@keyframes blink{0%,100%{opacity:1}50%{opacity:0}}
</style>

<script>
(function(){
var form=document.getElementById('ask-form');
var input=document.getElementById('ask-input');
var conv=document.getElementById('conversation');

form.addEventListener('submit',function(e){
  e.preventDefault();
  var q=input.value.trim();
  if(!q)return;

  // Show user message
  var userDiv=document.createElement('div');
  userDiv.className='msg-user';
  userDiv.textContent=q;
  conv.appendChild(userDiv);

  // Show thinking
  var agentDiv=document.createElement('div');
  agentDiv.className='msg-agent';
  agentDiv.innerHTML='<div style="color:#888;font-size:14px">Thinking...</div>';
  conv.appendChild(agentDiv);

  input.value='';
  input.focus();
  agentDiv.scrollIntoView({behavior:'smooth',block:'end'});

  var body=JSON.stringify({prompt:q,model:'standard'});
  fetch('/agent',{method:'POST',headers:{'Content-Type':'application/json'},body:body,credentials:'same-origin'})
  .then(function(resp){
    if(!resp.ok){
      agentDiv.innerHTML='<div class="card" style="color:#c00">Something went wrong.</div>';
      return;
    }
    var reader=resp.body.getReader();
    var decoder=new TextDecoder();
    var buf='';
    var streamText='';

    function read(){
      return reader.read().then(function(chunk){
        if(chunk.done)return;
        buf+=decoder.decode(chunk.value,{stream:true});
        var lines=buf.split('\n');
        buf=lines.pop();
        lines.forEach(function(line){
          if(!line.startsWith('data: '))return;
          try{
            var ev=JSON.parse(line.slice(6));
            if(ev.type==='thinking'){
              agentDiv.innerHTML='<div style="color:#888;font-size:14px">'+ev.message+'</div>';
            } else if(ev.type==='stream_start'){
              streamText='';
              agentDiv.innerHTML='<div style="white-space:pre-wrap"><span id="stream-out"></span><span class="stream-cursor"></span></div>';
            } else if(ev.type==='stream_token'){
              streamText+=ev.token;
              var el=document.getElementById('stream-out');
              if(el)el.textContent=streamText;
              agentDiv.scrollIntoView({behavior:'smooth',block:'end'});
            } else if(ev.type==='response'){
              agentDiv.innerHTML='<div class="card">'+ev.html+'</div>';
            } else if(ev.type==='error'){
              agentDiv.innerHTML='<div class="card" style="color:#c00">'+ev.message+'</div>';
            }
          }catch(ex){}
        });
        return read();
      });
    }
    return read();
  })
  .catch(function(err){
    agentDiv.innerHTML='<div class="card" style="color:#c00">Error: '+err.message+'</div>';
  });
});
})();
</script>`

	// Inject prefill if query param provided
	if prefill != "" {
		escaped := htmlEsc(prefill)
		content += `<script>(function(){
var input=document.getElementById('ask-input');
if(input){input.value="` + escaped + `";document.getElementById('ask-form').dispatchEvent(new Event('submit'));}
history.replaceState(null,'','/');
})()</script>`
	}

	html := app.RenderHTMLForRequest("Micro", "Your personal AI", content, r)
	w.Write([]byte(html))
}
