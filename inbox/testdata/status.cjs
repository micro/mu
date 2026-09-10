const vm = require('node:vm');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const code = fs.readFileSync('status.js', 'utf8');
function setup(restored) {
 const document = {activeElement: null};
 function element(value) { return {value, hidden: false, textContent: '', listeners: {}, addEventListener(k, fn) {this.listeners[k] = fn;}, focus() {document.activeElement = this;}}; }
 const label = element(''), input = element('Working'), feedback = element(''), edit = element('');
 input.hidden = true;
 const root = {dataset: {csrf: 'token'}, querySelector(s) {return s.includes('label') ? label : s.includes('input') ? input : s.includes('edit') ? edit : feedback;}};
 if (restored) {
 root.dataset.statusSaved = restored.saved;
 input.value = restored.draft;
 input.hidden = false;
 input.readOnly = true;
 label.hidden = true;
 }
 document.getElementById = () => root;
 const calls = [];
 let resolve;
 vm.runInNewContext(code, {document, URLSearchParams, AbortController, setTimeout, clearTimeout, fetch: (url, opts) => {calls.push({url, opts}); return new Promise(r => resolve = r);}});
 return {label, edit, input, feedback, calls, reply: r => resolve(r), key: key => input.listeners.keydown({key, preventDefault(){}})};
}
(async () => {
 let s = setup();
 assert.equal(s.calls.length, 0);
 s.edit.listeners.click(); assert.equal(s.input.hidden, false); assert.equal(s.edit.hidden, true);
 s.input.value = 'Cancelled'; s.key('Escape'); await s.input.listeners.blur();
 assert.equal(s.calls.length, 0); assert.equal(s.input.value, 'Working');
 s.label.listeners.click(); s.input.value = 'New status'; s.key('Enter');
 await s.input.listeners.blur(); assert.equal(s.calls.length, 1);
 assert.equal(new URLSearchParams(s.calls[0].opts.body).get('status'), 'New status');
 s.reply({ok:true,json:async()=>({status:'New status'})});
 await new Promise(setImmediate);
 assert.equal(s.label.textContent, '“New status”'); assert.equal(s.input.hidden, true);
 s.label.listeners.click(); s.input.value = ''; const cleared = s.input.listeners.blur();
 s.reply({ok:true,json:async()=>({status:''})}); await cleared;
 assert.equal(s.label.textContent, 'Set status');
 s.label.listeners.click(); s.input.value = 'Keep my draft'; const failed = s.input.listeners.blur();
 s.reply({ok:false}); await failed;
 assert.equal(s.input.value, 'Keep my draft'); assert.equal(s.input.hidden, false);
 assert.match(s.feedback.textContent, /Could not save/);
 const retry = s.input.listeners.blur(); s.reply({ok:true,json:async()=>({status:'Keep my draft'})}); await retry;
 assert.equal(s.label.textContent, '“Keep my draft”');
 assert.equal(s.input.defaultValue, 'Keep my draft');
 s = setup({saved:'Previous', draft:'Pending draft'});
 assert.equal(s.input.readOnly, false);
 assert.match(s.feedback.textContent, /interrupted/);
 const resumed = s.input.listeners.blur();
 assert.equal(s.calls.length, 1);
 s.reply({ok:true,json:async()=>({status:'Pending draft'})}); await resumed;
 assert.equal(s.input.defaultValue, 'Pending draft');
 s.label.listeners.click(); assert.equal(s.input.value, 'Pending draft');
 s.input.value = 'Another draft'; s.input.listeners.input();
 assert.equal(s.input.defaultValue, 'Another draft');
 s.key('Escape'); assert.equal(s.input.defaultValue, 'Pending draft');
 console.log('Status lifecycle passed');
})().catch(e => {console.error(e); process.exitCode = 1;});
