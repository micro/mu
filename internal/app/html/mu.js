if (typeof document === 'undefined') {
var APP_PREFIX = 'mu_';
var VERSION = 'v3-compact';
var CACHE_NAME = APP_PREFIX + VERSION;

var STATIC_CACHE = [
  '/mu.png',
  '/home.png',
  '/chat.png',
  '/mail.png',
  '/post.png',
  '/news.png',
  '/video.png',
  '/wallet.png',
  '/agent.svg',
  '/stream.svg',
  '/places.svg',
  '/weather.png',
  '/markets.svg',
  '/account.png',
  '/logout.png',
  '/icon-192.png',
  '/icon-512.png'
];

self.addEventListener('fetch', function (e) {
  const url = new URL(e.request.url);

  if (e.request.method !== 'GET') {
    return;
  }

  if (url.pathname.match(/\.(png|jpg|jpeg|gif|svg|ico)$/)) {
    e.respondWith(
      caches.match(e.request).then(cached => cached || fetch(e.request))
    );
  }
});

self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(function (cache) {
      return Promise.all(STATIC_CACHE.map(function (url) {
        return cache.add(url).catch(function () {});
      }));
    }).then(function () {
      return self.skipWaiting();
    }).catch(function () {
      return self.skipWaiting();
    })
  );
});

self.addEventListener('activate', function (e) {
  e.waitUntil(
    caches.keys().then(keys => {
      console.log('Clearing all old caches');
      return Promise.all(
        keys.map(key => {
          if (key !== CACHE_NAME) {
            console.log('Deleting cache:', key);
            return caches.delete(key);
          }
        })
      );
    }).then(() => self.clients.claim())
  );
});

self.addEventListener('push', function (e) {
  var n = {}, why = '';
  try { n = e.data ? e.data.json() : {}; } catch (err) { why = 'the payload could not be read'; }
  if (!why && !n.title) why = 'the payload had no title';

  var title = 'Micro \u2014 ' + (n.title || 'Notification unreadable');
  var body = why ? ('This device woke up but could not read it: ' + why + '.') : (n.body || '');
  var tag = n.tag || 'mu';

  e.waitUntil(
    self.registration.showNotification(title, {
      body: body,
      icon: '/icon-192.png',
      badge: '/icon-192.png',
      tag: tag,
      renotify: true,
      data: {url: n.url || '/inbox'}
    }).then(function () {
      return receipt(tag, !why, why);
    }, function (err) {
      return receipt(tag, false, 'showNotification failed: ' + (err && err.message ? err.message : 'unknown'));
    })
  );
});

function receipt(tag, shown, why) {
  try {
    return fetch('/notify/received', {
      method: 'POST',
      credentials: 'include',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({tag: tag, shown: !!shown, why: why || ''})
    }).catch(function () {});
  } catch (err) { return Promise.resolve(); }
}

self.addEventListener('notificationclick', function (e) {
  e.notification.close();
  var url = (e.notification.data && e.notification.data.url) || '/inbox';
  e.waitUntil(
    self.clients.matchAll({type: 'window', includeUncontrolled: true}).then(function (all) {
      for (var i = 0; i < all.length; i++) {
        if (all[i].url.indexOf(self.registration.scope) === 0 && 'focus' in all[i]) {
          all[i].navigate(url);
          return all[i].focus();
        }
      }
      if (self.clients.openWindow) return self.clients.openWindow(url);
    })
  );
});

self.addEventListener('message', function (e) {
  if (!e.data || e.data.mu !== 'version') return;
  var reply = {mu: 'version', version: VERSION};
  if (e.ports && e.ports[0]) { e.ports[0].postMessage(reply); return; }
  if (e.source && e.source.postMessage) e.source.postMessage(reply);
});

} else {
function getCsrfToken() {
  var m = document.cookie.match('(?:^|; )csrf_token=([^;]*)');
  return m ? decodeURIComponent(m[1]) : '';
}

// All browser writes use the current session token, including pages left open
// across a deployment. Refresh before sending; never replay a mutation.
(function() {
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async function(url, opts) {
    const request = new Request(url, opts);
    const write = !['GET','HEAD','OPTIONS'].includes(request.method);
    if (write && new URL(request.url).origin === location.origin && getCsrfToken()) {
      const session = await nativeFetch('/session', {credentials:'same-origin',cache:'no-store'});
      if (!session.ok) throw Error('Please reload the page and sign in again.');
      request.headers.set('X-CSRF-Token', getCsrfToken());
    }
    return nativeFetch(request);
  };
})();

document.addEventListener('submit', function(e) {
  var form = e.target;
  if (!form || form.tagName !== 'FORM') return;
  var method = (form.method || 'GET').toUpperCase();
  if (method !== 'POST') return;
  if (form.querySelector('input[name="_csrf"]')) return;
  var token = getCsrfToken();
  if (!token) return;
  var input = document.createElement('input');
  input.type = 'hidden';
  input.name = '_csrf';
  input.value = token;
  form.appendChild(input);
}, true);

function timeAgo(timestamp) {
  const now = Math.floor(Date.now() / 1000);
  const deltaMinutes = (now - timestamp) / 60;

  if (deltaMinutes <= 523440) { // less than 363 days
    return distanceOfTime(deltaMinutes) + ' ago';
  } else {
    const date = new Date(timestamp * 1000);
    return date.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' });
  }
}

function distanceOfTime(minutes) {
  if (minutes < 1) {
    const secs = Math.max(1, Math.floor(minutes * 60));
    return secs === 1 ? '1 sec' : secs + ' secs';
  } else if (minutes < 2) {
    return '1 minute';
  } else if (minutes < 60) {
    return Math.floor(minutes) + ' minutes';
  } else if (minutes < 1440) {
    const hrs = Math.floor(minutes / 60);
    return hrs === 1 ? '1 hour' : hrs + ' hours';
  } else if (minutes < 2880) {
    return '1 day';
  } else if (minutes < 43800) {
    return Math.floor(minutes / 1440) + ' days';
  } else if (minutes < 87600) {
    return '1 month';
  } else {
    return Math.floor(minutes / 43800) + ' months';
  }
}

function updateTimestamps() {
  document.querySelectorAll('[data-timestamp]').forEach(el => {
    const timestamp = parseInt(el.dataset.timestamp);
    if (!isNaN(timestamp) && timestamp > 0) {
      el.textContent = timeAgo(timestamp);
    }
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', function() {
    updateTimestamps();
    setInterval(updateTimestamps, 60000);
  });
} else {
  updateTimestamps();
  setInterval(updateTimestamps, 60000);
}

window.muConfirm=function(message){return Promise.resolve(window.confirm(message));};
function showToast(message, type = 'info', duration = 4000) {
  const existing = document.getElementById('mu-toast');
  if (existing) existing.remove();

  const toast = document.createElement('div');
  toast.id = 'mu-toast';
  toast.className = 'mu-toast mu-toast-' + type;
  toast.textContent = message;

  const close = document.createElement('span');
  close.textContent = '×';
  close.className = 'mu-toast-close';
  close.onclick = () => toast.remove();
  toast.appendChild(close);

  document.body.appendChild(toast);

  if (duration > 0) {
    setTimeout(() => {
      if (toast.parentNode) {
        toast.classList.add('mu-toast-hide');
        setTimeout(() => toast.remove(), 300);
      }
    }, duration);
  }
}

async function apiCall(url, options = {}) {
  const defaults = {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin'
  };

  const config = { ...defaults, ...options };
  if (options.body && typeof options.body === 'object') {
    config.body = JSON.stringify(options.body);
  }

  try {
    const response = await fetch(url, config);
    const data = await response.json();

    if (!response.ok) {
      const errorMsg = data.error || data.message || 'Request failed';
      showToast(errorMsg, 'error');
      return { ok: false, error: errorMsg, status: response.status };
    }

    return { ok: true, data, status: response.status };
  } catch (err) {
    const errorMsg = 'Network error - please try again';
    showToast(errorMsg, 'error');
    return { ok: false, error: errorMsg, status: 0 };
  }
}

}

