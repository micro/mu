// Mu's focused browser client. All assistant data travels through MUCP.
const $ = id => document.getElementById(id);
const params = new URLSearchParams(location.search);
let recovering = false;
let agent, agents = [], thread = params.get('thread') || '', epoch = 0, busy = false, lastReply = '';
const connection = $('connection'), status = $('status');
const el = (tag, text, className) => {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
};
function safeURL(value) {
  try { const u = new URL(value, location.origin); return ['https:', 'http:'].includes(u.protocol) && !u.username && !u.password ? u.href : ''; } catch { return ''; }
}
function link(text, href, className) {
  const node = el('a', text, className), url = safeURL(href);
  if (url) { node.href = url; node.rel = 'noopener noreferrer'; }
  return node;
}
// A deliberately small Markdown renderer. Model text is never interpreted as HTML.
function inline(parent, text) {
  const pattern = /\[([^\]\n]+)\]\(([^\s)]+)\)|\*\*([^*\n]+)\*\*|`([^`\n]+)`/g;
  let end = 0;
  for (const m of text.matchAll(pattern)) {
    parent.append(document.createTextNode(text.slice(end, m.index)));
    parent.append(m[1] ? link(m[1], m[2]) : el(m[3] ? 'strong' : 'code', m[3] || m[4]));
    end = m.index + m[0].length;
  }
  parent.append(document.createTextNode(text.slice(end)));
}
function markdown(text) {
  const fragment = document.createDocumentFragment();
  let list, code, paragraph;
  for (const line of String(text || '').split('\n')) {
    if (line.startsWith('```')) {
      list = paragraph = null;
      if (code) code = null;
      else { const pre = el('pre'); code = el('code'); pre.append(code); fragment.append(pre); }
      continue;
    }
    if (code) { code.append(document.createTextNode(line + '\n')); continue; }
    if (!line.trim()) { list = paragraph = null; continue; }
    const bullet = line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)$/);
    if (bullet) {
      paragraph = null;
      if (!list) { list = el('ul'); fragment.append(list); }
      const item = el('li'); inline(item, bullet[1]); list.append(item); continue;
    }
    list = null;
    const heading = line.match(/^#{1,6}\s+(.*)$/);
    if (heading) { paragraph = null; const h = el('h3'); inline(h, heading[1]); fragment.append(h); continue; }
    if (!paragraph) { paragraph = el('p'); fragment.append(paragraph); } else paragraph.append(el('br'));
    inline(paragraph, line);
  }
  return fragment;
}
function contentBlock(block) {
  const card = el('div', undefined, 'client-result');
  const item = block.source || block.resource;
  if (item) {
    card.append(link(item.title || block.fallback || 'Open', item.url));
    if (item.summary) card.append(el('p', item.summary));
  } else if (block.video?.provider === 'youtube' && /^[A-Za-z0-9_-]{11}$/.test(block.video.id)) {
    const frame = el('iframe');
    frame.src = 'https://www.youtube-nocookie.com/embed/' + block.video.id;
    frame.title = block.video.title || 'Video'; frame.allow = 'encrypted-media; picture-in-picture; fullscreen';
    frame.referrerPolicy = 'strict-origin-when-cross-origin'; frame.loading = 'lazy';
    card.append(frame);
    // YouTube does not advertise audio-only capability. Never simulate it by hiding a player.
  } else if (block.audio && safeURL(block.audio.url)) {
    const audio = el('audio'); audio.controls = true; audio.preload = 'none'; audio.src = safeURL(block.audio.url);
    card.append(el('p', block.audio.title || block.fallback), audio);
  } else if (block.image && safeURL(block.image.url)) {
    const image = el('img'); image.src = safeURL(block.image.url); image.alt = block.image.description || block.fallback;
    image.loading = 'lazy'; image.referrerPolicy = 'no-referrer'; card.append(image);
  } else card.append(el('p', block.fallback || 'This result is not supported by this client.'));
  return card;
}
function message(role, text, content = []) {
  const section = el('section', undefined, 'turn');
  const body = el('div', undefined, role === 'ROLE_PERSON' ? 'request' : 'answer');
  body.append(el('strong', role === 'ROLE_PERSON' ? 'You' : agent?.name || 'Assistant', 'metadata-row'));
  const prose = el('div', undefined, 'message-body'); prose.append(markdown(text));
  for (const item of content) prose.append(contentBlock(item));
  body.append(prose); section.append(body); $('responses').append(section);
  if (role === 'ROLE_AGENT') lastReply = text;
  return section;
}
async function rpc(operation, body = {}) {
  const id = crypto.randomUUID();
  const csrf = decodeURIComponent((document.cookie.match(/(?:^|; )csrf_token=([^;]+)/) || [])[1] || '');
  const response = await fetch('/', {method: 'POST', credentials: 'same-origin', cache: 'no-store',
    signal: AbortSignal.timeout(operation === 'Ask' ? 135000 : 20000),
    headers: {'Content-Type': 'application/json', 'Micro-Protocol': 'mucp/1', 'Micro-Service': 'assistant',
      'Micro-Endpoint': 'Assistant.' + operation, 'Micro-Id': id, 'X-CSRF-Token': csrf}, body: JSON.stringify(body)});
  let data;
  try { data = await response.json(); } catch { data = {}; }
  if (!response.ok) {
    const error = Error(data.error?.message || (typeof data.error === 'string' ? data.error : 'The runtime could not complete this request.'));
    error.status = response.status; error.thread = response.headers.get('Micro-Thread'); throw error;
  }
  if (response.headers.get('Micro-Protocol') !== 'mucp/1' || response.headers.get('Micro-Id') !== id)
    throw Error('This runtime does not support the client’s MUCP version.');
  return data;
}
function failure(error) {
  if (error.status === 401) {
    connection.replaceChildren(el('span', 'Sign in to use your agents. '), link('Sign in', '/login?redirect=' + encodeURIComponent(location.pathname + location.search), 'btn'));
    $('chat').hidden = true;
  } else { const target = $('chat').hidden ? connection : status; target.textContent = error.message || 'Connection lost. Open Chats before retrying; your message may already have been accepted.'; }
}
function setThread(id) {
  thread = id;
  const q = new URLSearchParams({agent: agent.id}); if (id) q.set('thread', id);
  history.replaceState(null, '', '?' + q);
}
function setBusy(value) { busy = value; $('send').disabled = value; }
async function readThread(id, version, token = '', append = false) {
  const result = await rpc('ReadThread', {thread: id, pageSize: 100, pageToken: token});
  if (version !== epoch) return;
  agent = agents.find(a => a.id === result.thread.agentId) || agent;
  $('agent-description').textContent = agent.description;
  $('agent-name').textContent = result.thread.subject || agent.name;
  if (!append) { $('responses').replaceChildren(); lastReply = ''; }
  for (const m of result.messages || []) message(m.role, m.text, m.content);
  if (result.nextPageToken) return readThread(id, version, result.nextPageToken, true);
  if (result.pending) {
    status.textContent = 'Working…'; setBusy(true);
    setTimeout(() => { if (version === epoch) readThread(id, version).catch(error => recover(error, id, version)); }, 3000);
  } else { recovering = false; setBusy(false); status.textContent = ''; }
}
function recover(error, id, version) {
  if (version !== epoch) return;
  if ([400, 401, 403, 404].includes(error.status)) { setBusy(false); failure(error); return; }
  status.textContent = 'Reconnecting…';
  setTimeout(() => { if (version === epoch) readThread(id, version).catch(error => recover(error, id, version)); }, 5000);
}
function newChat() {
  epoch++; recovering = false; setBusy(false); setThread(''); $('responses').replaceChildren(); lastReply = '';
  $('agent-name').textContent = agent.name; status.textContent = ''; stopVoice(); $('prompt').focus();
}
$('new-chat').addEventListener('click', event => { event.preventDefault(); newChat(); });
$('compose').addEventListener('submit', async event => {
  event.preventDefault(); const prompt = $('prompt').value.trim();
  if (busy || !prompt) return;
  if (new TextEncoder().encode(prompt).length > 8000) { status.textContent = 'Please shorten your message (maximum 8,000 UTF-8 bytes).'; return; }
  stopVoice(); setBusy(true); const version = epoch;
  message('ROLE_PERSON', prompt); $('prompt').value = ''; $('prompt').blur(); status.textContent = 'Working…';
  try {
    const result = await rpc('Ask', {agentId: agent.id, thread, prompt, mode: 'ACCESS_MODE_PERSONAL'});
    if (version !== epoch) return;
    setThread(result.thread); message('ROLE_AGENT', result.text, result.content); status.textContent = '';
    $('responses').lastElementChild?.scrollIntoView({block: 'nearest', behavior: 'smooth'});
  } catch (error) {
    if (version !== epoch) return;
    if (error.thread) {
      setThread(error.thread);
      if (error.status === 504) { recovering = true; readThread(thread, version).catch(e => recover(e, thread, version)); return; }
    }
    failure(error);
    if (!error.status) status.textContent = 'Connection lost. Open Chats to check for the reply before sending again.';
  } finally { if (version === epoch && !recovering) setBusy(false); }
});
$('prompt').addEventListener('keydown', event => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); $('compose').requestSubmit(); }
});
async function directory(view, pageToken = '', append = false) {
  $('chat').hidden = true; const panel = $('directory'); panel.hidden = false;
  if (!append) panel.replaceChildren(el('h1', view === 'agents' ? 'Choose an agent' : 'Chats'));
  const list = el('div', undefined, 'collection-list'); panel.append(list);
  if (view === 'agents') {
    for (const a of agents) {
      const item = link('', '?agent=' + encodeURIComponent(a.id), 'collection-item');
      const body = el('div'); body.append(el('strong', a.name), el('p', a.description, 'text-muted')); item.append(body); list.append(item);
    }
  } else {
    const result = await rpc('ListThreads', {pageSize: 25, pageToken});
    for (const t of result.threads || []) {
      const item = link('', '?thread=' + encodeURIComponent(t.id), 'collection-item');
      const body = el('div'); body.append(el('strong', t.subject || 'Untitled chat'), el('p', agents.find(a => a.id === t.agentId)?.name || 'Assistant', 'text-muted')); item.append(body); list.append(item);
    }
    if (!append && !result.threads?.length) panel.append(el('p', 'Your conversations will appear here.'));
    if (result.nextPageToken) {
      const more = el('button', 'More'); panel.append(more);
      more.addEventListener('click', async () => { more.disabled = true; try { await directory(view, result.nextPageToken, true); more.remove(); } catch (e) { more.disabled = false; failure(e); } });
    }
  }
}
// Browser voice is optional and explicit. Dictation fills the composer for review.
let recognition, wakeLock;
async function keepAwake() { try { wakeLock = await navigator.wakeLock?.request('screen'); } catch {} }
function stopVoice() { recognition?.stop(); window.speechSynthesis?.cancel(); wakeLock?.release(); $('stop').hidden = true; }
const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
if (Recognition) {
  $('dictate').hidden = false;
  $('dictate').addEventListener('click', () => {
    stopVoice(); recognition = new Recognition(); recognition.interimResults = true;
    const before = $('prompt').value; status.textContent = 'Listening…'; $('stop').hidden = false;
    recognition.onresult = event => { $('prompt').value = before + (before ? ' ' : '') + Array.from(event.results, r => r[0].transcript).join(' '); };
    recognition.onerror = () => { status.textContent = 'Could not hear you. Try again or type your message.'; };
    recognition.onend = () => { $('stop').hidden = true; wakeLock?.release(); if (status.textContent === 'Listening…') status.textContent = 'Review your message, then send.'; };
    recognition.start(); keepAwake();
  });
}
if ('speechSynthesis' in window) {
  $('read').hidden = false;
  $('read').addEventListener('click', () => {
    if (!lastReply) return; stopVoice(); const speech = new SpeechSynthesisUtterance(lastReply);
    speech.onend = speech.onerror = () => { $('stop').hidden = true; status.textContent = ''; wakeLock?.release(); };
    status.textContent = 'Reading reply…'; $('stop').hidden = false; keepAwake(); speechSynthesis.speak(speech);
  });
}
$('stop').addEventListener('click', () => { stopVoice(); status.textContent = ''; });
window.addEventListener('pagehide', () => { epoch++; stopVoice(); });
async function start() {
  const result = await rpc('ListAgents'); agents = result.agents || [];
  connection.textContent = '';
  if (!agents.length) { connection.textContent = 'No agents are available on this runtime.'; return; }
  const selected = params.get('agent');
  agent = selected ? agents.find(a => a.id === selected || a.slug === selected) : agents.find(a => a.id === 'micro') || agents[0];
  if (!agent) throw Error('This agent is not available on your account.');
  const view = params.get('view');
  if (view === 'account') {
    const panel = $('directory'); panel.hidden = false;
    panel.append(el('h1', 'Account'), el('p', 'Connected to ' + location.origin));
    const list = el('div', undefined, 'collection-list');
    for (const [title, path] of [['Account and billing', '/account'], ['Open full web experience', '/home'], ['Downloads', '/download'], ['Sign out', '/logout']]) list.append(link(title, path, 'collection-item'));
    panel.append(list); return;
  }
  if (view === 'agents' || view === 'chats') return directory(view);
  $('chat').hidden = false; $('agent-name').textContent = agent.name; $('agent-description').textContent = agent.description;
  if (thread) { setBusy(true); await readThread(thread, epoch); }
}
start().catch(failure);
if ('serviceWorker' in navigator) navigator.serviceWorker.register('sw.js', {scope: './'}).catch(() => {});
