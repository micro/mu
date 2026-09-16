if (typeof document === 'undefined') {
var APP_PREFIX = 'mu_';
var VERSION = 'v2-conversation';
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
  return m ? m[1] : '';
}

(function() {
  var _fetch = window.fetch;
  window.fetch = function(url, opts) {
    opts = opts || {};
    var method = (opts.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
      opts.headers = opts.headers || {};
      if (opts.headers instanceof Headers) {
        if (!opts.headers.has('X-CSRF-Token')) {
          opts.headers.set('X-CSRF-Token', getCsrfToken());
        }
      } else {
        if (!opts.headers['X-CSRF-Token']) {
          opts.headers['X-CSRF-Token'] = getCsrfToken();
        }
      }
    }
    return _fetch.call(this, url, opts);
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
/* Account menu follows the shared disclosure interaction on every page. */
if (typeof document !== "undefined") {
document.addEventListener('click', function(event) {
  var menu=document.querySelector('.nav-account-disclosure[open]');
  if(menu && !menu.contains(event.target)) menu.open=false;
});
document.addEventListener('keydown', function(event) {
  if(event.key!=='Escape') return;
  var menu=document.querySelector('.nav-account-disclosure[open]');
  if(menu){menu.open=false;var summary=menu.querySelector('summary');if(summary)summary.focus();}
});

}