if (typeof document !== "undefined") {

// Command surface
(()=>{'use strict';
const form=document.querySelector('#command-form'),input=document.querySelector('#command-input'),log=document.querySelector('#responses'),send=document.querySelector('#send'),status=document.querySelector('#status');
if(!form)return;
const conversation=form.closest('.conversation'),panel=form.closest('.prompt-panel');
const reducedMotion=window.matchMedia('(prefers-reduced-motion: reduce)');
if(conversation.classList.contains('is-active'))log.scrollTop=log.scrollHeight;
let busy=false,thread=new URLSearchParams(location.search).get('session')||new URLSearchParams(location.search).get('continue')||'';
function remember(){if(thread)history.replaceState(null,'','/?session='+encodeURIComponent(thread));}
function headers(accept){const token=(document.cookie.match(/(?:^|; )csrf_token=([^;]+)/)||[])[1]||'';return {'Content-Type':'application/json','Accept':accept,'X-CSRF-Token':decodeURIComponent(token)};}
async function assistant(command,answer){
 const response=await fetch('/agent',{method:'POST',credentials:'same-origin',headers:headers('text/event-stream'),body:JSON.stringify({prompt:command,context_id:thread,agent:form.dataset.agent||'',stream_text:true})});
 if(!response.ok)throw Error(response.status===401?'Log in to ask the assistant.':await failure(response));
 const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',done=false;
 function event(line){if(!line.startsWith('data: '))return;const e=JSON.parse(line.slice(6));if(e.type==='flow_id'){thread=e.thread||thread;remember();}if(e.type==='working'||e.type==='tool_start')status.textContent=e.message||'Working…';if(e.type==='stream_token'){status.textContent='Writing…';}if(e.type==='response'){answer.innerHTML=e.html;done=true;}if(e.type==='error'){throw Error(e.message||'Request failed.');}}
 try{while(true){const chunk=await reader.read();buffer+=decoder.decode(chunk.value||new Uint8Array(),{stream:!chunk.done});const lines=buffer.split('\n');buffer=lines.pop();for(const line of lines)event(line);if(chunk.done){if(buffer)event(buffer);break;}}}finally{reader.releaseLock();}
 if(!done)throw Error('The connection closed before the response completed. Your request may still be running; check inbox before submitting it again.');
}
async function failure(response){try{const j=await response.json();return typeof j.error==='string'?j.error:(j.error?.message||'Request failed.');}catch{return 'Request failed ('+response.status+').';}}
async function run(command){
 if(busy||!command.trim())return;busy=true;send.disabled=true;status.textContent='Working…';input.value='';
 const first=!conversation.classList.contains('is-active'),before=form.getBoundingClientRect().top;
 if(first)conversation.classList.add('is-active');
 if(first&&!reducedMotion.matches)panel.animate([{transform:'translateY('+(before-form.getBoundingClientRect().top)+'px)'},{transform:'translateY(0)'}],{duration:320,easing:'cubic-bezier(.2,.7,.2,1)'});
 const turn=document.createElement('section');turn.className='turn';const q=document.createElement('div');q.className='request';
 q.textContent=command;
 const answer=document.createElement('div');answer.className='answer';turn.append(q,answer);log.append(turn);
 log.querySelectorAll('.turn').forEach(item=>item.style.minHeight='');
 turn.style.minHeight=log.clientHeight+'px';
 requestAnimationFrame(()=>{
  const top=log.scrollTop+turn.getBoundingClientRect().top-log.getBoundingClientRect().top;
  log.scrollTo({top:Math.max(0,top),behavior:first||reducedMotion.matches?'instant':'smooth'});
 });
 try{await assistant(command,answer);status.textContent='';}catch(error){answer.textContent=error.message;answer.classList.add('error');status.textContent='Request stopped.';}finally{busy=false;send.disabled=false;input.focus({preventScroll:true});}
}
form.addEventListener('submit',e=>{e.preventDefault();run(input.value.trim());});
input.addEventListener('keydown',e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();form.requestSubmit();}});
try{if(location.hash){input.value=decodeURIComponent(location.hash.slice(1));history.replaceState(null,'','/');}}catch{}
})();


// account/pages.go
	if (document.getElementById('passkey-login') && window.PublicKeyCredential) {
	  PublicKeyCredential.isConditionalMediationAvailable && PublicKeyCredential.isConditionalMediationAvailable().then(function(){});
	  // classList, not style.display: .d-none is display:none !important and an
	  // inline style loses to it, which is why this button was never once seen.
	  document.getElementById('passkey-login').classList.remove('d-none');
	}

	function base64urlToBuffer(b64) {
	  var pad = b64.length % 4;
	  if (pad) b64 += '='.repeat(4 - pad);
	  var str = atob(b64.replace(/-/g, '+').replace(/_/g, '/'));
	  var buf = new Uint8Array(str.length);
	  for (var i = 0; i < str.length; i++) buf[i] = str.charCodeAt(i);
	  return buf.buffer;
	}

	function bufferToBase64url(buf) {
	  var bytes = new Uint8Array(buf);
	  var str = '';
	  for (var i = 0; i < bytes.length; i++) str += String.fromCharCode(bytes[i]);
	  return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
	}

	async function loginWithPasskey() {
	  try {
	    var beginRes = await fetch('/passkey/login/begin', {method: 'POST'});
	    if (!beginRes.ok) { alert('Passkey login not available'); return; }
	    var options = await beginRes.json();

	    options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
	    if (options.publicKey.allowCredentials) {
	      options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(function(c) {
	        return Object.assign({}, c, {id: base64urlToBuffer(c.id)});
	      });
	    }

	    var assertion = await navigator.credentials.get(options);

	    var body = {
	      id: assertion.id,
	      rawId: bufferToBase64url(assertion.rawId),
	      type: assertion.type,
	      response: {
	        authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
	        clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
	        signature: bufferToBase64url(assertion.response.signature),
	        userHandle: bufferToBase64url(assertion.response.userHandle)
	      }
	    };
	    if (assertion.authenticatorAttachment) {
	      body.authenticatorAttachment = assertion.authenticatorAttachment;
	    }

	    var finishRes = await fetch('/passkey/login/finish'+window.location.search, {
	      method: 'POST',
	      headers: {'Content-Type': 'application/json'},
	      body: JSON.stringify(body)
	    });
	    var result = await finishRes.json();
	    if (result.success) {
	      window.location.href = result.redirect || '/';
	    } else {
	      document.getElementById('auth-status').textContent='Sign-in failed. Please try another sign-in method.';
	    }
	  } catch (e) {
	    if (e.name !== 'NotAllowedError') document.getElementById('auth-status').textContent='Unable to sign in. Please try again.';
	  }
	}


