package home

import (
	"net/http"

	"mu/internal/app"
)

// AssistantHandler serves the logged-in "/" — a clean conversational
// AI interface. Like the landing page but authenticated, with inline
// responses. No sidebar, no dashboard chrome.
func AssistantHandler(w http.ResponseWriter, r *http.Request) {
	page := `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mu</title>
<meta name="description" content="Your personal AI">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/favicon.ico">
<link rel="apple-touch-icon" href="/icon-192.png">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:'Nunito Sans',sans-serif;background:#fff;color:#111;min-height:100vh;display:flex;flex-direction:column}
.assistant{flex:1;display:flex;flex-direction:column;align-items:center;padding:0 20px}
.top-bar{width:100%%;display:flex;justify-content:space-between;align-items:center;padding:16px 0;max-width:720px}
.top-bar .brand{font-size:1.2rem;font-weight:800;letter-spacing:-0.5px;text-decoration:none;color:#111}
.top-bar nav a{color:#555;text-decoration:none;font-size:13px;margin-left:16px}
.top-bar nav a:hover{color:#111}
.spacer{flex:1;display:flex;align-items:center;justify-content:center;width:100%%;max-width:720px}
.prompt-area{width:100%%;margin-bottom:24px}
.prompt-area form{display:flex;align-items:center;gap:0;border:1px solid #ddd;border-radius:6px;background:#fff;padding:4px 4px 4px 12px;transition:border-color 0.2s}
.prompt-area form:focus-within{border-color:#999}
.prompt-area textarea{flex:1;padding:10px 0;border:none;font-size:16px;font-family:inherit;resize:none;line-height:1.4;overflow:hidden;background:transparent;outline:none}
.prompt-area button{flex-shrink:0;width:36px;height:36px;background:#111;color:#fff;border:none;border-radius:6px;cursor:pointer;display:flex;align-items:center;justify-content:center;font-size:18px}
#response{width:100%%;max-width:720px;margin-bottom:32px;font-size:15px;line-height:1.7}
#response .card{background:#f9f9f9;border-radius:8px;padding:16px;margin-bottom:12px}
#response .loading{color:#888;font-size:14px}
</style>
</head>
<body>
<div class="assistant">
  <div class="top-bar">
    <a href="/" class="brand">Mu</a>
    <nav>
      <a href="/home">Home</a>
      <a href="/agent">Agent</a>
      <a href="/news">News</a>
      <a href="/markets">Markets</a>
      <a href="/account">Account</a>
    </nav>
  </div>
  <div class="spacer">
    <div class="prompt-area" id="prompt-area">
      <form id="ask-form">
        <textarea id="prompt" placeholder="Ask anything..." maxlength="1024" rows="1"
          onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();document.getElementById('ask-form').dispatchEvent(new Event('submit'))}"
          oninput="this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px'"></textarea>
        <button type="submit">&#x2192;</button>
      </form>
    </div>
  </div>
  <div id="response"></div>
</div>
<script>
(function(){
var form=document.getElementById('ask-form');
var input=document.getElementById('prompt');
var response=document.getElementById('response');
var promptArea=document.getElementById('prompt-area');
var spacer=document.querySelector('.spacer');

form.addEventListener('submit',function(e){
  e.preventDefault();
  var q=input.value.trim();
  if(!q)return;

  // Move prompt to top
  spacer.style.flex='0';
  spacer.style.alignItems='flex-start';
  spacer.style.paddingTop='0';

  response.innerHTML='<div class="loading">Thinking...</div>';

  var body=JSON.stringify({prompt:q,model:'standard'});
  fetch('/agent',{method:'POST',headers:{'Content-Type':'application/json'},body:body,credentials:'same-origin'})
  .then(function(resp){
    if(!resp.ok){
      response.innerHTML='<div class="card">Something went wrong. <a href="/login">Log in</a> if needed.</div>';
      return;
    }
    var reader=resp.body.getReader();
    var decoder=new TextDecoder();
    var buf='';
    var streamText='';
    var streaming=false;

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
              response.innerHTML='<div class="loading">'+ev.message+'</div>';
            } else if(ev.type==='stream_start'){
              streaming=true;
              streamText='';
              response.innerHTML='<div class="card" id="stream-card"><span id="stream-text" style="white-space:pre-wrap"></span><span style="display:inline-block;width:2px;height:1em;background:#000;vertical-align:text-bottom;animation:blink 0.8s step-end infinite;margin-left:2px"></span></div>';
            } else if(ev.type==='stream_token'){
              streamText+=ev.token;
              var el=document.getElementById('stream-text');
              if(el)el.textContent=streamText;
            } else if(ev.type==='response'){
              response.innerHTML=ev.html;
            } else if(ev.type==='error'){
              response.innerHTML='<div class="card" style="color:#c00">'+ev.message+'</div>';
            }
          }catch(ex){}
        });
        return read();
      });
    }
    return read();
  })
  .catch(function(err){
    response.innerHTML='<div class="card" style="color:#c00">Error: '+err.message+'</div>';
  });

  input.value='';
  input.focus();
});
})();
</script>
<style>@keyframes blink{0%%,100%%{opacity:1}50%%{opacity:0}}</style>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = app.Version // keep import
	w.Write([]byte(page))
}
