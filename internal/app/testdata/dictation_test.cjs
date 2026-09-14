const fs=require('fs'),vm=require('vm'),assert=require('assert');
const handlers={};let recognizer,submits=0;
const listen=(key,fn)=>(handlers[key]??=[]).push(fn);
const fire=(key,event={})=>(handlers[key]||[]).forEach(f=>f(event));
const input={value:'Find for me',selectionStart:5,selectionEnd:5,maxLength:8000,addEventListener:listen,dispatchEvent:e=>fire(e.type,e)};
const mic={hidden:true,setAttribute(){}};const status={textContent:''};
class Recognition {constructor(){recognizer=this}start(){this.onstart()}stop(){this.stopped=true}}
const doc={getElementById:id=>({'mu-chat-mic':mic,'mu-chat-input':input,'mu-chat-voice-status':status,'mu-chat-form':{addEventListener:listen}}[id]),addEventListener:listen};
const context={document:doc,window:{SpeechRecognition:Recognition,addEventListener:listen},navigator:{language:'en-GB'},Event:class{constructor(type){this.type=type}}};
vm.runInNewContext(fs.readFileSync('dictation.js','utf8'),context);
listen('submit',()=>submits++);
assert(!mic.hidden);mic.onclick();recognizer.onresult({results:[[{transcript:'Arabic fruits '}]]});
assert.equal(input.value,'Find Arabic fruits for me');assert.equal(submits,0,'dictation submitted a message');
fire('beforeinput');input.value='Edited by hand';recognizer.onresult({results:[[{transcript:'late words'}]]});
assert.equal(input.value,'Edited by hand','late dictation overwrote an edit');recognizer.onend();
input.selectionStart=input.selectionEnd=input.value.length;mic.onclick();fire('submit');input.value='';recognizer.onresult({results:[[{transcript:'late after send'}]]});assert.equal(input.value,'');recognizer.onend();
mic.onclick();recognizer.onerror({error:'not-allowed'});assert(status.textContent.includes('permission'));assert.equal(submits,1);
console.log('Dictation preserves edits, does not auto-send, and handles denied permission.');