// account/passkey.go
async function registerPasskey() {
  try {
    const beginRes = await fetch('/passkey/register/begin', {method: 'POST'});
    if (!beginRes.ok) { alert('Failed to start registration'); return; }
    const options = await beginRes.json();

    options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
    options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);
    if (options.publicKey.excludeCredentials) {
      options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(c => ({
        ...c, id: base64urlToBuffer(c.id)
      }));
    }

    const credential = await navigator.credentials.create(options);

    const attestation = {
      id: credential.id,
      rawId: bufferToBase64url(credential.rawId),
      type: credential.type,
      response: {
        attestationObject: bufferToBase64url(credential.response.attestationObject),
        clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)
      }
    };
    if (credential.response.getTransports) {
      attestation.response.transports = credential.response.getTransports();
    }
    if (credential.authenticatorAttachment) {
      attestation.authenticatorAttachment = credential.authenticatorAttachment;
    }

    const finishRes = await fetch('/passkey/register/finish', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(attestation)
    });
    const result = await finishRes.json();
    if (result.success) {
      location.reload();
    } else {
      alert('Registration failed');
    }
  } catch (e) {
    if (e.name !== 'NotAllowedError') alert('Error: ' + e.message);
  }
}


// account/place.go
function muUseMyLocation(btn){
  if(!navigator.geolocation){btn.textContent='Not available';return}
  btn.textContent='Locating…';
  navigator.geolocation.getCurrentPosition(function(p){
    document.getElementById('place-lat').value=p.coords.latitude.toFixed(2);
    document.getElementById('place-lon').value=p.coords.longitude.toFixed(2);
    try{document.getElementById('place-zone').value=Intl.DateTimeFormat().resolvedOptions().timeZone||''}catch(e){}
    btn.textContent='Got it — press Save';
  },function(){btn.textContent='Could not locate'},{timeout:8000});
}
// The opposite. Empties every field, including the hidden ones, and posts —
// which is the input SetPlace already treats as "forget where I am".
function muForgetLocation(btn){
  if(!confirm('Forget where you are? Your agents stop knowing — no forecast, no trains, no prayer times.'))return;
  var f=btn.form||btn.closest('form');if(!f)return;
  var name=f.querySelector('[name="place"]');if(name)name.value='';
  ['place-lat','place-lon','place-zone'].forEach(function(id){
    var e=document.getElementById(id);if(e)e.value='';
  });
  f.submit();
}


// account/token.go
async function createToken(e) {
	e.preventDefault();
	var form = e.target;

	var res = await fetch('/token', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({client: form.client.value, name: form.name.value, expires_in: parseInt(form.expires_in.value), services:form.client.value==='services'?Array.from(form.querySelectorAll('[name="services"]:checked'),el=>el.value):[], permissions:form.client.value==='api'?['read',...Array.from(form.querySelectorAll('[name="capability"]:checked'),el=>el.value),...(form.api_write.checked?['write']:[])]:[]})
	});
	var result = await res.json();
	if (result.success) {
		document.getElementById('new-token').textContent = result.token;
		document.getElementById('token-result').classList.remove('d-none');

	} else {
		alert(typeof result.error==='string'?result.error:(result.error?.message||'Failed to create token'));
	}
}


