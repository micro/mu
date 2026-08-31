// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v154';
var CACHE_NAME = APP_PREFIX + VERSION;

// Minimal caching - only icons
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

// ============================================
// SERVICE WORKER EVENT LISTENERS
// ============================================

self.addEventListener('fetch', function (e) {
  // Let browser handle all fetches naturally - only cache icons
  const url = new URL(e.request.url);
  
  if (e.request.method !== 'GET') {
    return;
  }
  
  // Only intercept icons
  if (url.pathname.match(/\.(png|jpg|jpeg|gif|svg|ico)$/)) {
    e.respondWith(
      caches.match(e.request).then(cached => cached || fetch(e.request))
    );
  }
});

// Installing must not be able to fail.
//
// This was caches.addAll(STATIC_CACHE), and addAll rejects the whole batch if a
// single request fails. A rejected promise in an install event's waitUntil
// means the worker does not install — so it never activates, never handles a
// push, and nothing anywhere says so. The page registers happily, the push
// service accepts every notification, and the handset has no worker to wake.
//
// That is not hypothetical. '/reminder.svg' was in this list and had been a 404
// on the live instance for who knows how long: an icon nothing else references,
// left behind by a rename. One missing decoration silently switched off every
// notification on every device, and the record could only say "sent — the
// device has not said it arrived", which is also what a phone in a tunnel looks
// like. Days went into the sending half, which was correct throughout.
//
// So each file is added on its own and a failure is swallowed. These are icons.
// The push handler below is the product, and it must not be hostage to whether
// a decoration is still where somebody left it. skipWaiting runs either way.
self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(function (cache) {
      return Promise.all(STATIC_CACHE.map(function (url) {
        return cache.add(url).catch(function () {});
      }));
    }).then(function () {
      return self.skipWaiting();
    }).catch(function () {
      // Even the cache being unavailable — a private window, storage denied —
      // must not stop the worker taking over.
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

// A notification arriving while nothing is open.
//
// This is the half of the product that was invisible on a phone: mail turns up
// at four in the morning, the agent answers it, and the only way to find out
// was to open the site and look. The payload is encrypted end to end — the push
// service forwards bytes it cannot read — so it is decrypted here and nowhere
// else. See internal/push.
//
// # Nothing here may fail quietly
//
// This read the payload, and returned without doing anything if it could not
// find a title. That is the one branch that must never exist in a service
// worker: the server has no way to see this code, so a silent return produces
// "the push service accepted it and nothing appeared" — which is
// indistinguishable, from the server, from never having sent it. Hours go into
// the wrong half of the system.
//
// So every path ends in a notification, and every path posts a receipt. A push
// that arrives mangled says so on the handset and in the record.
self.addEventListener('push', function (e) {
  var n = {}, why = '';
  try { n = e.data ? e.data.json() : {}; } catch (err) { why = 'the payload could not be read'; }
  if (!why && !n.title) why = 'the payload had no title';

  var title = n.title || 'Notification unreadable';
  var body = why ? ('This device woke up but could not read it: ' + why + '.') : (n.body || '');
  var tag = n.tag || 'mu';

  e.waitUntil(
    self.registration.showNotification(title, {
      body: body,
      icon: '/icon-192.png',
      badge: '/icon-192.png',
      // Two arrivals in one conversation replace each other rather than stack.
      tag: tag,
      renotify: true,
      data: {url: n.url || '/inbox'}
    }).then(function () {
      return receipt(tag, !why, why);
    }, function (err) {
      // showNotification itself refused — permission revoked under us, or the
      // OS is suppressing. Worth recording precisely because there is nothing
      // on the screen to notice.
      return receipt(tag, false, 'showNotification failed: ' + (err && err.message ? err.message : 'unknown'));
    })
  );
});

// Tell the server this device woke up. Best effort: a receipt that fails must
// never stop the notification.
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

// Tapping it goes where it is about. A notification you cannot act on trains
// somebody to ignore the next one.
//
// An open tab is focused rather than a second one opened, which is what makes
// this feel like an app rather than a series of windows.
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

// Which copy of this file is actually running on this device.
//
// A push that the push service accepts and the handset never shows leaves no
// trace anywhere: no notification, and — if the worker predates the receipt
// code — no receipt either. Five sent, five accepted, nothing back, and no way
// from here to tell "the worker did not wake" from "the worker is an old one
// that cannot say it woke". Those need completely different fixes and looked
// identical.
//
// So the worker answers when asked. A page posts {mu: 'version'} and gets the
// VERSION back. No reply means the worker on this device is older than this
// line, which is itself the answer — and the card says so and offers the
// update rather than leaving somebody to guess.
self.addEventListener('message', function (e) {
  if (!e.data || e.data.mu !== 'version') return;
  var reply = {mu: 'version', version: VERSION};
  if (e.ports && e.ports[0]) { e.ports[0].postMessage(reply); return; }
  if (e.source && e.source.postMessage) e.source.postMessage(reply);
});

// ============================================
// PAGE JAVASCRIPT (only run in window context)
// ============================================

// Exit early if we're in service worker context
if (typeof document === 'undefined') {
  // We're in service worker context, don't execute page code
  // Service worker code above will still run
} else {
  // We're in window context, execute page code

// ============================================
// CSRF PROTECTION
// ============================================

// Read CSRF token from cookie set by the server.
function getCsrfToken() {
  var m = document.cookie.match('(?:^|; )csrf_token=([^;]*)');
  return m ? m[1] : '';
}

// Monkey-patch fetch to auto-include CSRF token on state-changing requests.
(function() {
  var _fetch = window.fetch;
  window.fetch = function(url, opts) {
    opts = opts || {};
    var method = (opts.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
      opts.headers = opts.headers || {};
      // Support both Headers object and plain object
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

// Auto-inject CSRF hidden field into ALL form submissions (including dynamic forms).
// Uses a capturing submit listener on document so it fires before the form submits.
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

// Dismiss open tooltips when tapping elsewhere.
document.addEventListener('click', function() {
  document.querySelectorAll('.card-tooltip.show').forEach(function(e) {
    e.classList.remove('show');
  });
});

// ============================================
// TIMESTAMP UPDATES
// ============================================

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

// Update timestamps immediately and then every minute
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', function() {
    updateTimestamps();
    setInterval(updateTimestamps, 60000);
  });
} else {
  updateTimestamps();
  setInterval(updateTimestamps, 60000);
}

// ============================================
// MICRO DIALOG
// ============================================

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}

function formatMicroResponse(text) {
  // Basic markdown-like formatting
  return escapeHtml(text)
    .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    .replace(/\n/g, '<br>');
}

// Close dialog on Escape key
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeMicroDialog();
  }
});

