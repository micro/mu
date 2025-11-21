// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v43';
var CACHE_NAME = APP_PREFIX + VERSION;

// Minimal caching - only icons
var STATIC_CACHE = [
  '/mu.png',
  '/home.png',
  '/chat.png',
  '/post.png',
  '/news.png',
  '/video.png',
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

self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(cache => cache.addAll(STATIC_CACHE))
      .then(() => self.skipWaiting())
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
  
  // Create placeholder for LLM response
  const responseDiv = document.createElement('div');
  responseDiv.className = 'message';
  responseDiv.innerHTML = `<span class="llm">llm</span><div class="llm-response"></div>`;
  d.appendChild(responseDiv);
  const responseContent = responseDiv.querySelector('.llm-response');
  
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
    
    // Stage the response character by character to preserve HTML formatting
    const fullResponse = result.answer;
    let currentIndex = 0;
    
    function addNextChunk() {
      if (currentIndex < fullResponse.length) {
        const chunkSize = 5; // Add 5 characters at a time
        const chunk = fullResponse.slice(currentIndex, currentIndex + chunkSize);
        responseContent.innerHTML = fullResponse.slice(0, currentIndex + chunkSize);
        currentIndex += chunkSize;
        d.scrollTop = d.scrollHeight;
        setTimeout(addNextChunk, 20); // 20ms delay
      } else {
        // Save context after full response is displayed
        context.push({answer: result.answer, prompt: prompt});
        setContext();
      }
    }
    
    addNextChunk();
  })
  .catch(error => {
    console.error('Error:', error);
    responseContent.innerHTML = 'Error: Failed to get response';
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
  const content = document.getElementById('content');

  // Only adjust for mobile keyboards when viewport is small
  if (window.visualViewport && window.innerWidth <= 600) {
    // Prevent scrolling when input gains focus
    prompt.addEventListener('focus', () => {
      container.style.overflow = 'hidden';
      window.scrollTo(0, 0);
    });

    window.visualViewport.addEventListener('resize', () => {
      const viewportHeight = window.visualViewport.height;
      
      // Adjust content height based on actual visible viewport
      if (content) {
        content.style.height = (viewportHeight - 51) + 'px';
      }
      
      // Keep messages scrolled to bottom
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
    var logoutLink = document.getElementById("logout-link");
    if (sess.type == "account") {
      if (logoutLink) logoutLink.style.display = '';
    } else {
      if (logoutLink) logoutLink.style.display = 'none';
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
