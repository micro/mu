const assert=require('node:assert/strict');
const vm=require('node:vm');
const fs=require('node:fs');
const source=fs.readFileSync('html/wake.js','utf8');
function target(){return {events:{},addEventListener(k,f){this.events[k]=f},removeEventListener(k,f){if(this.events[k]===f)delete this.events[k]},dispatchEvent(e){if(this.events[e.type])this.events[e.type](e)},setAttribute(k,v){this[k]=v}}}
function setup(supported=true){
 const panel=Object.assign(target(),{hidden:true,isConnected:true,open:true}),button=target(),status=target(),box=Object.assign(target(),{value:'',maxLength:1024});
 let updates=0;box.events.input=()=>updates++;
 const nodes={'mu-chat-wake':panel,'mu-chat-wake-toggle':button,'mu-chat-wake-status':status,'mu-chat-input':box};
 const document=Object.assign(target(),{hidden:false,documentElement:{lang:'en-GB'},getElementById:id=>nodes[id]});
 const window=Object.assign(target(),{speechSynthesis:{speaking:false,pending:false},muSay(){throw Error('Speak must not be changed or invoked')},muChatAsk(){throw Error('Voice must not auto-send')}});
 const instances=[];
 class Recognition{constructor(){instances.push(this)}start(){this.started=true;if(this.onstart)this.onstart()}abort(){this.aborted=true;if(this.onend)this.onend()}result(text,final=true){if(this.onresult)this.onresult({resultIndex:0,results:[Object.assign([{transcript:text}],{isFinal:final})]})}}
 if(supported)window.SpeechRecognition=Recognition;
 let seq=0;const timers=new Map(),intervals=new Map();
 vm.runInNewContext(source,{window,document,Event:class{constructor(type){this.type=type}},Date,setTimeout(f,ms){timers.set(++seq,{f,ms});return seq},clearTimeout(id){timers.delete(id)},setInterval(f){intervals.set(++seq,f);return seq},clearInterval(id){intervals.delete(id)}});
 return {panel,button,status,box,window,document,instances,timers,intervals,updates:()=>updates,click:()=>button.events.click(),tick(ms){for(const [id,t] of [...timers])if(t.ms===ms){timers.delete(id);t.f()}},monitor(){for(const f of intervals.values())f()}};
}
let s=setup(false);assert.equal(s.panel.hidden,true);assert.equal(s.instances.length,0);
s=setup();assert.equal(s.panel.hidden,false);assert.equal(s.instances.length,0,'never listen on load');
s.click();assert.equal(s.button['aria-pressed'],'true');let r=s.instances[0];
r.result('weather in London');assert.equal(s.box.value,'','unaddressed speech ignored');
r.result('hey microphone weather');assert.equal(s.box.value,'','whole wake word required');
r.result('hey micro weather',false);assert.equal(s.box.value,'','interim wake ignored');
r.result('Hey Micro, weather in London tomorrow');assert.equal(s.box.value,'weather in London tomorrow');assert(r.aborted);assert.equal(s.updates(),1);
s.tick(1000);r=s.instances.at(-1);s.box.value='Please';r.result('Hey Micro');assert.equal(s.box.value,'Please');r.result('show the weather',false);assert.equal(s.box.value,'Please show the weather');r.result('show the weather tomorrow');assert.equal(s.box.value,'Please show the weather tomorrow');
s.click();assert.equal(s.button['aria-pressed'],'false');const count=s.instances.length;s.tick(1000);assert.equal(s.instances.length,count,'stop must cancel retries');
s=setup();s.click();r=s.instances[0];r.result('Hey Micro');s.tick(10000);assert(r.aborted);assert.equal(s.box.value,'','wake without request times out');
s=setup();s.click();r=s.instances[0];r.onerror({error:'not-allowed'});assert(r.aborted);assert.match(s.status.textContent,/denied/);assert.equal(s.timers.size,0);
s=setup();s.click();r=s.instances[0];s.document.hidden=true;s.document.events.visibilitychange();assert(r.aborted);r.result('Hey Micro send secrets');assert.equal(s.box.value,'');assert.equal(s.intervals.size,0);
s=setup();s.click();r=s.instances[0];s.panel.isConnected=false;s.document.events['mu:navigated']();assert(r.aborted);assert.equal(s.button.events.click,undefined);
s=setup();s.click();r=s.instances[0];s.window.speechSynthesis.speaking=true;r.result('Hey Micro repeat');assert.equal(s.box.value,'');s.monitor();assert(r.aborted);s.tick(1000);assert.equal(s.instances.length,1,'do not listen to spoken output');s.window.speechSynthesis.speaking=false;s.tick(1000);assert.equal(s.instances.length,2);
s=setup();s.click();r=s.instances[0];s.box.maxLength=8;r.result('Hey Micro a very long request');assert.equal(s.box.value,'');assert.match(s.status.textContent,/too long/);
s=setup();s.click();r=s.instances[0];s.window.events.pagehide();assert(r.aborted);assert.equal(s.timers.size,0);
// BFCache restores the same DOM and does not rerun scripts. Keep controls bound.
s=setup();s.click();r=s.instances[0];s.window.events.pagehide();assert(r.aborted);s.click();assert.equal(s.instances.length,2,'Start must still work after restoring the page');
// Providers can end after the wake phrase, before the next utterance.
s=setup();s.click();r=s.instances[0];r.result('Hey Micro');r.onend();s.tick(1000);r=s.instances.at(-1);r.result('weather tomorrow');assert.equal(s.box.value,'weather tomorrow');
// The capture deadline survives repeated end/restart cycles.
s=setup();s.click();r=s.instances[0];r.result('Hey Micro');r.onend();s.tick(1000);s.tick(10000);s.tick(1000);s.instances.at(-1).result('weather tomorrow');assert.equal(s.box.value,'');
// No hidden listening after closing the disclosure.
s=setup();s.click();r=s.instances[0];s.panel.open=false;s.panel.events.toggle();assert(r.aborted);assert.equal(s.button['aria-pressed'],'false');assert.equal(s.timers.size,0);assert.equal(s.intervals.size,0);
console.log('wake dictation lifecycle passed');