// ============================================
// CHAT FUNCTIONALITY
// ============================================

// Constants
const TOPICS_SELECTOR = '#topics .head';
const CHAT_PATH = '/chat';

var isAuthenticated = false;
var topic = '';


// setTopic records which room the message box is posting into.
//
// It used to also write the room's name into #messages as a bold line, which
// was the third place the page said it: the shell renders the title as the
// page heading, the server renders the About block under it, and then this put
// it at the top of the conversation as well. Naming the room once is enough,
// and the heading is where a reader looks for it.
//
// The toggle that came with that block is gone too. It hid the summary by
// setting style.display, which .d-none would have beaten silently — the About
// block folds with <details> now, which cannot be argued with by the cascade.
function setTopic(t) {
  topic = t;

  const topicInput = document.getElementById('topic');
  if (topicInput) {
    topicInput.value = t;
  }
}

// loadChat joins the room named in the URL. Without one, /chat is the rooms
// list — a server-rendered page with nothing to connect to, so there is
// nothing for this to do.
//
// It used to also drive a topic picker and a question box that posted to the
// server for a model answer. Both are gone: chat is people talking to each
// other, and a question for a model goes to /agent.
function loadChat() {
  const urlParams = new URLSearchParams(window.location.search);
  const roomId = urlParams.get('id');
  if (!roomId) {
    return;
  }

  // A topic room is named for its topic, so the heading can come from the id.
  if (roomId.startsWith('chat_')) {
    setTopic(roomId.replace('chat_', ''));
  }

  // Sending needs an account, so the box reflects that before we connect.
  updateChatFormState();

  // Connect after the auth check completes — sending needs an account.
  setTimeout(() => {
    if (isAuthenticated) {
      connectRoomWebSocket(roomId);
      bindRoomForm();
    }
  }, 500);
}

// bindRoomForm points the message box at the websocket. The form has no action
// of its own, so until this runs it does nothing rather than posting somewhere.
function bindRoomForm() {
  const chatForm = document.getElementById('chat-form');
  if (chatForm) {
    chatForm.onsubmit = function(e) {
      e.preventDefault();
      sendRoomMessage(this);
      return false;
    };
  }
}

// ============================================
// VIDEO FUNCTIONALITY
// ============================================

async function getVideos(el) {
  if (!isAuthenticated) {
    showToast('Please login to search', 'error');
    return false;
  }

  const formData = new FormData(el);
  const data = {};
  for (let [key, value] of formData.entries()) {
    data[key] = value;
  }

  const result = await apiCall('/video', { body: data });
  
  if (result.ok && result.data.html) {
    let d = document.getElementById('results');
    if (!d) {
      d = document.createElement('div');
      d.id = 'results';
      const content = document.getElementById('content');
      content.innerHTML += '<h1>Results</h1>';
      content.appendChild(d);
    } else {
      d.innerHTML = '';
    }
    d.innerHTML = result.data.html;
    document.getElementById('query').value = data.query;
  }

  return false;
}

// ============================================
// SESSION MANAGEMENT
// ============================================

function getCookie(name) {
  var match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  if (match) return match[2];
}

