(()=>{'use strict';
const form=document.querySelector('#command-form'),input=document.querySelector('#command-input'),log=document.querySelector('#responses'),send=document.querySelector('#send'),status=document.querySelector('#status');
let busy=false,thread=new URLSearchParams(location.search).get('session')||new URLSearchParams(location.search).get('continue')||'';
function remember(){if(thread)history.replaceState(null,'','/?session='+encodeURIComponent(thread));}
function headers(accept){const token=(document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)||[])[1]||'';return {'Content-Type':'application/json','Accept':accept,'X-CSRF-Token':decodeURIComponent(token)};}
async function assistant(command,answer){
 const response=await fetch('/agent',{method:'POST',credentials:'same-origin',headers:headers('text/event-stream'),body:JSON.stringify({prompt:command,context_id:thread,stream_text:true})});
 if(!response.ok)throw Error(response.status===401?'Log in to ask the assistant.':await failure(response));
 const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',done=false,text='';
 function event(line){if(!line.startsWith('data: '))return;const e=JSON.parse(line.slice(6));if(e.type==='flow_id'){thread=e.thread||thread;remember();}if(e.type==='working'||e.type==='tool_start')status.textContent=e.message||'Working…';if(e.type==='stream_token'){text+=e.text||e.token||'';answer.textContent=text;}if(e.type==='response'){answer.innerHTML=e.html;done=true;}if(e.type==='error'){throw Error(e.message||'Request failed.');}}
 try{while(true){const chunk=await reader.read();buffer+=decoder.decode(chunk.value||new Uint8Array(),{stream:!chunk.done});const lines=buffer.split('\n');buffer=lines.pop();for(const line of lines)event(line);if(chunk.done){if(buffer)event(buffer);break;}}}finally{reader.releaseLock();}
 if(!done)throw Error('The connection closed before the response completed. Your request may still be running; check inbox before submitting it again.');
}
async function failure(response){try{const j=await response.json();return typeof j.error==='string'?j.error:(j.error?.message||'Request failed.');}catch{return 'Request failed ('+response.status+').';}}
async function run(command){
 if(busy||!command.trim())return;busy=true;send.disabled=true;status.textContent='Working…';input.value='';
 const turn=document.createElement('section');turn.className='turn';const q=document.createElement('div');q.className='request';
 // Administrative values are neither displayed nor persisted in browser history.
 q.textContent=/^admin\s+config\s+set\s/i.test(command)?command.trim().split(/\s+/).slice(0,4).join(' ')+' [value hidden]':command;
 const answer=document.createElement('div');answer.className='answer';turn.append(q,answer);log.append(turn);
 try{const response=await fetch('/command',{method:'POST',credentials:'same-origin',headers:headers('application/json'),body:JSON.stringify({command,thread})});if(!response.ok)throw Error(await failure(response));const data=await response.json();if(data.assistant)await assistant(command,answer);else {answer.innerHTML=data.html;if(data.thread){thread=data.thread;remember();}}status.textContent='';}catch(error){answer.textContent=error.message;answer.classList.add('error');status.textContent='Request stopped.';}finally{busy=false;send.disabled=false;input.focus();}
}
form.addEventListener('submit',e=>{e.preventDefault();run(input.value.trim());});
input.addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();form.requestSubmit();}});
document.querySelectorAll('[data-command]').forEach(button=>button.addEventListener('click',()=>run(button.dataset.command)));
try{if(location.hash){input.value=decodeURIComponent(location.hash.slice(1));history.replaceState(null,'','/');}}catch{}
})();
