const fs = require('node:fs'), vm = require('node:vm'), assert = require('node:assert/strict');
const source = fs.readFileSync('html/mu.js', 'utf8');
const scheduler = source.slice(source.indexOf('function muPoll('), source.indexOf('// Update only message/list regions.'));
const inbox = source.slice(source.indexOf('// Update only message/list regions.'), source.indexOf('// The remaining controls run in pages'));
const activity = source.slice(source.indexOf('// One live work projection'), source.indexOf("if (typeof document !== 'undefined') {\n  const initReferenceIndex"));
function harness() {
  let now = 0, serial = 0;
  const timers = new Map(), events = {};
  const on = (name, fn) => (events[name] ||= []).push(fn);
  const emit = (name, extra = {}) => (events[name] || []).forEach(fn => fn(extra));
  const context = {
    document: {hidden: false, addEventListener: on, dispatchEvent: e => emit(e.type)},
    window: {addEventListener: on}, navigator: {onLine: true},
    setTimeout(fn, delay) { const id = ++serial; timers.set(id, {fn, at: now + delay}); return id; },
    clearTimeout: id => timers.delete(id), Event: class { constructor(type) { this.type = type; } },
    AbortSignal, console,
  };
  vm.createContext(context); vm.runInContext(scheduler, context);
  async function settle() { for (let i = 0; i < 30; i++) await Promise.resolve(); }
  async function advance(ms) {
    const end = now + ms;
    for (;;) {
      const next = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
      if (!next || next[1].at > end) break;
      now = next[1].at; timers.delete(next[0]); next[1].fn(); await settle();
    }
    now = end; await settle();
  }
  return {context, emit, advance, settle, timers};
}
(async () => {
  // Execute the actual inbox and activity loops together, with a one-hour clock.
  const h = harness(), c = h.context;
  let pending = false, state = '', inboxCalls = 0, agentCalls = 0;
  const region = {children: [], contains: () => false, querySelector: () => null, replaceChildren(...nodes) { this.children = nodes; }};
  const status = {textContent: ''};
  const root = {dataset: {conversationActivity: '/agent?view=activity-status&thread=test'}, isConnected: true,
    contains: () => false, querySelector: () => null, replaceChildren() {}};
  const reader = {querySelector(q) {
    if (q === '[data-inbox-messages]') return region;
    if (q === '[data-inbox-state]') return status;
    if (q === '[data-work-pending="true"]') return root.dataset.workPending === 'true' ? root : null;
    return null;
  }};
  c.document.querySelector = q => q === '[data-inbox-watch]' ? reader : null;
  c.document.querySelectorAll = () => [root];
  c.window.getSelection = () => '';
  c.window.scrollTo = () => {};
  c.location = {href: '/inbox?id=test'};
  c.DOMParser = class { parseFromString() { return {querySelector: q => q === '[data-inbox-state]' ? {textContent: state} : region}; } };
  c.fetch = async url => {
    if (url.startsWith('/agent')) { agentCalls++; return {ok: true, json: async () => ({pending, deliveries: [], html: ''})}; }
    inboxCalls++; return {ok: true, headers: {get: () => 'text/html'}, text: async () => ''};
  };
  vm.runInContext(inbox + '\n' + activity, c);
  await h.advance(3600000);
  assert.equal(inboxCalls, 60); assert.equal(agentCalls, 61);
  pending = true; c.document.dispatchEvent(new c.Event('mu:conversation')); await h.settle();
  const activeInbox = inboxCalls, activeAgent = agentCalls;
  await h.advance(15000);
  assert(inboxCalls >= activeInbox + 5); assert.equal(agentCalls, activeAgent + 3);
  pending = false; await h.advance(5000);
  const idleInbox = inboxCalls, idleAgent = agentCalls;
  await h.advance(59000);
  assert.equal(inboxCalls, idleInbox); assert.equal(agentCalls, idleAgent);
  c.document.hidden = true; h.emit('visibilitychange'); await h.advance(3600000);
  assert.equal(inboxCalls, idleInbox); assert.equal(agentCalls, idleAgent);
  c.document.hidden = false; h.emit('visibilitychange'); await h.settle();
  assert.equal(inboxCalls, idleInbox + 1); assert.equal(agentCalls, idleAgent + 1);
  c.navigator.onLine = false; h.emit('offline'); await h.advance(3600000);
  assert.equal(inboxCalls, idleInbox + 1);
  c.navigator.onLine = true; h.emit('online'); await h.settle();
  assert.equal(inboxCalls, idleInbox + 2);
  h.emit('pagehide'); await h.advance(3600000); assert.equal(inboxCalls, idleInbox + 2);
  h.emit('pageshow', {persisted: true}); await h.settle(); assert.equal(inboxCalls, idleInbox + 3);

  // A request taking longer than its interval cannot overlap or lose a resume.
  const slow = harness(); let calls = 0, finish;
  slow.context.refresh = () => { calls++; return new Promise(resolve => { finish = resolve; }); };
  vm.runInContext('muPoll(refresh, 0)', slow.context);
  await slow.advance(0); slow.emit('online'); await slow.advance(120000); assert.equal(calls, 1);
  finish(3000); await slow.settle(); await slow.advance(0); assert.equal(calls, 2);
  finish(60000); await slow.settle();

  // Failed active checks back off to one minute, then two, rather than spinning.
  const failed = harness(); let attempts = 0;
  failed.context.refresh = async () => { attempts++; throw Error('offline server'); };
  vm.runInContext('muPoll(refresh, 0)', failed.context);
  await failed.advance(0); await failed.advance(59999); assert.equal(attempts, 1);
  await failed.advance(1); assert.equal(attempts, 2);
  await failed.advance(119999); assert.equal(attempts, 2);
  await failed.advance(1); assert.equal(attempts, 3);
  console.log('PASS: hourly request budget, active/idle transitions, visibility, offline, bfcache, overlap and failure backoff');
})().catch(error => { console.error(error); process.exitCode = 1; });