function setSession() {
  fetch("/session", {
    method: "POST",
    headers: {
      'Content-Type': 'application/json'
    },
  }).then(response => {
    if (!response.ok) {
      throw new Error('Not authenticated');
    }
    return response.text().then(text => {
      try {
        return JSON.parse(text);
      } catch (e) {
        console.error('Failed to parse session response:', text.substring(0, 100));
        throw new Error('Invalid session response');
      }
    });
  })
  .then(sess => {
    console.log('Success:', sess);
    // Nav elements (sidebar)
    var navAccount = document.getElementById("nav-account");
    var navLogout = document.getElementById("nav-logout");
    var navLogin = document.getElementById("nav-login");
    var navUsername = document.getElementById("nav-username");
    var navAdmin = document.getElementById("nav-admin");

    if (sess.type == "account") {
      isAuthenticated = true;
      // Show authenticated nav items
      if (navAccount) navAccount.style.display = 'flex';
      if (navLogout) navLogout.style.display = 'flex';
      if (navLogin) navLogin.style.display = 'none';
      if (navUsername && sess.account) {
        navUsername.textContent = '@' + sess.account;
        navUsername.style.display = 'block';
        var navMeAv = document.getElementById("nav-me-av");
        if (navMeAv) navMeAv.textContent = sess.account.charAt(0).toUpperCase();
      }
      // Rendered server-side for an admin so the link works without JS, and
      // corrected here for the same reason nav-username is: a page cached for
      // one viewer must not hand the next one a door that is not theirs.
      if (navAdmin) navAdmin.style.display = sess.admin ? 'flex' : 'none';
      // Show the wallet link and badge its credit balance for logged-in users.
      document.body.classList.add('signed-in');
      // How many conversations are waiting, for the envelope in the header.
      // Initialize card customization for home page
      if (window.location.pathname === '/home') {
        initCardCustomization();
      }
    } else {
      isAuthenticated = false;
      if (navAdmin) navAdmin.style.display = 'none';
      // Hide authenticated nav items, show login
      if (navAccount) navAccount.style.display = 'none';
      if (navLogout) navLogout.style.display = 'none';
      if (navLogin) {
        navLogin.style.display = 'flex';
        // Update login link to include redirect parameter
        if (window.location.pathname !== '/login' && window.location.pathname !== '/signup' && window.location.pathname !== '/') {
          const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
          navLogin.href = '/login?redirect=' + redirectUrl;
        }
      }
    }
    updateChatFormState();
    updateSearchFormsState();
  })
  .catch(error => {
    console.error('Error:', error);
    isAuthenticated = false;
    var navAccount = document.getElementById("nav-account");
    var navLogout = document.getElementById("nav-logout");
    var navLogin = document.getElementById("nav-login");
    var navAdmin = document.getElementById("nav-admin");
    if (navAdmin) navAdmin.style.display = 'none';
    if (navAccount) navAccount.style.display = 'none';
    if (navLogout) navLogout.style.display = 'none';
    if (navLogin) {
      navLogin.style.display = 'flex';
      // Update login link to include redirect parameter
      if (window.location.pathname !== '/login' && window.location.pathname !== '/signup' && window.location.pathname !== '/') {
        const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
        navLogin.href = '/login?redirect=' + redirectUrl;
      }
    }
    
    updateChatFormState();
    updateSearchFormsState();
  });
}

function updateChatFormState() {
  const chatPrompt = document.getElementById('prompt');
  const chatButton = document.querySelector('#chat-form button');
  
  if (chatPrompt && chatButton) {
    if (isAuthenticated) {
      chatPrompt.placeholder = 'Say something';
      chatPrompt.disabled = false;
      chatButton.disabled = false;
      chatPrompt.style.cursor = '';
      chatButton.style.cursor = '';
      chatPrompt.onclick = null;
      chatButton.onclick = null;
    } else {
      chatPrompt.placeholder = 'Log in to join the discussion';
      chatPrompt.disabled = true;
      chatButton.disabled = true;
      chatPrompt.style.cursor = 'pointer';
      chatButton.style.cursor = 'pointer';
      const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
      const redirectToLogin = () => window.location.href = '/login?redirect=' + redirectUrl;
      chatPrompt.onclick = redirectToLogin;
      chatButton.onclick = redirectToLogin;
    }
  }
}

function updateSearchFormsState() {
  // Update news search form
  const newsQuery = document.getElementById('news-query');
  const newsBtn = document.getElementById('news-search-btn');
  
  if (newsQuery && newsBtn) {
    if (isAuthenticated) {
      newsQuery.placeholder = 'Search news';
      newsQuery.disabled = false;
      newsBtn.disabled = false;
      newsQuery.style.cursor = '';
      newsBtn.style.cursor = '';
      newsQuery.onclick = null;
      newsBtn.onclick = null;
    } else {
      newsQuery.placeholder = 'Login to search';
      newsQuery.disabled = true;
      newsBtn.disabled = true;
      newsQuery.style.cursor = 'pointer';
      newsBtn.style.cursor = 'pointer';
      const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
      const redirectToLogin = () => window.location.href = '/login?redirect=' + redirectUrl;
      newsQuery.onclick = redirectToLogin;
      newsBtn.onclick = redirectToLogin;
    }
  }
  
  // Update video search form
  const videoQuery = document.getElementById('query');
  const videoBtn = document.getElementById('video-search-btn');
  
  if (videoQuery && videoBtn) {
    if (isAuthenticated) {
      videoQuery.disabled = false;
      videoBtn.disabled = false;
      videoQuery.style.cursor = '';
      videoBtn.style.cursor = '';
      videoQuery.onclick = null;
      videoBtn.onclick = null;
    } else {
      videoQuery.placeholder = 'Login to search';
      videoQuery.disabled = true;
      videoBtn.disabled = true;
      videoQuery.style.cursor = 'pointer';
      videoBtn.style.cursor = 'pointer';
      const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
      const redirectToLogin = () => window.location.href = '/login?redirect=' + redirectUrl;
      videoQuery.onclick = redirectToLogin;
      videoBtn.onclick = redirectToLogin;
    }
  }
}