// cardJS
(function(){
  var go = document.getElementById('push-go');
  if (!go) return;
  var state = document.getElementById('push-state');
  function say(t){ if (state) state.textContent = t; }

  if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
    go.disabled = true;
    say('This browser cannot show notifications.');
    return;
  }
  if (Notification.permission === 'denied') {
    go.disabled = true;
    say('Blocked in this browser’s settings.');
    return;
  }

  // base64url to bytes: what the subscription call wants the key as.
  function keyBytes(s){
    var pad = '='.repeat((4 - s.length % 4) % 4);
    var raw = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'));
    var out = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }
  function b64(buf){
    var bytes = new Uint8Array(buf), s = '';
    for (var i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
    return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  // What to call this device, in words somebody recognises.
  //
  // It was navigator.platform, which reports "Linux armv81" for an Android
  // phone and "Linux x86_64" for a laptop — the two devices somebody actually
  // owns, named after an instruction set. A list you cannot read is a list you
  // cannot act on: the whole point of naming devices is deciding which one to
  // turn off.
  //
  // Browser and kind, because that is what tells two rows apart. userAgentData
  // where it exists (Chromium) and the user-agent string where it does not,
  // which is Firefox and Safari — and platform last, so a device this cannot
  // read is still named something rather than nothing.
  function deviceLabel(){
    var d = navigator.userAgentData, os = '', browser = '';
    var ua = navigator.userAgent || '';

    if (/iPhone/i.test(ua)) os = 'iPhone';
    else if (/iPad/i.test(ua)) os = 'iPad';
    else if (/Android/i.test(ua)) os = 'Android';
    else if (/Macintosh|Mac OS X/i.test(ua)) os = 'Mac';
    else if (/Windows/i.test(ua)) os = 'Windows';
    else if (/CrOS/i.test(ua)) os = 'ChromeOS';
    else if (/Linux/i.test(ua)) os = 'Linux';

    // Order matters: Edge and Opera both say "Chrome", and Chrome says
    // "Safari". Most specific first.
    if (/Edg\//.test(ua)) browser = 'Edge';
    else if (/OPR\/|Opera/.test(ua)) browser = 'Opera';
    else if (/Firefox\//.test(ua)) browser = 'Firefox';
    else if (/SamsungBrowser/.test(ua)) browser = 'Samsung Internet';
    else if (/Chrome\//.test(ua)) browser = 'Chrome';
    else if (/Safari\//.test(ua)) browser = 'Safari';

    // An installed app is worth saying: it is a different thing from the same
    // browser with a tab open, and it is the one that receives when nothing is.
    var installed = false;
    try {
      installed = window.matchMedia('(display-mode: standalone)').matches ||
        navigator.standalone === true;
    } catch (e) {}

    if (!os && d && d.platform) os = d.platform;
    if (!os && !browser) return navigator.platform || '';

    var name = browser && os ? browser + ' on ' + os : (browser || os);
    return installed ? name + ' (installed)' : name;
  }

  // Hand one subscription to the server. Idempotent: it is matched on the
  // endpoint, so sending the same one twice updates it rather than doubling it.
  function tell(sub){
    var raw = sub.toJSON();
    return fetch('/notify/subscribe', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrfToken()
      },
      body: JSON.stringify({
        endpoint: sub.endpoint,
        keys: {p256dh: raw.keys.p256dh, auth: raw.keys.auth},
        label: deviceLabel()
      })
    }).then(function(res){ return res.json(); });
  }

  // Which copy of the app is actually installed on this device.
  //
  // The record said "sent — the device has not said it arrived" five times
  // running, and that line has two completely different causes: the worker
  // never woke, or the worker woke and is too old to contain the code that
  // reports back. Same symptom, opposite fixes.
  //
  // So ask it. A worker built after this was written answers with its version;
  // an older one has no message handler and never replies, and the silence is
  // the diagnosis. Two seconds, because a sleeping worker has to be started
  // before it can answer and that is not instant on a phone.
  var workerLine = document.getElementById('push-worker');
  var update = document.getElementById('push-update');
  function saidWorker(text, stale){
    if (!workerLine) return;
    workerLine.textContent = text;
    workerLine.hidden = false;
    if (stale && update) update.classList.remove('d-none');
  }
  function askWorker(){
    if (!workerLine) return;
    navigator.serviceWorker.getRegistration().then(function(reg){
      var sw = reg && (reg.active || reg.waiting);
      if (!sw) { saidWorker('This device has no copy of the app installed yet. Reload the page.', true); return; }
      var chan = new MessageChannel();
      var answered = false;
      chan.port1.onmessage = function(e){
        answered = true;
        var v = (e.data && e.data.version) || '?';
        saidWorker('This device runs ' + v + '.', false);
      };
      try { sw.postMessage({mu: 'version'}, [chan.port2]); } catch (err) {}
      setTimeout(function(){
        if (answered) return;
        saidWorker('This device is running an old copy of the app, which cannot ' +
          'report whether a notification arrived. Update it.', true);
      }, 2000);
    }).catch(function(){});
  }
  askWorker();

  // Force the browser to fetch the worker again and take the new one. mu.js
  // calls skipWaiting on install, so a fresh copy activates without waiting for
  // every tab to close — the reload is so this page is controlled by it.
  if (update) update.addEventListener('click', function(){
    update.disabled = true;
    update.textContent = 'Updating…';
    navigator.serviceWorker.getRegistration().then(function(reg){
      if (!reg) return;
      return reg.update();
    }).then(function(){
      location.reload();
    }).catch(function(){
      update.disabled = false;
      update.textContent = 'Update this device';
      say('Could not update this device.');
    });
  });

  var test = document.getElementById('push-test');
  var off = document.getElementById('push-off');

  function on(){
    // Nothing to say. The pair of buttons is the state: "Notifications ·
    // Turn off" already means they are on, and "On for this device." beside
    // it was the same fact a second time — which is what made the row read
    // as two unrelated things rather than one control. push-state stays for
    // what it is actually for, which is the failures below.
    say('');
    go.classList.add('d-none');
    if (test) test.classList.remove('d-none');
    if (off) off.classList.remove('d-none');
  }

  function offNow(){
    go.classList.remove('d-none');
    go.disabled = false;
    if (test) test.classList.add('d-none');
    if (off) off.classList.add('d-none');
    say('Off.');
  }

  // Turning it off, properly: the browser stops holding a subscription and
  // this instance stops holding a row. Doing either alone is what leaves the
  // two disagreeing — a device the server goes on sending to forever, or a
  // browser that silently re-registers on the next page load, which is exactly
  // what the re-post below does.
  // Removing one device by name, from the list.
  //
  // The endpoint is the identity, and it is on the row. Removing the one this
  // browser is holding also has to unsubscribe the browser — otherwise the
  // server forgets it and the browser goes on holding a subscription nothing
  // will ever send to, which is the state that made "turn it off" unreliable
  // before there was a way off at all.
  document.querySelectorAll('.push-dev-off').forEach(function(btn){
    btn.addEventListener('click', function(){
      var row = btn.closest('.push-device');
      if (!row) return;
      var ep = row.getAttribute('data-endpoint');
      row.classList.add('going');
      btn.disabled = true;
      var done = function(){ row.remove(); };
      var drop = function(){
        return fetch('/push/unsubscribe', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': getCsrfToken()
          },
          body: JSON.stringify({endpoint: ep})
        });
      };
      // If this is the browser's own subscription, let go of it here too.
      navigator.serviceWorker.ready.then(function(reg){
        return reg.pushManager.getSubscription();
      }).then(function(sub){
        if (sub && sub.endpoint === ep) return sub.unsubscribe();
      }).catch(function(){}).then(drop).then(done).catch(function(){
        row.classList.remove('going');
        btn.disabled = false;
        say('That did not work.');
      });
    });
  });

  // And say which row is the browser you are looking at, because "Remove" on
  // the wrong one is the mistake this list makes possible.
  if (navigator.serviceWorker) {
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      if (!sub) return;
      var row = document.querySelector('.push-device[data-endpoint="' +
        (window.CSS && CSS.escape ? CSS.escape(sub.endpoint) : sub.endpoint) + '"]');
      if (row) row.classList.add('is-here');
    }).catch(function(){});
  }

  if (off) off.addEventListener('click', function(){
    off.disabled = true;
    off.textContent = 'Turning off…';
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      var endpoint = sub ? sub.endpoint : '';
      // Unsubscribe the browser first. If this page then fails to reach the
      // server the worst case is a row nothing can deliver to, which the send
      // path prunes on the first 410 — the other order leaves a browser that
      // re-registers itself.
      var done = sub ? sub.unsubscribe() : Promise.resolve();
      return done.then(function(){
        return fetch('/notify/unsubscribe', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-CSRF-Token': getCsrfToken()
          },
          body: JSON.stringify({endpoint: endpoint})
        });
      });
    }).then(function(){
      off.disabled = false;
      off.textContent = 'Turn off';
      offNow();
    }).catch(function(){
      off.disabled = false;
      off.textContent = 'Turn off';
      say('That did not work.');
    });
  });

  // Proof, on demand. Only offered once this device is actually subscribed —
  // before that there is nothing for it to arrive on.
  if (test) test.addEventListener('click', function(){
    test.disabled = true;
    test.textContent = 'Sending…';
    // This browser's own subscription, so the server sends there and nowhere
    // else. Without it the test proved that *a* device works, which is not
    // what the answer said and not what anybody wanted to know.
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).catch(function(){ return null; }).then(function(sub){
      return fetch('/notify/test', {
        method: 'POST',
        credentials: 'same-origin',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrfToken()
        },
        body: JSON.stringify({endpoint: sub ? sub.endpoint : ''})
      });
    }).then(function(res){ return res.json(); }).then(function(data){
      test.disabled = false;
      test.textContent = 'Send a test';
      // Both outcomes out loud. Saying nothing on success is indistinguishable
      // from the button being dead, which is what it looked like — and if the
      // notification itself does not arrive, "the push service took it" is the
      // fact that tells you the problem is on the device and not here.
      //
      // Named, because which push service took it is most of the diagnosis:
      // web.push.apple.com is a phone, fcm.googleapis.com is usually not.
      if (!data || !data.ok) { say((data && data.error) || 'That did not work.'); return; }
      if (data.here) {
        say('Sent to this device' + (data.where ? ' via ' + data.where : '') +
            '. If nothing appears, the push service took it and the device did not show it.');
      } else {
        // No endpoint to send: this page could not name its own subscription,
        // so it went to everything registered. Say that rather than claim a
        // device the server never checked.
        say('Sent to all ' + (data.devices || 0) + ' registered device(s) — this browser ' +
            'could not name its own, so turn notifications off and on again here.');
      }
    }).catch(function(){
      test.disabled = false;
      test.textContent = 'Send a test';
      say('Could not reach the server.');
    });
  });

  // What the browser is already holding, told to the server again.
  //
  // This is the half that was missing. It also answers the question the server
  // cannot: "on" for the account is not "on here", and only this browser knows
  // whether this device is one of them.
  if (Notification.permission === 'granted') {
    navigator.serviceWorker.ready.then(function(reg){
      return reg.pushManager.getSubscription();
    }).then(function(sub){
      if (!sub) return;
      return tell(sub).then(function(data){ if (data && data.ok) on(); });
    }).catch(function(){ /* leave the button as it is */ });
  }

  go.addEventListener('click', function(){
    go.disabled = true;
    say('Asking…');
    Notification.requestPermission().then(function(p){
      if (p !== 'granted') { go.disabled = false; say('Not allowed.'); return; }
      return navigator.serviceWorker.ready.then(function(reg){
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: keyBytes(document.getElementById('push-key').value)
        });
      }).then(tell).then(function(data){
        if (data && data.ok) { on(); }
        else { go.disabled = false; say((data && data.error) || 'That did not work.'); }
      });
    }).catch(function(err){
      go.disabled = false;
      say('That did not work: ' + err.message);
    });
  });
})();


