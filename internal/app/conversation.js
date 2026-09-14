(function(){
var config=JSON.parse(document.getElementById("conversation-config").textContent);
var contextId=config.contextId;
var attachment=config.attachment;
var SESSION=config.serverOwned;
var PENDING=config.pending;
var AGENT_NAME=config.agentName;

var form=document.getElementById('mu-chat-form');
var input=document.getElementById('mu-chat-input');
var conv=document.getElementById('mu-chat-conv');
function nearBottom(){
  if(!conv) return true;
  return (conv.scrollTop+conv.clientHeight)>=(conv.scrollHeight-nearEnough);
}
var nearEnough=120;
function revealQuestion(){toBottom(false);}
function toBottom(force,smooth){
  if(!force && !nearBottom()) return;
  requestAnimationFrame(function(){
    conv.scrollTo({top:conv.scrollHeight,behavior:smooth?'smooth':'auto'});
  });
}
function fitConv(){var v=window.visualViewport;document.documentElement.style.setProperty('--visible-height',(v?v.height:window.innerHeight)+'px');}
if(!form)return;
input.addEventListener('keydown',function(e){if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();form.requestSubmit();}});
input.addEventListener('input',function(){input.style.height='auto';input.style.height=Math.min(input.scrollHeight,140)+'px';});
var NS=config.storageNS;
var PERSIST=!!NS;
var CKEY='mu_chat_conv:'+NS;
var HKEY='mu_chat_hist:'+NS;
var TKEY='mu_chat_ctx:'+NS;
var DKEY='mu_chat_draft:'+NS;
function draftKey(){return DKEY+(SESSION?':'+(contextId||attachment):'');}
function scrollKey(){return 'mu_chat_scroll:'+NS+':'+(contextId||attachment);}
var history=[];

if(!SESSION && PERSIST){
  try{
    var savedConv=sessionStorage.getItem(CKEY);
    if(savedConv)conv.innerHTML=savedConv;

    var savedHist=sessionStorage.getItem(HKEY);
    if(savedHist)history=JSON.parse(savedHist)||[];
    var savedCtx=sessionStorage.getItem(TKEY);
    if(savedCtx)contextId=savedCtx;
    var savedDraft=sessionStorage.getItem(draftKey());
    if(savedDraft)input.value=savedDraft;
  }catch(e){}
}

if(SESSION&&PERSIST){try{var savedDraft=sessionStorage.getItem('mu_chat_continue_draft:'+contextId)||sessionStorage.getItem(draftKey());if(savedDraft)input.value=savedDraft;sessionStorage.removeItem('mu_chat_continue_draft:'+contextId);}catch(e){}}

function esc(s){return String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');}
function protectCurrencyDollars(s){return String(s||'').replace(/\$(?=\d)/g,'$\u2060');}

function save(){
  if(SESSION||!PERSIST)return; // server owns reopened sessions; ephemeral surfaces don't save
  try{
    sessionStorage.setItem(CKEY,conv.innerHTML);
    sessionStorage.setItem(HKEY,JSON.stringify(NS==='landing'?history:history.slice(-6)));
    sessionStorage.setItem(TKEY,contextId||'');
  }catch(e){}
}
function saveDraft(){
  if(!PERSIST)return;
  try{sessionStorage.setItem(draftKey(),input.value||'');}catch(e){}
}

function agentName(){
  try{
    var sel=document.getElementById('mu-chat-agent-pick');
    if(sel&&sel.value&&sel.selectedIndex>=0){
      var label=(sel.options[sel.selectedIndex].textContent||'').trim();
      if(label)return label;
    }
  }catch(e){}
  return AGENT_NAME;
}

var viewEpoch=0,detachActive=null,busy=false;
function ask(q){
  q=String(q||'').trim();
  if(!q||busy)return;
  busy=true;
  var epoch=viewEpoch;
  
  var u=document.createElement('div');u.className='mu-user';u.textContent=q;conv.appendChild(u);
  var byName=agentName();
  if(byName){var by=document.createElement('div');by.className='mu-by';by.textContent=byName;conv.appendChild(by);}
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
  input.value='';saveDraft();input.style.height='auto';
  var touchInput=window.matchMedia('(pointer: coarse)').matches;
  if(touchInput){input.blur();}else{input.focus({preventScroll:true});}

  var terminal=false,flowID='',recoveryThread='',completionTimer=null;
  var workLabel='Working';
  var t0=Date.now();
  var timer=null;
  var done=[];
  function renderWork(){
    if(terminal)return;
    var dots=['.','..','...'][Math.floor((Date.now()-t0)/450)%3];
    var secs=Math.round((Date.now()-t0)/1000);
    var past='';
    for(var i=Math.max(0,done.length-5);i<done.length;i++){
      past+='<div class="mu-step">'+esc(done[i])+'</div>';
    }
    a.innerHTML=past+'<div class="mu-think"><span class="mu-spin"></span><span>'+esc(workLabel)+dots+'</span>'+(secs>=1?'<span class="mu-think-t">'+secs+'s</span>':'')+'</div>';
  }
  function startWork(label){
    if(terminal)return;
    if(label&&label!==workLabel){
      if(workLabel&&workLabel!=='Working'&&done[done.length-1]!==workLabel)done.push(workLabel);
      workLabel=label;
    }
    renderWork();if(!timer)timer=setInterval(renderWork,450);
  }
  var running=0;
  function stopWork(){busy=false;if(timer){clearInterval(timer);timer=null;}}
  startWork('Working');

  save();
  toBottom(true);
  var streamText='';
  var body=JSON.stringify({context:requestClientContext(),prompt:q,attachment:(!contextId?attachment:""),history:history.slice(-6),context_id:contextId||'',agent:(window.muActiveAgent||''),stream_text:true});
  var recoveryUntil=Date.now()+600000;
  function checkCompletion(){
    if(terminal||epoch!==viewEpoch)return;
    fetch('/agent/pending?thread='+encodeURIComponent(recoveryThread)+'&flow='+encodeURIComponent(flowID),
      {headers:{'Accept':'application/json'},credentials:'same-origin'})
      .then(function(r){return r.ok?r.json():null;})
      .then(function(d){
        if(terminal||epoch!==viewEpoch)return;
        if(d&&d.html){
          terminal=true;stopWork();clearTimeout(completionTimer);
          if(d.answer_html)a.innerHTML=d.answer_html;else a.innerHTML=d.html;
          if(typeof d.text==='string')history.push({prompt:q,answer:d.text});
          save();revealQuestion(u);streamController.abort();return;
        }
        if(d&&!d.waiting){
          terminal=true;stopWork();a.innerHTML='<div class="mu-err">'+esc(d.error||'The run stopped without returning an answer.')+'</div>';save();streamController.abort();return;
        }
        retryCompletion();
      }).catch(retryCompletion);
  }
  function retryCompletion(){
    if(terminal||epoch!==viewEpoch)return;
    if(Date.now()>recoveryUntil){terminal=true;stopWork();a.innerHTML='<div class="mu-err">No answer came back. Please try again.</div>';save();streamController.abort();return;}
    completionTimer=setTimeout(checkCompletion,3000);
  }
  var streamController=new AbortController();
  detachActive=function(){terminal=true;clearTimeout(completionTimer);stopWork();streamController.abort();};
  var requestHeaders={'Content-Type':'application/json','Accept':'text/event-stream'};
  var csrfCookie=(document.cookie||'').match(/(?:^|; )csrf_token=([^;]+)/);
  if(csrfCookie){try{requestHeaders['X-CSRF-Token']=decodeURIComponent(csrfCookie[1]);}catch(e){}}
  fetch('/agent',{signal:streamController.signal,method:'POST',headers:requestHeaders,body:body,credentials:'same-origin'})
  .then(function(resp){
    if(epoch!==viewEpoch)throw 'handled';
    if(resp.status===402||resp.status===429){
      return resp.json().catch(function(){return {};}).then(function(j){
        stopWork();var message=resp.status===402?'You need more credits to continue.':'Please wait a moment before trying again.';
        a.innerHTML='<div class="mu-err" role="status">'+esc(message)+(resp.status===402?' <a href="/wallet">View credits →</a>':'')+'</div>';input.value=q;saveDraft();save();throw 'handled';
      });
    }
    if(resp.status===403){
      return resp.text().then(function(raw){
        stopWork();input.value=q;saveDraft();
        var message='This request was refused. Refresh the page to update your sign-in state, then try again.';
        try{var refusal=JSON.parse(raw);if(typeof refusal.error==='string'&&refusal.error)message=refusal.error;}catch(e){}
        if(/csrf/i.test(message))message='Your sign-in security token has changed. Refresh the page, then try again.';
        a.innerHTML='<div class="mu-err" role="status">'+esc(message)+'</div>';save();throw 'handled';
      });
    }
    if(resp.status===401){
      return resp.json().catch(function(){return {};}).then(function(j){
        stopWork();
        var msg=esc(j.error||'Sign in to continue this conversation.');
        input.value=q;saveDraft();
        var returnTo=SESSION&&contextId?'/?session='+encodeURIComponent(contextId):'/';
        try{sessionStorage.setItem(contextId?'mu_chat_continue_draft:'+contextId:'mu_chat_draft:landing',q);}catch(e){}
        a.innerHTML='<div class="mu-cta">'+msg+' <a href="/login?redirect='+encodeURIComponent(returnTo)+'">Sign in to continue →</a></div>';
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
        if(epoch!==viewEpoch){stopWork();reader.cancel();return;}
        if(chunk.done){stopWork();if(!terminal)throw Error('Response connection closed before the answer arrived');save();return;}
        buf+=decoder.decode(chunk.value,{stream:true});
        var lines=buf.split('\n');
        buf=lines.pop();
        lines.forEach(function(line){
          if(line.indexOf('data: ')!==0)return;
          try{
            var ev=JSON.parse(line.slice(6));
            if(terminal)return;
            if(ev.type==='flow_id'){
              flowID=ev.flow_id||'';recoveryThread=ev.thread||'';
              if(ev.thread&&!completionTimer)completionTimer=setTimeout(checkCompletion,4000);
              var id=ev.thread||ev.flow_id;
              if(id){
                var fresh=!contextId;
                contextId=id;save();
                window.dispatchEvent(new CustomEvent('mu-chat-thread',{detail:id}));
                if(fresh&&window.muSessionStarted)window.muSessionStarted(id,q);
              }
            }else if(ev.type==='working'){
              startWork(ev.message);
            }else if(ev.type==='tool_start'){
              running++;
              startWork(ev.message||'Working');
            }else if(ev.type==='tool_done'){
              if(running>0)running--;
              if(running===0)startWork('Working');
            }else if(ev.type==='stream_start'){
              streamText='';
            }else if(ev.type==='stream_token'){
              if(timer){clearInterval(timer);timer=null;}
              streamText+=ev.text||ev.token||'';
              a.textContent=streamText;toBottom(false);
            }else if(ev.type==='response'){
              terminal=true;clearTimeout(completionTimer);stopWork();
              a.innerHTML=ev.html;
              if(typeof ev.text==='string')streamText=ev.text;
              history.push({prompt:q,answer:streamText,results:ev.results||[]});
              save();
              revealQuestion(u);
            }else if(ev.type==='error'){
              terminal=true;clearTimeout(completionTimer);stopWork();
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
    if(terminal||err==='handled'||epoch!==viewEpoch)return;
    if(recoveryThread&&flowID){
      busy=true;
      a.innerHTML='<div class="mu-think"><span class="mu-spin"></span><span>Reconnecting...</span></div>';
      clearTimeout(completionTimer);checkCompletion();return;
    }
    a.innerHTML='<div class="mu-err">Error: '+esc(err&&err.message||err)+'</div>';save();
  });
}

window.muChatAsk=ask;
form.addEventListener('submit',function(e){e.preventDefault();ask(input.value);});

window.muChatNew=function(){
  busy=false;viewEpoch++;if(detachActive){detachActive();detachActive=null;}
  try{sessionStorage.removeItem(draftKey());sessionStorage.removeItem(scrollKey());}catch(e){}
  conv.innerHTML='';history=[];contextId='';input.value='';
  window.dispatchEvent(new CustomEvent('mu-chat-new'));
  try{sessionStorage.removeItem(CKEY);sessionStorage.removeItem(HKEY);sessionStorage.removeItem(TKEY);sessionStorage.removeItem(DKEY);}catch(e){}
  
  input.focus({preventScroll:true});
};

function watchPending(){
  busy=true;
  var pendingID=contextId;
  var a=document.createElement('div');a.className='mu-agent';conv.appendChild(a);
	var t0=Date.now(),timer=null,progress=[],current='Working';
  function draw(){
    var dots=['.','..','...'][Math.floor((Date.now()-t0)/450)%3];
    var secs=Math.round((Date.now()-t0)/1000);
		var past='';for(var i=Math.max(0,progress.length-5);i<progress.length;i++)past+='<div class="mu-step">'+esc(progress[i])+'</div>';
		a.innerHTML=past+'<div class="mu-think"><span class="mu-spin"></span><span>'+esc(current)+dots+
      '</span>'+(secs>=1?'<span class="mu-think-t">'+secs+'s</span>':'')+'</div>';
  }
  draw();timer=setInterval(draw,450);
  function done(html){
    busy=false;
    if(timer){clearInterval(timer);timer=null;}
    if(html){a.outerHTML=html;}else{a.remove();}
    toBottom(false);
  }

  var every=3000,giveUp=Date.now()+600000;
  function poll(){
    if(contextId!==pendingID||!a.isConnected){clearInterval(timer);return;}
    fetch('/agent/pending?thread='+encodeURIComponent(pendingID),
      {headers:{'Accept':'application/json'},credentials:'same-origin'})
      .then(function(r){return r.ok?r.json():null})
      .then(function(d){
        if(contextId!==pendingID||!a.isConnected){clearInterval(timer);return;}
        if(!d){done('');return;}
        if(d.html){done(d.html);return;}
        if(Array.isArray(d.steps)){
          progress=[];current='Working';
          d.steps.forEach(function(s){if(s.status==='running')current=s.label;else progress.push(s.label);});
          draw();
        }
        if(!d.waiting){
          done(d.error?'<div class="mu-agent"><div class="card mu-err">'+esc(d.error)+'</div></div>':'');return;
        }
        if(Date.now()>giveUp){
          done('<div class="mu-agent"><div class="card">No answer came back. '+
            'The run may have stopped when the server restarted — ask again.</div></div>');
          return;
        }
        setTimeout(poll,every);
      })
      .catch(function(){setTimeout(poll,every);});
  }
  setTimeout(poll,every);
}
if(PENDING&&contextId&&conv)watchPending();

var restoredScroll=null;
try{if(PERSIST)restoredScroll=JSON.parse(sessionStorage.getItem(scrollKey()));}catch(e){}
var pinned=!restoredScroll||restoredScroll.bottom;
if(conv){
  conv.addEventListener('scroll',function(){ if(!nearBottom()) pinned=false; });
}
function pin(){ if(pinned) toBottom(true); }
fitConv();
if(restoredScroll&&!restoredScroll.bottom){
  requestAnimationFrame(function(){conv.scrollTop=restoredScroll.top;});
  window.addEventListener("load",function(){conv.scrollTop=restoredScroll.top;});
}else{toBottom(true);}
window.addEventListener("pagehide",function(){
  saveDraft();
  if(PERSIST&&conv){try{sessionStorage.setItem(scrollKey(),JSON.stringify({top:conv.scrollTop,bottom:nearBottom()}));}catch(e){}}
});
window.addEventListener('load',function(){ fitConv(); pin(); });
if(conv&&window.ResizeObserver){ new ResizeObserver(pin).observe(conv); }
window.addEventListener('resize',function(){ fitConv(); toBottom(false); });
if(window.visualViewport){
  var onView=function(){ fitConv(); toBottom(false); };
  window.visualViewport.addEventListener('resize',onView);
  window.visualViewport.addEventListener('scroll',onView);
}
if(input) input.addEventListener('input',function(){fitConv();saveDraft();});
})();


// Result actions use the existing Bookmarks service without leaving the conversation.
document.addEventListener('click',async function(e){
 var button=e.target.closest('[data-save-url]');if(!button)return;
 var status=button.parentElement.querySelector('[role=status]');button.disabled=true;
 try{
  var body=new URLSearchParams({action:'add',url:button.dataset.saveUrl,title:button.dataset.saveTitle});
  var token=(document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)||[])[1]||'';
  var r=await fetch('/bookmarks',{method:'POST',body:body,headers:{Accept:'application/json','X-CSRF-Token':decodeURIComponent(token)}});
  if(r.status===401){status.innerHTML='<a href="/login?redirect=%2F">Sign in to save</a>';return;}
  if(!r.ok)throw Error('Could not save. Try again.');
  button.textContent='Saved';status.textContent='Saved to Bookmarks';
 }catch(e){status.textContent=e.message;}finally{button.disabled=false;}
});
