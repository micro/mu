// Included inside the sandbox parent's bridge, after frame and j are defined.
var streams=new Map();
function cancelStream(id){
  var s=streams.get(id);
  if(s){streams.delete(id);s.abort();}
}
function cancelStreams(){streams.forEach(function(s){s.abort()});streams.clear();}
var frameLoaded=false;
if(frame) frame.addEventListener('load',function(){
  // An inline app script can start work before its first load event. That
  // event finishes initialization; only subsequent loads leave a document.
  if(frameLoaded) revokeAgent();
  frameLoaded=true;
});
window.addEventListener('pagehide',revokeAgent);

async function streamAgent(win,id,path,body,init){
  if(!Number.isSafeInteger(id)||streams.has(id)) return;
  var controller=new AbortController();
  streams.set(id,controller);
  var timer=setTimeout(function(){controller.abort()},360000);
  var reader;
  function current(){return streams.get(id)===controller;}
  try{
    init.signal=controller.signal;
    init.headers.Accept='text/event-stream';
    // The same fixed endpoint and authenticated write gate as the web client.
    init.body=JSON.stringify({prompt:body.prompt,context_id:body.context_id||'',stream_text:true});
    var response=await fetch(path,init);
    if(!response.ok){
      var failure=await response.json().catch(function(){return {}});
      throw new Error(failure.error||('Request failed ('+response.status+')'));
    }
    if(!(response.headers.get('Content-Type')||'').includes('text/event-stream')||!response.body){
      throw new Error('The server did not return an agent stream.');
    }
    reader=response.body.getReader();
    var decoder=new TextDecoder(),buffer='',lines=[],result=null,done=false,flow='',thread='';
    function dispatch(){
      if(!lines.length) return;
      var event=JSON.parse(lines.join('\n'));lines=[];
      if(event.type==='error') throw new Error(event.message||'The agent could not finish.');
      if(event.type==='flow_id'){flow=event.flow_id||'';thread=event.thread||'';}
      if(event.type==='response') result={answer:event.text,flow_id:event.flow_id||flow,thread:thread};
      if(event.type==='done'){done=true;return;}
      if(['flow_id','working','tool_start','tool_done','stream_token','response'].includes(event.type)&&current()){
        win.postMessage({mu:'event',id:id,event:event},'*');
      }
    }
    function consume(text,eof){
      buffer+=text;
      var pos;
      while((pos=buffer.indexOf('\n'))!==-1){
        var line=buffer.slice(0,pos).replace(/\r$/,'');buffer=buffer.slice(pos+1);
        if(line==='') dispatch();
        else if(line.startsWith('data:')) lines.push(line.slice(5).replace(/^ /,''));
        if(done) return;
      }
      if(buffer.length>1048576) throw new Error('Agent event exceeds the size limit.');
      if(eof&&buffer.trim()) throw new Error('The agent connection ended inside an event.');
    }
    while(!done){
      var chunk=await reader.read();
      if(!current()) return;
      consume(decoder.decode(chunk.value,{stream:!chunk.done}),chunk.done);
      if(chunk.done) break;
    }
    if(!done||!result||typeof result.answer!=='string'){
      throw new Error('The connection ended before the answer was complete. Check your conversation before trying again.');
    }
    if(current()) reply(win,id,result,null);
  }catch(err){
    if(current()) reply(win,id,null,err.name==='AbortError'
      ?'Connection timed out. The request may still be running; check your conversation before trying again.'
      :String(err.message||err));
  }finally{
    clearTimeout(timer);
    if(reader){try{await reader.cancel()}catch(ignore){}}
    if(current()) streams.delete(id);
  }
}