// askJS
(function(){
  var ask = document.getElementById('push-ask');
  var go = document.getElementById('push-go');
  if (!ask || !go) return;

  // A control that cannot work is worse than no control. cardJS disables the
  // button and writes the reason into push-state for both of these; on this
  // row there is no room to read a reason, so there is nothing to show.
  if (!('serviceWorker' in navigator) || !('PushManager' in window) ||
      typeof Notification === 'undefined' || Notification.permission === 'denied') {
    return; // stays hidden
  }

  // Reveal once, whichever way round it is. cardJS decides which of the two
  // buttons is showing — it swaps them on load if this device already has a
  // subscription, and again after turning on or off — so all this has to do is
  // stop hiding the pair. Nothing here duplicates that decision.
  ask.hidden = false;
})();


Object.assign(window,{loginWithPasskey,registerPasskey,base64urlToBuffer,bufferToBase64url,muUseMyLocation,muForgetLocation,createToken});
if ('serviceWorker' in navigator) {
 navigator.serviceWorker.register('/mu.js', {scope:'/'}).catch(()=>{});
}
}

function muToggleSyslog(id){const row=document.getElementById(id);if(row)row.style.display=row.style.display==='none'?'table-row':'none';}

// Service forms use the shared browser script.
if(typeof document !== "undefined"){
(function(){
  function csrf() {
    var m = document.cookie.match(/(?:^|; )csrf_token=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
  }
  function values(form) {
    var out = {};
    form.querySelectorAll('[data-type]').forEach(function(el){
      var v = el.value.trim();
      if (v === '') return;
      if (el.dataset.type === 'number') out[el.name] = parseFloat(v);
      else if (el.dataset.type === 'integer') out[el.name] = parseInt(v, 10);
      else if (el.dataset.type === 'boolean') out[el.name] = v === 'true';
      else out[el.name] = v;
    });
    return out;
  }
  document.querySelectorAll('form.try').forEach(function(form){
    form.addEventListener('submit', function(ev){
      ev.preventDefault();
      var out = form.querySelector('.try-out');
      var args = values(form);
      var path = form.dataset.path.replace('/api/v1/', '/services/call/');
      var opts = { credentials: 'same-origin', cache: 'no-store', headers: {} };
      opts.method = 'POST';
      opts.headers['Content-Type'] = 'application/json';
      var t = csrf();
      if (t) opts.headers['X-CSRF-Token'] = t;
      opts.body = JSON.stringify(args);
      out.hidden = false;
      out.textContent = '…';
      fetch(path, opts)
        .then(function(r){ return r.text().then(function(t){ return {status: r.status, text: t}; }); })
        .then(function(res){
          var body = res.text;
          try { body = JSON.stringify(JSON.parse(body), null, 2); } catch (e) {}
          out.textContent = res.status + '\n\n' + body;
        })
        .catch(function(e){ out.textContent = String(e); });
    });
  });
})();

}

if(typeof document!=='undefined'){
 const tokenForm=document.getElementById('create-token-form');
 if(tokenForm){const update=()=>tokenForm.querySelectorAll('[data-token-access]').forEach(section=>{section.hidden=section.dataset.tokenAccess!==tokenForm.client.value;section.disabled=section.hidden;});tokenForm.client.addEventListener('change',update);update();}
}

if (typeof document !== 'undefined') {
 const consent = document.getElementById('oauth-consent');
 if (consent) {
  const mode = consent.elements.access;
  const search = document.getElementById('oauth-service-search');
  const list = consent.querySelector('.oauth-service-list');
  const rows = Array.from(list.children);
  consent.querySelector('.oauth-search').hidden = false;
  function updateConsent() {
   consent.querySelectorAll('[data-oauth-access]').forEach(section => {
    section.hidden = section.dataset.oauthAccess !== mode.value;
    section.disabled = section.hidden;
   });
   const field = mode.value === 'services' ? 'service' : 'capability';
   const selected = mode.value === 'all' ? ['All current services'] : Array.from(consent.querySelectorAll('input[name="' + field + '"]:checked')).map(input => input.value);
   const picker = document.getElementById('oauth-access-picker');
   picker.hidden = mode.value === 'all';
   document.getElementById('oauth-access-summary').textContent = selected.length ? selected.join(', ') + (consent.elements.write.checked ? ' · Read and act' : ' · Read') : 'No access selected';
   consent.querySelector('button[type="submit"]').disabled = selected.length === 0;
   rows.sort((a,b) => Number(b.querySelector('input').checked) - Number(a.querySelector('input').checked) || a.textContent.localeCompare(b.textContent)).forEach(row => list.appendChild(row));
  }
  search.addEventListener('input', () => {
   const query = search.value.trim().toLowerCase();
   rows.forEach(row => { row.hidden = !row.textContent.toLowerCase().includes(query); });
   document.getElementById('oauth-no-results').hidden = rows.some(row => !row.hidden);
  });
  mode.addEventListener('change', () => { document.getElementById('oauth-access-picker').open = mode.value !== 'all'; });
  consent.addEventListener('change', updateConsent);
  updateConsent();
 }
}

if (typeof document !== 'undefined') {
 // Shared live HTML preview. An opaque sandbox isolates authored code from
 // account cookies and the editor; do not add allow-same-origin here.
 document.querySelectorAll('[data-preview-target]').forEach(source => {
  const frame = document.getElementById(source.dataset.previewTarget);
  if (!frame) return;
  let timer;
  const refresh = () => { frame.srcdoc = source.value || '<p style="font:14px system-ui;color:#666;padding:12px">Your app preview appears here.</p>'; };
  source.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(refresh,350); });
  refresh();
 });
 // Public web/video queries only. Places uses its account-scoped server store.
 document.querySelectorAll('[data-recent-searches]').forEach(host => {
  const form = document.getElementById(host.dataset.recentSearches);
  if (!form) return;
  const input = form.querySelector('input[name="query"],input[name="q"]');
  if (!input) return;
  const key = host.dataset.storageKey;
  let recent = [];
  try { recent = JSON.parse(localStorage.getItem(key)||'[]'); } catch (_) {}
  recent = (Array.isArray(recent)?recent:[]).map(v=>typeof v==='string'?v:(v&&v.query||'')).filter(v=>typeof v==='string'&&v.trim()).slice(0,10);
  const save = () => { try { localStorage.setItem(key,JSON.stringify(recent)); } catch (_) {} };
  form.addEventListener('submit', () => { const q=input.value.trim(); if(q){ recent=[q,...recent.filter(v=>v.toLowerCase()!==q.toLowerCase())].slice(0,10);save();} });
  if(!recent.length)return;
  const heading=document.createElement('h3');heading.textContent='Recent searches';
  const row=document.createElement('div');row.className='form-actions';
  [...new Set(recent)].forEach(q=>{const button=document.createElement('button');button.type='button';button.className='pill';button.textContent=q;button.addEventListener('click',()=>{input.value=q;form.requestSubmit();});row.appendChild(button);});
  const clear=document.createElement('button');clear.type='button';clear.className='mini-btn';clear.textContent='Clear';clear.addEventListener('click',()=>{recent=[];save();host.replaceChildren();});row.appendChild(clear);
  host.replaceChildren(heading,row);
 });
 // In-memory terminal history: commands may contain secrets and must not be
 // persisted in localStorage or shared with another signed-in account.
 const terminal=document.getElementById('shell-command');
 const output=document.getElementById('shell-result');
 if(terminal&&output){
  const input=terminal.elements.command,button=terminal.querySelector('button[type="submit"]');
  let busy=false,history=[],cursor=0,draft='';
  input.addEventListener('keydown',e=>{if(e.key!=='ArrowUp'&&e.key!=='ArrowDown')return;e.preventDefault();if(cursor===history.length)draft=input.value;cursor=Math.max(0,Math.min(history.length,cursor+(e.key==='ArrowUp'?-1:1)));input.value=cursor===history.length?draft:history[cursor];});
  terminal.addEventListener('submit',async e=>{
   e.preventDefault();const command=input.value.trim();if(busy||!command)return;
   const body=new URLSearchParams(new FormData(terminal));
   history.push(command);history=history.slice(-50);cursor=history.length;draft='';input.value='';busy=true;button.disabled=true;
   const entry=document.createElement('section');entry.className='shell-entry';
   const label=document.createElement('div');label.className='shell-command';label.textContent='$ '+command;
   const result=document.createElement('div');result.textContent='Running…';entry.append(label,result);output.appendChild(entry);entry.scrollIntoView({block:'nearest'});
   try{
    const response=await fetch(terminal.action,{method:'POST',body,credentials:'same-origin',headers:{Accept:'text/html'}});
    const doc=new DOMParser().parseFromString(await response.text(),'text/html'),fragment=doc.getElementById('shell-result');
    if(!response.ok||!fragment)throw new Error('Could not retrieve the result. The command may have run; it has not been retried.');
    result.replaceChildren(...Array.from(fragment.childNodes));
   }catch(error){result.textContent=error.message;result.className='text-error';}
   finally{busy=false;button.disabled=false;while(output.querySelectorAll('.shell-entry').length>50)output.querySelector('.shell-entry').remove();input.focus();entry.scrollIntoView({block:'nearest'});}
  });
 }
 // Collapse citation sections without hiding the article itself.
 document.querySelectorAll('.reader-content').forEach(article=>{
  Array.from(article.querySelectorAll('h2,h3,h4')).filter(h=>/^(references|sources|further reading)\s*:?$/i.test(h.textContent.trim())).forEach(heading=>{
   const details=document.createElement('details');details.className='references';const summary=document.createElement('summary');summary.textContent=heading.textContent;details.appendChild(summary);heading.before(details);
   const rank=Number(heading.tagName.slice(1));let next=heading.nextElementSibling;heading.remove();
   while(next&&!(/^H[1-6]$/.test(next.tagName)&&Number(next.tagName.slice(1))<=rank)){const after=next.nextElementSibling;details.appendChild(next);next=after;}
  });
 });
}

