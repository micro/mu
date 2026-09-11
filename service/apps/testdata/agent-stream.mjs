import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
const sources=JSON.parse(readFileSync(process.argv[2],'utf8'));
const script=s=>s.replace(/^<script>\s*/, '').replace(/<\/script>\s*$/, '');
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function bridge(fetcher,confirm=()=>true){
  const messages=[],listeners={},frameListeners={},calls=[];
  const win={postMessage:m=>messages.push(m)};
  const frame={contentWindow:win,addEventListener:(name,fn)=>frameListeners[name]=fn};
  const access={hidden:true},revoke={addEventListener:(name,fn)=>listeners.revoke=fn};
  const context={Map,Number,AbortController,TextDecoder,setTimeout,clearTimeout,
    document:{getElementById:id=>id==='app-frame'?frame:id==='app-agent-access'?access:revoke,cookie:'csrf_token=secret'},
    window:{confirm,addEventListener:(name,fn)=>listeners[name]=fn},
    fetch:(path,init)=>{calls.push({path,init});return fetcher(path,init)}};
  vm.runInNewContext(script(sources.bridge),context);
  return {messages,calls,frameListeners,listeners,win,access,document:context.document,
    call:(id=1,op='agent.stream',source=win)=>listeners.message({source,data:{mu:'call',id,op,args:{body:{prompt:'Hello',context_id:'conversation'}}}})};
}
const wire=events=>events.map(e=>'data: '+JSON.stringify(e)+'\r\n\r\n').join('');
const identity={type:'flow_id',flow_id:'run',thread:'conversation'};
const final={type:'response',text:'Hello 🌍',flow_id:'run'};
const done={type:'done'};
{
  let enqueue,finish;
  const b=bridge(()=>new Response(new ReadableStream({start(c){enqueue=v=>c.enqueue(v);finish=()=>c.close()}}),{headers:{'Content-Type':'text/event-stream'}}));
  b.call(9,'agent.stream',{});
  assert.equal(b.calls.length,0,'unrelated window must not start work');
  b.call();await tick();
  b.frameListeners.load();
  assert.equal(b.calls[0].init.signal.aborted,false,'initial load must preserve a startup request');
  const bytes=new TextEncoder().encode(': heartbeat\r\n\r\n'+wire([identity,{type:'stream_token',text:'Hello 🌍'}]));
  for(const byte of bytes) enqueue(Uint8Array.of(byte));
  await tick();await tick();
  assert.equal(b.messages.at(-1).event.text,'Hello 🌍','deltas arrive before completion, including split UTF-8');
  assert.equal(b.messages.some(m=>m.mu==='reply'),false);
  enqueue(new TextEncoder().encode(wire([final,done])));finish();await tick();
  assert.deepEqual(JSON.parse(JSON.stringify(b.messages.at(-1).result)),{answer:'Hello 🌍',flow_id:'run',thread:'conversation'});
  assert.equal(b.calls[0].path,'/agent');
  assert.equal(b.calls[0].init.headers['X-CSRF-Token'],'secret');
  assert.equal(JSON.parse(b.calls[0].init.body).stream_text,true);
  assert.equal(JSON.parse(b.calls[0].init.body).context_id,'conversation');
  assert.equal(JSON.stringify(b.messages).includes('secret'),false);
}
for(const [name,response,expected] of [
  ['HTTP failure',()=>new Response('{"error":"Sign in"}',{status:401}),'Sign in'],
  ['JSON instead of SSE',()=>new Response('{}'),'did not return'],
  ['truncated stream',()=>new Response(wire([identity,final]),{headers:{'Content-Type':'text/event-stream'}}),'before the answer was complete'],
  ['server failure',()=>new Response(wire([{type:'error',message:'No provider'},done]),{headers:{'Content-Type':'text/event-stream'}}),'No provider'],
  ['malformed event',()=>new Response('data: nope\n\n',{headers:{'Content-Type':'text/event-stream'}}),'JSON'],
]){
  const b=bridge(response);b.call();await tick();await tick();
  assert.ok(b.messages.at(-1).error.includes(expected),name+': '+JSON.stringify(b.messages));
  assert.equal(b.calls.length,1,'a failed stream must never rerun the prompt');
}
for(const action of ['load','pagehide','cancel','revoke']){
  const b=bridge((path,init)=>new Promise((resolve,reject)=>init.signal.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError')))));
  b.call();b.call();assert.equal(b.calls.length,1,'duplicate request IDs must not start duplicate work');
  if(action==='load'){b.frameListeners.load();b.frameListeners.load();}
  if(action==='pagehide') b.listeners.pagehide();
  if(action==='revoke') b.listeners.revoke();
  if(action==='cancel') b.listeners.message({source:b.win,data:{mu:'cancel',id:1}});
  await tick();
  assert.equal(b.calls[0].init.signal.aborted,true);
  assert.equal(b.messages.length,0,'a detached/cancelled frame must receive no late result');
}
{
  const posted=[],listeners={};
  const parent={postMessage:m=>posted.push(m)};
  const ctx={parent,Promise,Error,setTimeout,clearTimeout,addEventListener:(name,fn)=>listeners[name]=fn};
  ctx.window=ctx;
  vm.createContext(ctx);vm.runInContext(script(sources.shim),ctx);
  const events=[];
  const result=ctx.mu.agent.stream('Hello',{context_id:'conversation',onEvent:e=>events.push(e)});
  const request=posted.at(-1);
  assert.equal(request.op,'agent.stream');
  assert.equal(request.args.body.context_id,'conversation');
  listeners.message({source:{},data:{mu:'reply',id:request.id,result:'forged'}});
  listeners.message({source:parent,data:{mu:'event',id:request.id,event:{type:'stream_token',text:'Hello'}}});
  assert.equal(events.length,1);
  listeners.message({source:parent,data:{mu:'reply',id:request.id,result:{answer:'Hello',thread:'conversation'}}});
  assert.equal((await result).answer,'Hello');
  const old=ctx.mu.agent('Hello');const oldRequest=posted.at(-1);
  assert.equal(oldRequest.op,'agent');
  listeners.message({source:parent,data:{mu:'reply',id:oldRequest.id,result:{answer:'Unchanged'}}});
  assert.equal(await old,'Unchanged');
  const failed=ctx.mu.agent.stream('Hello',{onEvent:()=>{throw new Error('callback failed')}});
  const bad=posted.at(-1);
  listeners.message({source:parent,data:{mu:'event',id:bad.id,event:{type:'stream_token',text:'Hello'}}});
  await assert.rejects(failed,/callback failed/);
  assert.equal(posted.at(-1).mu,'cancel');
}

// Untrusted app requests cannot authorize themselves, including the legacy
// synchronous agent path and generic service dispatch.
for (const op of ['agent', 'agent.stream', 'chat', 'blog.create', 'user', 'sdk:service', 'sdk:ai', 'sdk:fetch', 'places.search', 'places.nearby']) {
  const prompts=[];
  const b=bridge(()=>{throw new Error('unauthorized fetch')}, text=>{prompts.push(text);return false});
  b.call(1,op); await tick();
  assert.equal(b.calls.length,0,op);
  assert.equal(prompts.length,1,op);
  assert.match(prompts[0],/Hello/);
  assert.match(b.messages.at(-1).error,/declined/);
  b.call(2,op,{});
  assert.equal(prompts.length,1,'unrelated frames cannot prompt');
}
{
 const b=bridge(()=>Promise.resolve(new Response('{}')));
 b.call(1,'blog.list'); await tick();
 assert.equal(b.calls[0].init.credentials,'omit','public reads cannot borrow the viewer');
}

// A page grant covers only agent requests and remains under parent control.
{
 let prompts=0;
 const b=bridge(()=>Promise.resolve(new Response('{}')),()=>{prompts++;return true});
 b.call(1,'agent'); b.call(2,'agent'); await tick();
 assert.equal(prompts,1);
 assert.equal(b.access.hidden,false);
 b.call(3,'sdk:service'); await tick(); assert.equal(prompts,2);
 b.listeners.revoke(); assert.equal(b.access.hidden,true);
 b.call(4,'agent'); await tick(); assert.equal(prompts,3);
 b.frameListeners.load(); b.frameListeners.load();
 b.call(5,'agent'); await tick(); assert.equal(prompts,4);
 b.listeners.pagehide();
 b.call(6,'agent'); await tick(); assert.equal(prompts,5);
 const other=bridge(()=>Promise.resolve(new Response('{}')),()=>{prompts++;return true});
 other.call(1,'agent'); await tick(); assert.equal(prompts,6);
}

{
 const b=bridge(()=>Promise.resolve(new Response('{}')));
 b.call(1,'agent'); await tick();
 b.document.cookie='csrf_token=another-session';
 b.call(2,'agent'); await tick();
 assert.equal(b.calls.length,1,'page consent cannot follow an account switch');
 assert.match(b.messages.at(-1).error,/sign-in changed/);
 assert.equal(b.access.hidden,true);
}
