// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v30';
var CACHE_NAME = APP_PREFIX + VERSION;

// Static assets to cache
var STATIC_CACHE = [
  '/mu.css',
  '/mu.js',
  '/mu.png',
  '/home.png',
  '/chat.png',
  '/blog.png',
  '/news.png',
  '/video.png',
  '/mail.png',
  '/icon-192.png',
  '/icon-512.png',
  '/manifest.webmanifest'
];

// ============================================
// SERVICE WORKER EVENT LISTENERS
// ============================================

self.addEventListener('fetch', function (e) {
  // Let browser handle all fetches naturally - only cache static assets
  const url = new URL(e.request.url);
  
  if (e.request.method !== 'GET') {
    return;
  }
  
  // Only intercept static assets
  if (url.pathname.match(/\.(css|js|png|jpg|jpeg|gif|svg|ico|webmanifest)$/)) {
    e.respondWith(
      caches.match(e.request).then(cached => cached || fetch(e.request))
    );
  }
});

self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(cache => cache.addAll(STATIC_CACHE))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', function (e) {
  e.waitUntil(
    caches.keys().then(keys => 
      Promise.all(
        keys.filter(key => key.startsWith(APP_PREFIX) && key !== CACHE_NAME)
          .map(key => caches.delete(key))
      )
    ).then(() => self.clients.claim())
  );
});

// ============================================
// CHAT FUNCTIONALITY
// ============================================

let context = [];

function loadMessages() {
  console.log("loading messages");

  var d = document.getElementById("messages");

  context.forEach(function(data) {
    console.log(data);
    d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
    d.innerHTML += `<div class="message"><span class="llm">llm</span>${data["answer"]}</div>`;
  });

  d.scrollTop = d.scrollHeight;
}

function askLLM(el) {
  var d = document.getElementById('messages');

  const formData = new FormData(el);
  const data = {};

  // Iterate over formData and populate the data object
  for (let [key, value] of formData.entries()) {
    data[key] = value;
  }

  var p = document.getElementById("prompt");

  if (p.value == "") {
    return false;
  }

  // reset prompt
  p.value = '';

  console.log("sending", data);
  d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
  d.scrollTop = d.scrollHeight;

  var prompt = data["prompt"];

  data["context"] = context;

  fetch("/chat", {
    method: "POST",
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  }).then(response => response.json())
  .then(result => {
    console.log('Success:', result);
    d.innerHTML += `<div class="message"><span class="llm">llm</span>${result.answer}</div>`;
    d.scrollTop = d.scrollHeight;

    // save the context
    context.push({answer: result.answer, prompt: prompt});
    setContext();
  })
  .catch(error => {
    console.error('Error:', error);
  });

  return false;
}

function loadContext() {
  var ctx = sessionStorage.getItem("context");
  if (ctx == null || ctx == undefined || ctx == "") {
    return;
  }
  context = JSON.parse(ctx);
}

function setContext() {
  sessionStorage.setItem("context", JSON.stringify(context));
}

function loadChat() {
  loadContext();
  loadMessages();

  // scroll to bottom of prompt
  const prompt = document.getElementById('prompt');
  const messages = document.getElementById('messages');
  const container = document.getElementById('container');

  // Only adjust for mobile keyboards when viewport is small
  if (window.visualViewport && window.innerWidth <= 600) {
    // Prevent scrolling when input gains focus
    prompt.addEventListener('focus', () => {
      container.style.overflow = 'hidden';
      window.scrollTo(0, 0);
    });

    window.visualViewport.addEventListener('resize', () => {
      const viewportHeight = window.visualViewport.height;
      const documentHeight = document.documentElement.clientHeight;

      // Keyboard opened
      if (viewportHeight < documentHeight) {
        messages.style.height = (viewportHeight - 280) + 'px';
        container.style.overflow = 'hidden';
      } else {
        // Keyboard closed - reset to CSS default
        messages.style.height = '';
        // Ensure no scroll on container
        container.scrollTop = 0;
        window.scrollTo(0, 0);
      }

      messages.scrollTop = messages.scrollHeight;
    });
  }
}

// ============================================
// VIDEO FUNCTIONALITY
// ============================================

function getVideos(el) {
  const formData = new FormData(el);
  const data = {};

  // Iterate over formData and populate the data object
  for (let [key, value] of formData.entries()) {
    data[key] = value;
  }

  console.log("sending", data);

  fetch("/video", {
    method: "POST",
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  }).then(response => response.json())
  .then(result => {
    console.log('Success:', result);
    var d = document.getElementById('results');

    if (d == null) {
      d = document.createElement("div");
      d.setAttribute("id", "results");

      var content = document.getElementById('content');
      content.innerHTML += "<h1>Results</h1>";
      content.appendChild(d);
    } else {
      d.innerHTML = '';
    }

    d.innerHTML += result.html;
    document.getElementById('query').value = data["query"];
  })
  .catch(error => {
    console.error('Error:', error);
  });

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
  }).then(response => response.json())
  .then(sess => {
    console.log('Success:', sess);
    var acc = document.getElementById("account");
    if (sess.type == "account") {
      acc.innerHTML = "<a href='/logout'>Logout</a>";
    } else {
      acc.innerHTML = "<a href='/login'>Login</a>";
      // If we're on a protected page but not logged in, redirect
      const protectedPaths = ['/home', '/chat', '/blog', '/news', '/video'];
      if (protectedPaths.includes(window.location.pathname)) {
        window.location.href = '/';
      }
    }
  })
  .catch(error => {
    console.error('Error:', error);
    // On error, redirect to home
    window.location.href = '/';
  });
}

// ============================================
// EVENT LISTENERS
// ============================================

self.addEventListener("hashchange", function(event) {
  // Don't reload on hash change - anchors should just scroll
  if (window.location.hash) {
    console.log('Hash changed to:', window.location.hash);
  }
});

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
  
  // set nav
  var nav = document.getElementById("nav");
  for (const el of nav.children) {
    if (el.getAttribute("href") == window.location.pathname) {
      el.setAttribute("class", "active");
      continue;
    }
    el.removeAttribute("class");
  }

  // load chat
  if (window.location.pathname == "/chat") {
    loadChat();
  }

  // Check session status on page load
  setSession();
});