if (typeof document !== "undefined") {

(function(){
  var el=document.getElementById('map'), layer=document.getElementById('map-layer');
  if(!el||!layer) return;
  var SIZE=256, style=el.dataset.style, where=document.getElementById('map-where');
  var z=+el.dataset.zoom, minZ=+el.dataset.min, maxZ=+el.dataset.max;
  var routeShape=[], locationPin=null;
  var shared=new URLSearchParams(location.hash.slice(1));
  function validPoint(a,b){return Number.isFinite(a)&&Number.isFinite(b)&&Math.abs(a)<=85&&Math.abs(b)<=180;}
  if(shared.has('lat')&&shared.has('lon')&&validPoint(+shared.get('lat'),+shared.get('lon'))){
    el.dataset.lat=shared.get('lat');el.dataset.lon=shared.get('lon');
    var sharedZoom=Number(shared.get('z'));if(Number.isFinite(sharedZoom)&&shared.has('z'))z=Math.max(minZ,Math.min(maxZ,Math.round(sharedZoom)));
  }
  if(shared.has('pinlat')&&shared.has('pinlon')&&validPoint(+shared.get('pinlat'),+shared.get('pinlon')))locationPin={lat:+shared.get('pinlat'),lon:+shared.get('pinlon')};
 var overlay=document.createElementNS('http://www.w3.org/2000/svg','svg');overlay.classList.add('map-overlay');el.appendChild(overlay);
 el.addEventListener('map-route',function(e){routeShape=e.detail.filter(function(p){return Number.isFinite(p.Lat)&&Number.isFinite(p.Lon);});if(!routeShape.length){render();return;}var lat=routeShape.reduce(function(a,p){return a+p.Lat;},0)/routeShape.length,lon=routeShape.reduce(function(a,p){return a+p.Lon;},0)/routeShape.length;z=Math.min(maxZ,15);while(z>minZ){var xs=routeShape.map(function(p){return xOf(p.Lon,z)*SIZE;}),ys=routeShape.map(function(p){return yOf(p.Lat,z)*SIZE;});if(Math.max.apply(null,xs)-Math.min.apply(null,xs)<el.clientWidth-40&&Math.max.apply(null,ys)-Math.min.apply(null,ys)<el.clientHeight-40)break;z--;}cx=xOf(lon,z);cy=yOf(lat,z);layer.innerHTML='';live={};render();});
 var live={}, arrived=0, missing=0, asked=0;
  function done(){ say(); }

  // Web Mercator, the same formula the service uses server-side. Kept as a
  // float so the centre can sit anywhere in a tile rather than snapping.
  function xOf(lon,z){ return (lon+180)/360*Math.pow(2,z); }
  function yOf(lat,z){ var r=lat*Math.PI/180;
    return (1-Math.log(Math.tan(r)+1/Math.cos(r))/Math.PI)/2*Math.pow(2,z); }
  function lonOf(x,z){ return x/Math.pow(2,z)*360-180; }
  function latOf(y,z){ var n=Math.PI-2*Math.PI*y/Math.pow(2,z);
    return 180/Math.PI*Math.atan(0.5*(Math.exp(n)-Math.exp(-n))); }

  var cx=xOf(+el.dataset.lon,z), cy=yOf(+el.dataset.lat,z);

  function render(){
    var w=el.clientWidth, h=el.clientHeight, n=Math.pow(2,z);
    // The pixel at the top-left of the viewport, in world pixels.
    var left=cx*SIZE-w/2, top=cy*SIZE-h/2;
    var x0=Math.floor(left/SIZE), y0=Math.floor(top/SIZE);
    var x1=Math.floor((left+w)/SIZE), y1=Math.floor((top+h)/SIZE);
    var seen={};
    for(var y=y0;y<=y1;y++){
      for(var x=x0;x<=x1;x++){
        if(y<0||y>=n) continue;
        var wx=((x%n)+n)%n;            // wrap east-west, so a pan does not run out
        var k=z+'/'+wx+'/'+y;
        seen[k]=true;
        var img=live[k];
        if(!img){
          img=new Image();
          img.className='map-tile';
          img.alt='';
          img.src='/maps/tiles/'+style+'/'+z+'/'+wx+'/'+y+'.png';
          // A tile outside Britain is a 404 and that is normal here, so it
          // fades out rather than showing a broken image.
          img.onerror=function(){ this.classList.add('map-gap'); missing++; done(); };
          img.onload=function(){ arrived++; done(); };
          layer.appendChild(img);
          live[k]=img;
        }
        img.style.left=(x*SIZE-left)+'px';
        img.style.top=(y*SIZE-top)+'px';
      }
    }
    for(var have in live){
      if(!seen[have]){ layer.removeChild(live[have]); delete live[have]; }
    }
    overlay.replaceChildren();if(routeShape.length){var line=document.createElementNS('http://www.w3.org/2000/svg','polyline');line.setAttribute('points',routeShape.map(function(p){return (xOf(p.Lon,z)*SIZE-left)+','+(yOf(p.Lat,z)*SIZE-top);}).join(' '));line.setAttribute('fill','none');line.setAttribute('stroke','#2563eb');line.setAttribute('stroke-width','4');overlay.appendChild(line);}
 if(locationPin){var dot=document.createElementNS('http://www.w3.org/2000/svg','circle');dot.setAttribute('cx',xOf(locationPin.lon,z)*SIZE-left);dot.setAttribute('cy',yOf(locationPin.lat,z)*SIZE-top);dot.setAttribute('r','7');dot.setAttribute('fill','#2563eb');dot.setAttribute('stroke','white');dot.setAttribute('stroke-width','3');var title=document.createElementNS('http://www.w3.org/2000/svg','title');title.textContent='Selected location';dot.appendChild(title);overlay.appendChild(dot);}
 el.dataset.viewLat=latOf(cy,z);el.dataset.viewLon=((lonOf(cx,z)+180)%360+360)%360-180;el.dataset.viewZoom=z;
 if(locationPin){el.dataset.pinLat=locationPin.lat;el.dataset.pinLon=locationPin.lon;}
 asked=Object.keys(live).length;
    say();
  }

  // What the map is looking at, and — when nothing came back — why that might
  // be. A tile that fails is hidden, which is right for the sea around Britain
  // and wrong when every tile fails: a map where nothing loaded then looks
  // exactly like a map still loading, which is how "tiles do not load" becomes
  // a report with nothing in it. So it says so.
  function say(){
    if(!where) return;
    var at=latOf(cy,z).toFixed(4)+', '+lonOf(cx,z).toFixed(4)+'  ·  zoom '+z;
    if(asked>0 && arrived===0 && missing>=asked){
      where.textContent=at+(style==='world'?' · Map tiles could not be loaded.':' · No tiles loaded. This layer covers Britain and requires an Ordnance Survey key.');
      return;
    }
    where.textContent=at;
  }

  function zoomTo(next){
    next=Math.max(minZ,Math.min(maxZ,next));
    if(next===z) return;
    var lat=latOf(cy,z), lon=lonOf(cx,z);
    z=next; cx=xOf(lon,z); cy=yOf(lat,z);
    // Every tile is the wrong size now, so start again rather than reposition.
    layer.innerHTML=''; live={}; arrived=0; missing=0;
    render();
  }

  // Dragging. Pointer events, so a finger and a mouse are the same code.
  var dragging=false, lastX=0, lastY=0;
  el.addEventListener('pointerdown',function(e){
    dragging=true; lastX=e.clientX; lastY=e.clientY;
    el.setPointerCapture(e.pointerId); el.classList.add('map-dragging');
  });
  el.addEventListener('pointermove',function(e){
    if(!dragging) return;
    cx-=(e.clientX-lastX)/SIZE; cy-=(e.clientY-lastY)/SIZE;
    lastX=e.clientX; lastY=e.clientY;
    render();
  });
  function stop(e){ dragging=false; el.classList.remove('map-dragging');
    if(e&&e.pointerId!==undefined&&el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId); }
  el.addEventListener('pointerup',stop);
  el.addEventListener('pointercancel',stop);

  el.addEventListener('wheel',function(e){ e.preventDefault(); zoomTo(z+(e.deltaY<0?1:-1)); },{passive:false});
  el.addEventListener('dblclick',function(){ zoomTo(z+1); });

  var zin=document.getElementById('map-in'), zout=document.getElementById('map-out');
  if(zin) zin.onclick=function(){ zoomTo(z+1); };
  if(zout) zout.onclick=function(){ zoomTo(z-1); };

  // Where you are, asked for rather than taken. The browser prompts, and a
  // refusal leaves the map where it was rather than saying anything: somebody
  // who declines has answered the question.
  var here=document.getElementById('map-here');
  if(here) here.onclick=function(){
    if(!navigator.geolocation) return;
    here.disabled=true;
    navigator.geolocation.getCurrentPosition(function(p){
      here.disabled=false;
      locationPin={lat:p.coords.latitude,lon:p.coords.longitude};
      z=Math.max(z,14);
      cx=xOf(p.coords.longitude,z); cy=yOf(p.coords.latitude,z);
      layer.innerHTML=''; live={}; arrived=0; missing=0; render();
    },function(){ here.disabled=false; },{timeout:10000});
  };

  window.addEventListener('resize',render);
  render();
})();

}

