const fs=require('fs'),assert=require('assert');
const source=JSON.parse(fs.readFileSync(0,'utf8'));
const frame=e=>'data: '+JSON.stringify(e)+'\n\n';
async function scenario(frames,stall=false){
 const paints=[],children=[],timeouts=[];
 const element=()=>({className:'',style:{},isConnected:true,focus(){},scrollIntoView(){},set innerHTML(v){this.html=v;paints.push(v)},get innerHTML(){return this.html||''}});
 let idx=0,polls=0,scrolls=0;
 const env={viewEpoch:0,detachActive:null,hideBrief(){},sugDiv:element(),conv:{appendChild(e){children.push(e)}},document:{createElement:element},input:element(),saveDraft(){},save(){},esc:String,agentName(){return 'Micro'},toBottom(){},revealQuestion(node){assert(node.isConnected);assert.equal(node.className,"mu-user");scrolls++},transcript:false,history:[],contextId:'',attachment:'',window:{dispatchEvent(){}},CustomEvent:class{},TextDecoder,AbortController,
 setInterval(){return 1},clearInterval(){},setTimeout(fn,ms){timeouts.push({fn,ms});return timeouts.length},clearTimeout(){},
 fetch(url){
  if(url.startsWith('/agent/pending')){polls++;return Promise.resolve({ok:true,json:()=>Promise.resolve({waiting:false,html:'<div>Recovered</div>',answer_html:'Recovered',text:'Recovered'})})}
  return Promise.resolve({ok:true,status:200,body:{getReader(){return {read(){if(idx<frames.length)return Promise.resolve({value:new TextEncoder().encode(frames[idx++]),done:false});if(stall)return new Promise(()=>{});return Promise.resolve({done:true})},cancel(){}}}}})
 }};
 new Function('env','with(env){'+source+'; return ask;}')(env)('News');
 for(let i=0;i<100;i++)await Promise.resolve();
 if(stall){const timer=timeouts.find(t=>t.ms===4000);assert(timer,'missing live completion watchdog');timer.fn();for(let i=0;i<100;i++)await Promise.resolve();}
 if(env.history.length)assert(scrolls>0,"completed landing answer was not revealed");
 return {paints,answer:children[children.length-1],polls,history:env.history};
}
(async()=>{
 const id=frame({type:'flow_id',thread:'thread',flow_id:'flow'});
 let r=await scenario([id,frame({type:'stream_token',token:'partial words'}),frame({type:'response',html:'Complete answer',text:'Complete answer'}),frame({type:'tool_done'})]);
 assert.equal(r.answer.innerHTML,'Complete answer');assert.equal(r.history.length,1);assert(!r.paints.some(x=>x.includes('partial words')));assert.equal(r.polls,0);
 r=await scenario([id]);assert.equal(r.answer.innerHTML,'Recovered');assert.equal(r.history.length,1);assert.equal(r.polls,1);
 r=await scenario([id],true);assert.equal(r.answer.innerHTML,'Recovered');assert.equal(r.history.length,1);
 r=await scenario([id,'data: {"type":"response"']);assert.equal(r.answer.innerHTML,'Recovered');
 r=await scenario([id,frame({type:'error',message:'Failed'}),frame({type:'tool_done'})]);assert(r.answer.innerHTML.includes('Failed'));assert.equal(r.polls,0);
 console.log('completion scenarios passed');
})().catch(e=>{console.error(e);process.exitCode=1});
