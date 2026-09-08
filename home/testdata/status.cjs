const vm = require('node:vm');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const code = fs.readFileSync('status.js', 'utf8');
function setup() {
 const document = {activeElement: null};
 function element(value) { return {value, hidden: false, textContent: '', listeners: {}, addEventListener(k, fn) {this.listeners[k] = fn;}, focus() {document.activeElement = this;}}; }
 const label = element(''), input = element('Working'), feedback = element('');
 input.hidden = true;
 const root = {dataset: {csrf: 'token'}, querySelector(s) {return s.includes('label') ? label : s.includes('input') ? input : feedback;}};
 document.getElementById = () => root;
 const calls = [];
 let resolve;
 vm.runInNewContext(code, {document, URLSearchParams, AbortController, setTimeout, clearTimeout, fetch: (url, opts) => {calls.push({url, opts}); return new Promise(r => resolve = r);}});
 return {label, input, feedback, calls, reply: r => resolve(r), key: key => input.listeners.keydown({key, preventDefault(){}})};
}
(async () => {
 let s = setup();
 assert.equal(s.calls.length, 0);
 s.label.listeners.click(); assert.equal(s.input.hidden, false);
 s.input.value = 'Cancelled'; s.key('Escape'); await s.input.listeners.blur();
 assert.equal(s.calls.length, 0); assert.equal(s.input.value, 'Working');
 s.label.listeners.click(); s.input.value = 'New status'; s.key('Enter');
 await s.input.listeners.blur(); assert.equal(s.calls.length, 1);
 assert.equal(new URLSearchParams(s.calls[0].opts.body).get('status'), 'New status');
 s.reply({ok:true,json:async()=>({status:'New status'})});
 await new Promise(setImmediate);
 assert.equal(s.label.textContent, 'New status'); assert.equal(s.input.hidden, true);
 s.label.listeners.click(); s.input.value = ''; const cleared = s.input.listeners.blur();
 s.reply({ok:true,json:async()=>({status:''})}); await cleared;
 assert.equal(s.label.textContent, 'What are you up to?');
 s.label.listeners.click(); s.input.value = 'Keep my draft'; const failed = s.input.listeners.blur();
 s.reply({ok:false}); await failed;
 assert.equal(s.input.value, 'Keep my draft'); assert.equal(s.input.hidden, false);
 assert.match(s.feedback.textContent, /Could not save/);
 const retry = s.input.listeners.blur(); s.reply({ok:true,json:async()=>({status:'Keep my draft'})}); await retry;
 assert.equal(s.label.textContent, 'Keep my draft');
 console.log('Status lifecycle passed');
})().catch(e => {console.error(e); process.exitCode = 1;});