// ============================================
// EVENT LISTENERS
// ============================================

function highlightTopic(topicName) {
  // Specific selectors for topic elements
  const selectors = [TOPICS_SELECTOR];
  
  // Cache all matching elements to avoid multiple DOM queries
  const allTopicLinks = [];
  selectors.forEach(selector => {
    const elements = document.querySelectorAll(selector);
    allTopicLinks.push(...elements);
  });
  
  // Remove active from all
  allTopicLinks.forEach(link => {
    link.classList.remove('active');
  });
  
  // Cache the hash string to avoid repeated concatenation
  const hashString = '#' + topicName;
  
  // Add active class to the matching topic
  allTopicLinks.forEach(link => {
    const href = link.getAttribute('href');
    if (link.textContent === topicName || (href && href.endsWith(hashString))) {
      link.classList.add('active');
    }
  });
}

function handleHashChange() {
  if (!window.location.hash) return;
  
  const hash = window.location.hash.substring(1);
  console.log('Hash changed to:', hash);
  
  // Highlight the matching topic/tag
  highlightTopic(hash);
}

self.addEventListener("hashchange", handleHashChange);

self.addEventListener("popstate", handleHashChange);

self.addEventListener('DOMContentLoaded', function() {
  // Listen for service worker updates
  if (navigator.serviceWorker) {
    navigator.serviceWorker.addEventListener('message', event => {
      if (event.data && event.data.type === 'SW_UPDATED') {
        console.log('Service worker updated to version:', event.data.version);
        // Reload the page to get fresh content
        window.location.reload();
      }
    });
  }

  // Prevent page scroll on topic clicks for mobile chat
  const topicsDiv = document.getElementById('topics');
  const messagesBox = document.getElementById('messages');
  
  if (topicsDiv && messagesBox && window.innerWidth <= 600) {
    topicsDiv.addEventListener('click', function(e) {
      if (e.target.tagName === 'A' && e.target.hash) {
        e.preventDefault();
        const targetId = e.target.hash.substring(1);
        const targetElement = document.getElementById(targetId);
        if (targetElement) {
          // Scroll only the messages box
          const offset = targetElement.offsetTop - messagesBox.offsetTop;
          messagesBox.scrollTop = offset - 10; // 10px offset for spacing
          // Update hash without scrolling
          history.replaceState(null, null, e.target.hash);
        }
      }
    });
  }
  
  // set nav active state
  var nav = document.getElementById("nav");
  var navContainer = document.getElementById("nav-container");
  var navLinks = nav.querySelectorAll("a[href]");
  for (var i = 0; i < navLinks.length; i++) {
    var el = navLinks[i];
    if (el.id === 'nav-more-toggle') continue;
    if (el.getAttribute("href") == window.location.pathname) {
      el.classList.add("active");
    } else {
      el.classList.remove("active");
    }
  }
  

  // load chat
  if (window.location.pathname == CHAT_PATH) {
    loadChat();
  }
  
  // Handle hash on page load for topic highlighting (non-chat pages)
  if (window.location.hash && window.location.pathname !== CHAT_PATH) {
    handleHashChange();
  }
  
  // Prevent news search form submission when not authenticated
  const newsSearchForm = document.getElementById('news-search');
  if (newsSearchForm) {
    newsSearchForm.addEventListener('submit', function(e) {
      if (!isAuthenticated) {
        e.preventDefault();
        showToast('Please login to search', 'error');
        return false;
      }
    });
  }
  
  // Prevent video search form submission when not authenticated
  const videoSearchForm = document.getElementById('video-search');
  if (videoSearchForm) {
    videoSearchForm.addEventListener('submit', function(e) {
      if (!isAuthenticated) {
        e.preventDefault();
        showToast('Please login to search', 'error');
        return false;
      }
    });
  }

  // Check session status on page load
  setSession();
});

// Custom confirm dialog (PWA-compatible replacement for window.confirm)
window.muConfirm = function(message) {
  return new Promise(function(resolve) {
    var overlay = document.createElement('div');
    overlay.className = 'modal';
    overlay.innerHTML = '<div class="modal-content" style="max-width:400px;text-align:center;">' +
      '<p style="margin-bottom:20px;">' + message + '</p>' +
      '<div style="display:flex;gap:10px;">' +
      '<button id="mu-confirm-no" style="flex:1;padding:10px;border:1px solid #ccc;border-radius:5px;background:#f5f5f5;color:black;cursor:pointer;">Cancel</button>' +
      '<button id="mu-confirm-yes" style="flex:1;padding:10px;border:none;border-radius:5px;background:#dc3545;color:white;cursor:pointer;">Confirm</button>' +
      '</div></div>';
    document.body.appendChild(overlay);
    overlay.querySelector('#mu-confirm-yes').onclick = function() { overlay.remove(); resolve(true); };
    overlay.querySelector('#mu-confirm-no').onclick = function() { overlay.remove(); resolve(false); };
    overlay.addEventListener('click', function(e) { if (e.target === overlay) { overlay.remove(); resolve(false); } });
  });
};