if (typeof document !== "undefined") {
(function(){var form=document.getElementById('map-directions'),out=document.getElementById('map-directions-result');if(!form||form.dataset.wired)return;form.dataset.wired='1';var request=0;
document.getElementById('map-start-here').onclick=function(){if(!navigator.geolocation){out.textContent='Location is unavailable. Enter a starting point.';return;}navigator.geolocation.getCurrentPosition(function(p){form.elements.from.value=p.coords.latitude+','+p.coords.longitude;},function(){out.textContent='Could not get your location. Enter a starting point.';},{timeout:10000});};
form.onsubmit=async function(e){e.preventDefault();var id=++request;document.getElementById('map').dispatchEvent(new CustomEvent('map-route',{detail:[]}));out.textContent='Finding directions…';try{var q=new URLSearchParams(new FormData(form)),r=await fetch('/routes',{method:'POST',body:q,headers:{Accept:'application/json'}}),d=await r.json();if(id!==request)return;if(!r.ok)throw new Error(d.error||'Directions are unavailable.');out.replaceChildren();if(d.estimate){out.textContent='Road directions are unavailable on this instance. A straight-line estimate is not navigation.';return;}var summary=document.createElement('p');summary.textContent=d.summary;out.appendChild(summary);var list=document.createElement('ol');(d.steps||[]).forEach(function(s){var item=document.createElement('li');item.textContent=s.Text;list.appendChild(item);});out.appendChild(list);document.getElementById('map').dispatchEvent(new CustomEvent('map-route',{detail:d.shape||[]}));}catch(e){if(id===request)out.textContent=e.message||'Could not load directions.';}};
})();
}

