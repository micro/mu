// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v44';
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

// Constants
const CHAT_TOPIC_SELECTOR = '#topic-selector .head';
const TOPICS_SELECTOR = '#topics .head';
const CHAT_PATH = '/chat';

var context = [];
var topic = '';

function switchTopic(t) {
  topic = t;
  
  // Update hidden input (only exists on chat page)
  const topicInput = document.getElementById('topic');
  if (topicInput) {
    topicInput.value = t;
  }
  
  // Update active tab - match by text content
  document.querySelectorAll('#topic-selector .head').forEach(tab => {
    if (tab.textContent === t) {
      tab.classList.add('active');
    } else {
      tab.classList.remove('active');
    }
  });
  
  // Load context for this topic
  loadContext();
  
  // Clear and reload messages
  const messages = document.getElementById('messages');
  messages.innerHTML = '';
  
  // Show topic summary at top
  const summaryDiv = document.getElementById('topic-summary');
  console.log('Switching to topic:', t);
  console.log('Room data:', roomsData ? roomsData[t] : 'No roomsData');
  
  if (roomsData && roomsData[t]) {
    const room = roomsData[t];
    const summary = room.Summary || room.summary;
    console.log('Summary value:', summary);
    console.log('Summary type:', typeof summary);
    console.log('Summary length:', summary ? summary.length : 0);
    
    if (summary && summary.trim().length > 0) {
      console.log('Found summary for', t);
      summaryDiv.innerHTML = `<div class="topic-brief"><strong>${t}</strong><div>${summary}</div></div>`;
      summaryDiv.style.display = 'block';
    } else {
      console.log('No summary for', t);
      summaryDiv.innerHTML = `<div class="topic-brief"><strong>${t}</strong><div style="color: #999; font-style: italic;">Loading summary...</div></div>`;
      summaryDiv.style.display = 'block';
    }
  } else {
    summaryDiv.innerHTML = '';
    summaryDiv.style.display = 'none';
  }
  
  // Load conversation history for this topic
  loadMessages();
  
  // Scroll to bottom
  messages.scrollTop = messages.scrollHeight;
}

function loadContext() {
  const key = `context-${topic}`;
  const ctx = sessionStorage.getItem(key);
  if (ctx == null || ctx == undefined || ctx == "") {
    context = [];
    return;
  }
  context = JSON.parse(ctx);
}

function setContext() {
  const key = `context-${topic}`;
  sessionStorage.setItem(key, JSON.stringify(context));
}

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
  
  // Add current topic for enhanced RAG
  data["topic"] = topic;

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

function loadChat() {
  // Get topics from the page
  const topicLinks = document.querySelectorAll(CHAT_TOPIC_SELECTOR);
  
  // Guard against empty topic list
  if (topicLinks.length === 0) {
    console.warn('No topics available in chat');
    return;
  }
  
  // Check if there's a hash in the URL and if it exists
  let topicLoaded = false;
  if (window.location.hash) {
    const hash = window.location.hash.substring(1);
    topicLoaded = switchToTopicIfExists(hash);
  }
  
  // Fallback to first topic if no valid hash was found
  if (!topicLoaded) {
    switchTopic(topicLinks[0].textContent);
  }

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

function highlightTopic(topicName) {
  // Specific selectors for topic elements
  const selectors = [CHAT_TOPIC_SELECTOR, TOPICS_SELECTOR];
  
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

function switchToTopicIfExists(hash) {
  // Check if the topic exists in the selector
  const topicLinks = document.querySelectorAll(CHAT_TOPIC_SELECTOR);
  for (const link of topicLinks) {
    if (link.textContent === hash) {
      switchTopic(hash);
      return true;
    }
  }
  return false;
}

function handleHashChange() {
  if (!window.location.hash) return;
  
  const hash = window.location.hash.substring(1);
  console.log('Hash changed to:', hash);
  
  // Highlight the matching topic/tag
  highlightTopic(hash);
  
  // For chat page, switch to the topic if it exists
  if (window.location.pathname === CHAT_PATH) {
    switchToTopicIfExists(hash);
  }
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
  
  // set nav
  var nav = document.getElementById("nav");
  for (const el of nav.children) {
    if (el.getAttribute("href") == window.location.pathname) {
      el.setAttribute("class", "active");
      continue;
    }
    el.removeAttribute("class");
  }

  // Mobile logout handler - clicking account icon on mobile logs out
  if (window.innerWidth <= 600) {
    const account = document.getElementById('account');
    if (account) {
      account.style.cursor = 'pointer';
      account.addEventListener('click', function() {
        window.location.href = '/logout';
      });
    }
  }

  // load chat
  if (window.location.pathname == CHAT_PATH) {
    loadChat();
    
    // Add click handlers for chat topics
    document.querySelectorAll(CHAT_TOPIC_SELECTOR).forEach(link => {
      link.addEventListener('click', function(e) {
        e.preventDefault();
        const topicName = this.textContent;
        switchTopic(topicName);
        // Update URL hash with pushState for proper browser history
        history.pushState(null, null, '#' + topicName);
      });
    });
  }
  
  // Handle hash on page load for topic highlighting (non-chat pages)
  if (window.location.hash && window.location.pathname !== CHAT_PATH) {
    handleHashChange();
  }

  // Check session status on page load
  setSession();
});