// Flag a post (assigned to window for onclick access)
window.flagPost = async function flagPost(postId) {
  if (!await muConfirm('Flag this post as inappropriate? It will be hidden after 3 flags.')) {
    return;
  }

  const result = await apiCall('/flag', { body: { type: 'post', id: postId } });

  if (!result.ok) {
    return; // apiCall already shows error toast
  }

  if (result.data.success) {
    showToast('Post flagged. Flag count: ' + result.data.count, 'success');
    if (result.data.count >= 3) {
      setTimeout(() => location.reload(), 1000);
    }
  } else {
    showToast(result.data.message || 'Could not flag post', 'error');
  }
}




// ============================================
// CHAT ROOM WEBSOCKET
// ============================================

let roomWs;
let currentRoomId = null;

function getRoomStorageKey(roomId) {
  return 'mu_chat_room_' + roomId;
}

function saveMessageToStorage(roomId, msg) {
  const key = getRoomStorageKey(roomId);
  const messages = JSON.parse(localStorage.getItem(key) || '[]');
  messages.push(msg);
  // Keep last 100 messages per room
  if (messages.length > 100) {
    messages.shift();
  }
  localStorage.setItem(key, JSON.stringify(messages));
}

function loadMessagesFromStorage(roomId) {
  const key = getRoomStorageKey(roomId);
  return JSON.parse(localStorage.getItem(key) || '[]');
}

function connectRoomWebSocket(roomId) {
  // Don't reconnect if already connected to this room
  if (roomWs && roomWs.readyState === WebSocket.OPEN && currentRoomId === roomId) {
    return;
  }
  
  if (roomWs) {
    roomWs.close();
  }
  
  const isReconnect = currentRoomId === roomId;
  currentRoomId = roomId;
  
  // Only clear messages on initial connect, not on reconnect. Nothing has to be
  // preserved across the clear any more: what the room is about is rendered by
  // the server above #messages, outside everything this touches.
  if (!isReconnect) {
    const messagesDiv = document.getElementById('messages');
    if (messagesDiv) {
      messagesDiv.innerHTML = '';
    }
  }
  
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  roomWs = new WebSocket(protocol + '//' + window.location.host + '/chat?id=' + roomId);
  
  roomWs.onopen = function() {
    console.log('Connected to room:', roomId);
  };
  
  roomWs.onmessage = function(event) {
    const msg = JSON.parse(event.data);
    
    if (msg.type === 'user_list') {
      updateUserList(msg.users);
    } else {
      saveMessageToStorage(roomId, msg);
      displayRoomMessage(msg, true);
    }
  };
  
  roomWs.onclose = function() {
    console.log('Disconnected from room');
    // Only reconnect if authenticated and still on same room
    if (isAuthenticated && currentRoomId === roomId) {
      setTimeout(() => connectRoomWebSocket(roomId), 3000);
    }
  };
  
  roomWs.onerror = function(error) {
    console.error('WebSocket error:', error);
  };
}

function displayRoomMessage(msg, shouldScroll = true) {
  const messagesDiv = document.getElementById('messages');
  if (!messagesDiv) return;
  
  const msgDiv = document.createElement('div');
  msgDiv.className = 'message';
  
  const userSpan = msg.is_llm ?
    '<span class="llm"><a href="/@micro" style="color:inherit;text-decoration:none;">micro</a></span>' :
    '<span class="you"><a href="/@' + msg.username + '">' + msg.username + '</a></span>';
  
  let content;
  if (msg.is_llm) {
    // Markdown, which brings its own <p>, <ul> and headings — so no wrapper of
    // our own. It used to be wrapped in one, which was fine while the renderer
    // only produced inline tags and became <p><h4>…</h4></p> the moment it
    // produced a block: invalid, and the browser takes the message apart to
    // fix it.
    content = renderMarkdown(msg.content);
  } else {
    // Escape HTML and linkify URLs for user messages
    content = msg.content.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    content = linkifyText(content);
    content = '<p>' + content.replace(/\n/g, '<br>') + '</p>';
  }

  msgDiv.innerHTML = userSpan + content;
  messagesDiv.appendChild(msgDiv);
  
  if (shouldScroll) {
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
  }
}