if (typeof document !== "undefined" && document.getElementById("app-editor-state")) {
const initial=JSON.parse(document.getElementById("app-editor-state").textContent);

var codeEl = document.getElementById('code');
var preview = document.getElementById('preview');
var appIcon = initial.icon;
var editSlug = initial.slug;

// Pre-populate fields
codeEl.value = initial.html;
document.getElementById('appName').value = initial.name;
document.getElementById('appSlugInput').value = editSlug;
document.getElementById('appDesc').value = initial.description;
document.getElementById('appTags').value = initial.tags;
document.getElementById('appPublic').checked = initial.public;
document.getElementById('appPrice').value = initial.price;
showPreview();

codeEl.addEventListener('keydown', function(e) {
  if (e.key === 'Tab') {
    e.preventDefault();
    var start = this.selectionStart, end = this.selectionEnd;
    this.value = this.value.substring(0, start) + '  ' + this.value.substring(end);
    this.selectionStart = this.selectionEnd = start + 2;
  }
});

function showPreview() {
  var html = codeEl.value;
  if (!html.trim()) return;
  preview.style.display = '';
  preview.srcdoc = html;
}

function updatePreview() { showPreview(); }

function saveApp() {
  var name = document.getElementById('appName').value.trim();
  var newSlug = document.getElementById('appSlugInput').value.trim().toLowerCase().replace(/[^a-z0-9-]/g,'').replace(/^-|-$/g,'');
  var desc = document.getElementById('appDesc').value.trim();
  var tags = (document.getElementById('appTags').value || '').trim();
  var html = codeEl.value.trim();
  if (!name) { document.getElementById('statusMsg').textContent = 'App name is required'; return; }
  if (!html) { document.getElementById('statusMsg').textContent = 'No code to save'; return; }

  // If slug changed, rename first
  if (newSlug && newSlug !== editSlug) {
    document.getElementById('statusMsg').textContent = 'Renaming...';
    fetch('/apps/' + editSlug, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ slug: newSlug })
    }).then(function(r){ return r.json(); }).then(function(data){
      if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
      editSlug = data.slug || newSlug;
      document.getElementById('appSlugInput').value = editSlug;
      doSave();
    }).catch(function(e){ document.getElementById('statusMsg').textContent = e.message; });
    return;
  }
  doSave();
  function doSave() {
  document.getElementById('statusMsg').textContent = 'Saving...';
  fetch('/apps/' + editSlug, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, icon: appIcon, description: desc, tags: tags, html: html, public: document.getElementById('appPublic').checked, price: parseInt(document.getElementById('appPrice').value)||0 })
  })
  .then(function(r) {
    if (!r.ok) {
      return r.text().then(function(t) {
        try { var j = JSON.parse(t); throw new Error(j.error || 'Save failed'); }
        catch(e) { if (e.message) throw e; throw new Error('Save failed (status ' + r.status + ')'); }
      });
    }
    return r.json();
  })
  .then(function(data) {
    if (data.error) { document.getElementById('statusMsg').textContent = data.error; return; }
    var now = new Date();
    var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
    var ts = now.getDate() + ' ' + months[now.getMonth()] + ' ' + now.getFullYear() + ' ' + String(now.getHours()).padStart(2,'0') + ':' + String(now.getMinutes()).padStart(2,'0');
    document.getElementById('savedAt').textContent = 'Last saved ' + ts;
    document.getElementById('statusMsg').textContent = 'Saved!';
    setTimeout(function() { document.getElementById('statusMsg').textContent = ''; }, 3000);
  })
  .catch(function(e) { document.getElementById('statusMsg').textContent = e.message || 'Save failed'; });
  } // end doSave
}

function copyCode() {
  navigator.clipboard.writeText(codeEl.value).then(function() {
    document.getElementById('statusMsg').textContent = 'Copied!';
    setTimeout(function() { document.getElementById('statusMsg').textContent = ''; }, 2000);
  });
}

function deleteApp() {
  if (!confirm('Delete this app? This cannot be undone.')) return;
  fetch('/apps/' + editSlug + '/delete', { method: 'POST' })
  .then(function(r) { if (r.ok) window.location.href = '/apps'; else throw new Error('Delete failed'); })
  .catch(function(e) { document.getElementById('statusMsg').textContent = e.message; });
}

const actions={deleteApp,copyCode,updatePreview,saveApp};
document.querySelectorAll('[data-app-action]').forEach(button=>button.addEventListener('click',()=>actions[button.dataset.appAction]?.()));
}

if(typeof document!=='undefined'){
 const path=location.pathname;
 if(['/maps','/routes','/transit','/web'].includes(path)){
  const main=document.querySelector('main');
  if(main){
   const bar=document.createElement('div');bar.className='view-share';
   const button=document.createElement('button');button.type='button';button.textContent='Share link';
   const status=document.createElement('span');status.setAttribute('role','status');bar.append(button,status);main.appendChild(bar);
   const shared=new URLSearchParams(location.hash.slice(1));
   const form=path==='/maps'?document.getElementById('map-directions'):path==='/routes'?document.querySelector('form[action="/routes"]'):null;
   if(form)for(const key of ['from','to','mode'])if(shared.has(key)&&form.elements[key])form.elements[key].value=shared.get(key);
   button.addEventListener('click',async()=>{
    const url=new URL(location.href);url.hash='';const state=new URLSearchParams();
    if(form){url.search='';for(const key of ['from','to','mode'])if(form.elements[key]?.value)state.set(key,form.elements[key].value);}
    if(path==='/maps'){
     const map=document.getElementById('map');if(map){url.search='';url.searchParams.set('style',map.dataset.style);for(const [key,attr] of [['lat','viewLat'],['lon','viewLon'],['z','viewZoom'],['pinlat','pinLat'],['pinlon','pinLon']])if(map.dataset[attr])state.set(key,map.dataset[attr]);}
    }
    if(path==='/transit'){url.search='';const q=document.getElementById('xquery')?.value.trim();if(q)state.set('q',q);const stop=document.getElementById('xstops')?.dataset.selectedStop;if(stop)state.set('stop',stop);}
    url.hash=state.toString();
    try{if(navigator.share)await navigator.share({title:document.title,url:url.href});else{await navigator.clipboard.writeText(url.href);status.textContent='Link copied';}}catch(error){if(error.name!=='AbortError'){status.textContent='Copy this link: ';const a=document.createElement('a');a.href=url.href;a.textContent=url.href;status.append(a);}}
   });
  }
 }
}