// Linkify URLs in text
function linkifyText(text) {
  const urlRegex = /(https?:\/\/[^\s]+)/g;
  return text.replace(urlRegex, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>');
}

// Markdown, as much of it as a model actually writes.
//
// The old renderer did bold, italic, code, links and line breaks — and a model
// answering a question about a news article writes headings, bullet lists and
// numbered steps, none of which were handled. Those came through as literal
// "##" and "-" down the left of the message, which is what "it's in markdown
// and not formatted" meant.
//
// It also went straight to innerHTML without escaping. The text is written by a
// model that has just been handed whatever somebody typed in a shared room, so
// "reply with exactly <img src=x onerror=...>" was a live path to script in
// another person's page. Escaping is first now, and every rule below runs on
// text that can no longer contain a tag of its own.
function renderMarkdown(text) {
  var esc = function (s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  };

  // Fenced code comes out first and goes back last, so nothing below rewrites
  // what is inside it — a hash in a shell comment is not a heading.
  var fences = [];
  var src = esc(text).replace(/```[a-z]*\n?([\s\S]*?)```/g, function (_, code) {
    fences.push(code);
    return ' FENCE' + (fences.length - 1) + ' ';
  });

  var inline = function (s) {
    return s
      .replace(/`([^`]+?)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+?)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*([^*]+?)\*/g, '$1<em>$2</em>')
      // Only http(s), so a [click](javascript:...) is left as the text it is.
      .replace(/\[([^\]]+?)\]\((https?:\/\/[^)\s]+?)\)/g,
        '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  };

  var out = [];
  var list = null; // 'ul' or 'ol' while one is open
  var closeList = function () {
    if (list) { out.push('</' + list + '>'); list = null; }
  };
  var openList = function (kind) {
    if (list !== kind) { closeList(); out.push('<' + kind + '>'); list = kind; }
  };

  var lines = src.split('\n');
  var para = [];
  var flushPara = function () {
    if (para.length) { out.push('<p>' + inline(para.join('<br>')) + '</p>'); para = []; }
  };

  for (var i = 0; i < lines.length; i++) {
    var line = lines[i];
    var fence = line.match(/^ FENCE(\d+) $/);
    if (fence) {
      flushPara(); closeList();
      out.push('<pre><code>' + fences[+fence[1]] + '</code></pre>');
      continue;
    }
    if (!line.trim()) { flushPara(); closeList(); continue; }

    var heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      flushPara(); closeList();
      // Capped at h4: these sit inside a message, not at the top of a page.
      var level = Math.min(heading[1].length + 2, 4);
      out.push('<h' + level + '>' + inline(heading[2]) + '</h' + level + '>');
      continue;
    }
    var bullet = line.match(/^\s*[-*+]\s+(.*)$/);
    if (bullet) {
      flushPara(); openList('ul');
      out.push('<li>' + inline(bullet[1]) + '</li>');
      continue;
    }
    var numbered = line.match(/^\s*\d+[.)]\s+(.*)$/);
    if (numbered) {
      flushPara(); openList('ol');
      out.push('<li>' + inline(numbered[1]) + '</li>');
      continue;
    }
    // &gt; rather than >, because escaping has already happened by here.
    var quote = line.match(/^&gt;\s?(.*)$/);
    if (quote) {
      flushPara(); closeList();
      out.push('<blockquote>' + inline(quote[1]) + '</blockquote>');
      continue;
    }
    if (/^\s*([-*_])\s*\1\s*\1[\s\-*_]*$/.test(line)) {
      flushPara(); closeList();
      out.push('<hr>');
      continue;
    }
    para.push(line);
  }
  flushPara(); closeList();

  return out.join('');
}

function updateUserList(users) {
  var container = document.getElementById('chat-users');
  if (!container) {
    // Create the user list container inside messages div (at the top)
    var messagesDiv = document.getElementById('messages');
    if (!messagesDiv) return;
    container = document.createElement('div');
    container.id = 'chat-users';
    messagesDiv.insertBefore(container, messagesDiv.firstChild);
  }
  if (!users || users.length === 0) {
    container.innerHTML = '';
    return;
  }
  var parts = users.map(function(u) {
    return '<a href="/@' + encodeURIComponent(u) + '" title="View profile" style="color:#555;text-decoration:none;font-weight:600;">@' + u + '</a>';
  });
  container.innerHTML = '<span style="color:#999;">In room: </span>' + parts.join(' &nbsp;');
}

function sendRoomMessage(form) {
  const input = form.querySelector('input[name="prompt"]');
  if (!input) return;
  
  const content = input.value.trim();
  
  if (content && roomWs && roomWs.readyState === WebSocket.OPEN) {
    roomWs.send(JSON.stringify({ content: content }));
    input.value = '';
  }
}

// Which room this page is, read from the page itself.
//
// It used to be a global the server defined in a script at the end of <body>.
// Soft navigation swaps #content and re-runs only the scripts inside it, so
// that script was never carried over and the global was whatever the last full
// page load had left there — undefined on the way in, and stale on the way out.
// Reading it from an element inside the content means there is nothing to go
// stale: no element, no room.
function roomFromPage() {
  const el = document.getElementById('room-data');
  if (!el) return null;
  try {
    const d = JSON.parse(el.textContent);
    return d && d.id ? d : null;
  } catch (e) {
    return null;
  }
}

// Initialize room chat on page load and when switching topics
function initRoomChat() {
  const currentRoomData = roomFromPage();

  // Navigated away from a room. Without this the socket outlives the page that
  // opened it and keeps appending messages to a #messages that is no longer
  // there — or worse, to the next page's.
  if (!currentRoomData) {
    if (roomWs) { roomWs.close(); roomWs = null; }
    currentRoomId = null;
    return;
  }

  {
    // Set the topic to the room title and display context like regular topics
    topic = currentRoomData.title;
    
    // Update hidden input if it exists
    const topicInput = document.getElementById('topic');
    if (topicInput) {
      topicInput.value = currentRoomData.title;
    }
    

    
    // Connect WebSocket first (this will clear messages and load sessionStorage)
    connectRoomWebSocket(currentRoomData.id);
    
    // No context message. The room's summary, its name and the link to what it
    // is about are rendered by the server above #messages — see aboutRoom in
    // service/chat. This built a second copy of all three and inserted it as
    // the first thing inside #messages, so the page said the same paragraph
    // twice a hundred milliseconds after loading, with the room's title
    // repeated between them.
    //
    // It also wrote a summary through innerHTML, and a summary can come from a
    // page somebody else wrote.

    // Override chat form submission for room mode
    const chatForm = document.getElementById('chat-form');
    if (chatForm) {
      chatForm.onsubmit = function(e) {
        e.preventDefault();
        sendRoomMessage(this);
        return false;
      };
      
      // Update placeholder
      const input = chatForm.querySelector('input[name="prompt"]');
      if (input) {
        input.placeholder = 'Type your message...';
      }
    }
  }
}

// Both events, because a room can be arrived at either way and this only ever
// listened for one of them. DOMContentLoaded does not fire on a soft
// navigation, so clicking "Join discussion" swapped the room's markup in and
// then nothing wired it up: no socket, no context line, a form that posted
// nowhere. A hard refresh fixed it because that is a real page load, which is
// exactly the shape of a bug that only exists after soft navigation was added.
document.addEventListener('DOMContentLoaded', initRoomChat);
document.addEventListener('mu:navigated', initRoomChat);

// ============================================
// BLOG POST VALIDATION
// ============================================

// Validate blog post form on /blog page
document.addEventListener('DOMContentLoaded', function() {
  const form = document.getElementById('blog-form');
  if (!form) return;
  
  const textarea = document.getElementById('post-content');
  const charCount = document.getElementById('char-count');
  
  if (!textarea || !charCount) return;
  
  function updateCharCount() {
    const length = textarea.value.length;
    const remaining = 50 - length;
    
    if (length < 50) {
      charCount.textContent = 'Min 50 chars (' + remaining + ' more)';
      charCount.style.color = '#dc3545';
    } else if (length > 10000) {
      charCount.textContent = length + ' chars (max 10,000 exceeded!)';
      charCount.style.color = '#dc3545';
    } else {
      charCount.textContent = length + ' characters';
      charCount.style.color = '#28a745';
    }
  }
  
  textarea.addEventListener('input', updateCharCount);
  
  form.addEventListener('submit', function(e) {
    if (textarea.value.length < 50) {
      e.preventDefault();
      showToast('Post must be at least 50 characters', 'error');
      textarea.focus();
      return false;
    }
    if (textarea.value.length > 10000) {
      e.preventDefault();
      showToast('Post must not exceed 10,000 characters', 'error');
      textarea.focus();
      return false;
    }
  });
  
  updateCharCount();
});

// PRESENCE WEBSOCKET (HOME PAGE)
// ============================================

let presenceWs;
let presenceReconnectTimer;

function connectPresence() {
  if (presenceWs && presenceWs.readyState === WebSocket.OPEN) {
    return;
  }
  
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  presenceWs = new WebSocket(protocol + '//' + window.location.host + '/presence');
  
  presenceWs.onopen = function() {
    console.log('Connected to presence');
    // Send heartbeat every 30s to stay marked as online
    setInterval(() => {
      if (presenceWs && presenceWs.readyState === WebSocket.OPEN) {
        presenceWs.send(JSON.stringify({type: 'ping'}));
      }
    }, 30000);
  };
  
  presenceWs.onmessage = function(event) {
    const msg = JSON.parse(event.data);
    if (msg.type === 'presence') {
      updatePresenceDisplay(msg.users, msg.count);
    }
  };
  
  presenceWs.onclose = function() {
    console.log('Presence disconnected');
    // Reconnect after 5s
    if (presenceReconnectTimer) clearTimeout(presenceReconnectTimer);
    presenceReconnectTimer = setTimeout(connectPresence, 5000);
  };
  
  presenceWs.onerror = function(error) {
    console.error('Presence WebSocket error:', error);
  };
}

function updatePresenceDisplay(users, count) {
  const presenceContent = document.getElementById('presence-content');
  if (!presenceContent) return;
  
  function makeUserLink(u) {
    return '<a href="/@' + u + '" title="View profile" style="color: inherit;">@' + u + '</a>';
  }

  if (count === 0) {
    presenceContent.innerHTML = '<span class="info">No one else is here right now</span>';
  } else if (count === 1) {
    presenceContent.innerHTML = makeUserLink(users[0]) + ' is here';
  } else if (count <= 5) {
    const userLinks = users.map(makeUserLink).join(', ');
    presenceContent.innerHTML = userLinks + ' are here';
  } else {
    // Show first 3 users and count of others
    const firstThree = users.slice(0, 3).map(makeUserLink).join(', ');
    presenceContent.innerHTML = firstThree + ' and ' + (count - 3) + ' others are here';
  }
}

// Connect to presence on home page
if (window.location.pathname === '/home' || window.location.pathname === '/') {
  document.addEventListener('DOMContentLoaded', function() {
    // Small delay to let session check complete first
    setTimeout(connectPresence, 500);
    
    // Apply hidden cards immediately (from localStorage)
    applyHiddenCards();
  });
}

// ============================================
// CARD CUSTOMIZATION
// ============================================

function applyHiddenCards() {
  // Apply hidden cards from localStorage
  const hidden = JSON.parse(localStorage.getItem('mu_hidden_cards') || '[]');
  hidden.forEach(id => {
    const card = document.getElementById(id);
    if (card) card.style.display = 'none';
  });
}

// Available cards that can be shown/hidden
const availableCards = [
  { id: 'news', title: 'News' },
  { id: 'reminder', title: 'Reminder' },
  { id: 'markets', title: 'Markets' },
  { id: 'blog', title: 'Blog' },
  { id: 'video', title: 'Video' }
];

function initCardCustomization() {
  if (document.getElementById('customize-link')) return;
  
  const pageTitle = document.getElementById('page-title');
  if (!pageTitle || pageTitle.textContent !== 'Home') return;
  
  const link = document.createElement('a');
  link.id = 'customize-link';
  link.href = '#';
  link.textContent = 'Customize';
  link.style.cssText = 'font-size: 12px; color: var(--text-muted); position: absolute; right: 0; top: 50%; transform: translateY(-50%);';
  link.onclick = (e) => { e.preventDefault(); showCardModal(); };
  
  // Wrap title in relative container for absolute positioning
  const wrapper = document.createElement('div');
  wrapper.style.cssText = 'position: relative;';
  pageTitle.parentNode.insertBefore(wrapper, pageTitle);
  wrapper.appendChild(pageTitle);
  wrapper.appendChild(link);
}

function showCardModal() {
  const hidden = JSON.parse(localStorage.getItem('mu_hidden_cards') || '[]');
  
  // Build checkbox list from available cards
  let checkboxes = '';
  availableCards.forEach(card => {
    const checked = !hidden.includes(card.id) ? 'checked' : '';
    checkboxes += `<label style="display: block; margin: 12px 0; cursor: pointer;"><input type="checkbox" ${checked} data-card-id="${card.id}" style="width: auto; margin-right: 8px;"> ${card.title}</label>`;
  });
  
  // Create modal
  const modal = document.createElement('div');
  modal.id = 'card-customize-modal';
  modal.innerHTML = `
    <div style="position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); z-index: 1000; display: flex; align-items: center; justify-content: center;">
      <div style="background: white; padding: 20px; border-radius: 8px; max-width: 400px; width: 90%; max-height: 80vh; overflow-y: auto;">
        <h3 style="margin-top: 0;">Customize Home Cards</h3>
        <p style="color: var(--text-muted); font-size: 14px;">Choose which cards to show:</p>
        <div id="card-checkboxes">${checkboxes}</div>
        <div style="margin-top: 20px; display: flex; gap: 10px;">
          <button onclick="saveCardPrefs()" style="flex: 1;">Save</button>
          <button onclick="closeCardModal()" style="flex: 1; background: #666;">Cancel</button>
        </div>
      </div>
    </div>
  `;
  document.body.appendChild(modal);
}

function closeCardModal() {
  const modal = document.getElementById('card-customize-modal');
  if (modal) modal.remove();
}

function saveCardPrefs() {
  const checkboxes = document.querySelectorAll('#card-checkboxes input[type="checkbox"]');
  const hidden = [];
  
  checkboxes.forEach(cb => {
    const cardId = cb.dataset.cardId;
    const card = document.getElementById(cardId);
    if (!cb.checked) {
      hidden.push(cardId);
      if (card) card.style.display = 'none';
    } else {
      if (card) card.style.display = '';
    }
  });
  
  localStorage.setItem('mu_hidden_cards', JSON.stringify(hidden));
  closeCardModal();
}

// ============================================
// TOAST NOTIFICATIONS
// ============================================

function showToast(message, type = 'info', duration = 4000) {
  // Remove existing toast
  const existing = document.getElementById('mu-toast');
  if (existing) existing.remove();
  
  const toast = document.createElement('div');
  toast.id = 'mu-toast';
  toast.className = 'mu-toast mu-toast-' + type;
  toast.textContent = message;
  
  // Add close button
  const close = document.createElement('span');
  close.textContent = '×';
  close.className = 'mu-toast-close';
  close.onclick = () => toast.remove();
  toast.appendChild(close);
  
  document.body.appendChild(toast);
  
  // Auto dismiss
  if (duration > 0) {
    setTimeout(() => {
      if (toast.parentNode) {
        toast.classList.add('mu-toast-hide');
        setTimeout(() => toast.remove(), 300);
      }
    }, duration);
  }
}

// ============================================
// API HELPER
// ============================================

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

} // End of window context check
