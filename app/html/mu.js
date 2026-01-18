// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v94';
var CACHE_NAME = APP_PREFIX + VERSION;

// Minimal caching - only icons
var STATIC_CACHE = [
  '/mu.png',
  '/home.png',
  '/chat.png',
  '/post.png',
  '/news.png',
  '/video.png',
  '/account.png',
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
// PAGE JAVASCRIPT (only run in window context)
// ============================================

// Exit early if we're in service worker context
if (typeof document === 'undefined') {
  // We're in service worker context, don't execute page code
  // Service worker code above will still run
} else {
  // We're in window context, execute page code

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

function openMicroDialog() {
  const dialog = document.getElementById('micro-dialog');
  if (dialog) {
    dialog.style.display = 'flex';
    const input = document.getElementById('micro-input');
    if (input) {
      setTimeout(() => input.focus(), 100);
    }
    // Prevent body scroll on mobile
    document.body.style.overflow = 'hidden';
  }
}

function closeMicroDialog() {
  const dialog = document.getElementById('micro-dialog');
  if (dialog) {
    dialog.style.display = 'none';
    // Don't reset overflow on chat page - it should stay hidden
    if (!document.getElementById('messages')) {
      document.body.style.overflow = '';
    }
  }
}

function sendMicroMessage() {
  const input = document.getElementById('micro-input');
  const messages = document.getElementById('micro-messages');
  
  if (!input || !messages || !input.value.trim()) return;
  
  const query = input.value.trim();
  input.value = '';
  
  // Add user message
  const userMsg = document.createElement('div');
  userMsg.className = 'micro-msg user';
  userMsg.innerHTML = '<span>' + escapeHtml(query) + '</span>';
  messages.appendChild(userMsg);
  
  // Add loading message
  const assistantMsg = document.createElement('div');
  assistantMsg.className = 'micro-msg assistant';
  assistantMsg.innerHTML = '<span class="loading">Working...</span>';
  messages.appendChild(assistantMsg);
  
  messages.scrollTop = messages.scrollHeight;
  
  // Send to agent
  fetch('/agent/run', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ task: query })
  })
  .then(r => {
    if (r.status === 401) {
      throw new Error('login');
    }
    return r.json();
  })
  .then(result => {
    if (result.success && result.answer) {
      assistantMsg.innerHTML = '<span>' + formatMicroResponse(result.answer) + '</span>';
      
      // Handle navigation
      if (result.action === 'navigate' && result.url) {
        setTimeout(() => {
          window.location.href = result.url;
        }, 1000);
      }
    } else {
      assistantMsg.innerHTML = '<span>' + (result.answer || 'Sorry, I couldn\'t help with that.') + '</span>';
    }
    messages.scrollTop = messages.scrollHeight;
  })
  .catch(err => {
    if (err.message === 'login') {
      assistantMsg.innerHTML = '<span>Please <a href="/login">login</a> to use @micro</span>';
    } else {
      assistantMsg.innerHTML = '<span>Something went wrong. Try again.</span>';
    }
    messages.scrollTop = messages.scrollHeight;
  });
}

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
const CHAT_TOPIC_SELECTOR = '#topic-selector .head';
const TOPICS_SELECTOR = '#topics .head';
const CHAT_PATH = '/chat';

var isAuthenticated = false;
var context = [];
var topic = '';

// Show all topic summaries with join links (landing page)
function showAllTopicSummaries() {
  const messages = document.getElementById('messages');
  if (!messages || typeof summaries === 'undefined') return;
  
  messages.innerHTML = '';
  const topics = Object.keys(summaries).sort();
  
  topics.forEach(t => {
    if (summaries[t]) {
      const summaryMsg = document.createElement('div');
      summaryMsg.className = 'message';
      const topicBadge = `<span class="category">${t}</span>`;
      const joinLink = `<a href="/chat?id=chat_${encodeURIComponent(t)}" class="link" style="display: inline; margin-top: 8px;">Join discussion →</a>`;
      summaryMsg.innerHTML = `${topicBadge}<p>${summaries[t]}</p>${joinLink}`;
      messages.appendChild(summaryMsg);
    }
  });
}

// Show topic context without connecting to WebSocket
function showTopicContext(t) {
  topic = t;
  
  // Update hidden input
  const topicInput = document.getElementById('topic');
  if (topicInput) {
    topicInput.value = t;
  }
  
  // Update active tab
  document.querySelectorAll('#topic-selector .head').forEach(tab => {
    if (tab.textContent === t) {
      tab.classList.add('active');
    } else {
      tab.classList.remove('active');
    }
  });
  
  // Show context message
  const messages = document.getElementById('messages');
  if (messages) {
    messages.innerHTML = '';
    const contextMsg = document.createElement('div');
    contextMsg.className = 'context-message';
    let summary = '';
    if (typeof summaries !== 'undefined' && summaries[t]) {
      summary = '<br><span style="color: #666;">' + summaries[t] + '</span>';
    }
    contextMsg.innerHTML = '<strong>' + t + ' Discussion</strong>' + summary;
    messages.appendChild(contextMsg);
  }
  
  // Update URL
  const roomId = 'chat_' + t;
  history.replaceState(null, null, '/chat?id=' + roomId);
}

function switchTopic(t) {
  // Show context first
  showTopicContext(t);
  
  // Connect to WebSocket if authenticated
  const roomId = 'chat_' + t;
  if (isAuthenticated) {
    connectRoomWebSocket(roomId);
  }
  
  // Override form to use room messaging
  const chatForm = document.getElementById('chat-form');
  if (chatForm) {
    chatForm.onsubmit = function(e) {
      e.preventDefault();
      if (!isAuthenticated) {
        alert('Please login to chat');
        return false;
      }
      sendRoomMessage(this);
      return false;
    };
  }
}

function loadContext() {
  const ctx = sessionStorage.getItem('context');
  if (ctx == null || ctx == undefined || ctx == "") {
    context = [];
    return;
  }
  context = JSON.parse(ctx);
}

function setContext() {
  sessionStorage.setItem('context', JSON.stringify(context));
}

function loadMessages() {
  console.log("loading messages");

  var d = document.getElementById("messages");

  context.forEach(function(data) {
    console.log(data);
    d.innerHTML += `<div class="message"><span class="you">you</span><p>${data["prompt"]}</p></div>`;
    d.innerHTML += `<div class="message"><span class="llm">AI</span>${data["answer"]}</div>`;
  });

  d.scrollTop = d.scrollHeight;
}

function askLLM(el) {
  // Check authentication first
  if (!isAuthenticated) {
    alert('Please login to chat');
    return false;
  }

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
  
  // Create placeholder for AI response with loading dots
  const responseDiv = document.createElement('div');
  responseDiv.className = 'message';
  responseDiv.innerHTML = `<span class="micro">micro</span><div class="ai-response"><span class="loading-dots">...</span></div>`;
  d.appendChild(responseDiv);
  const responseContent = responseDiv.querySelector('.ai-response');
  
  d.scrollTop = d.scrollHeight;

  var prompt = data["prompt"];

  data["context"] = context;

  fetch("/chat", {
    method: "POST",
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  }).then(response => {
    if (response.status === 401) {
      throw new Error('Authentication required');
    }
    return response.json();
  })
  .then(result => {
    console.log('Success:', result);
    
    // Display the full response immediately
    responseContent.innerHTML = result.answer;
    d.scrollTop = d.scrollHeight;
    
    // Save context after response is displayed
    context.push({answer: result.answer, prompt: prompt});
    setContext();
  })
  .catch(error => {
    console.error('Error:', error);
    if (error.message === 'Authentication required') {
      const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
      responseContent.innerHTML = 'Please <a href="/login?redirect=' + redirectUrl + '">login</a> to chat';
    } else {
      responseContent.innerHTML = 'Error: Failed to get response';
    }
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
  
  // Check if we're already in a specific room (from URL)
  const urlParams = new URLSearchParams(window.location.search);
  const roomId = urlParams.get('id');
  const autoPrompt = urlParams.get('prompt');
  
  // If we have a chat room ID from URL, join that room
  if (roomId && roomId.startsWith('chat_')) {
    const topicName = roomId.replace('chat_', '');
    // Highlight the active tab
    document.querySelectorAll('#topic-selector .head').forEach(tab => {
      if (tab.textContent === topicName) {
        tab.classList.add('active');
      } else {
        tab.classList.remove('active');
      }
    });
    // Show context and connect if authenticated
    showTopicContext(topicName);
    // Connect after auth check completes
    setTimeout(() => {
      if (isAuthenticated) {
        connectRoomWebSocket(roomId);
        // Override form for room messaging
        const chatForm = document.getElementById('chat-form');
        if (chatForm) {
          chatForm.onsubmit = function(e) {
            e.preventDefault();
            sendRoomMessage(this);
            return false;
          };
        }
      }
    }, 500);
  } else if (!roomId && !autoPrompt) {
    // No room specified - show all topic summaries with join links
    showAllTopicSummaries();
  }
  
  // Auto-submit prompt if provided (legacy support)
  if (autoPrompt) {
    loadContext();
    loadMessages();
    const promptInput = document.getElementById('prompt');
    const form = document.getElementById('chat-form');
    if (promptInput && form) {
      promptInput.value = autoPrompt;
      const newUrl = window.location.pathname + window.location.hash;
      window.history.replaceState({}, document.title, newUrl);
      setTimeout(function() { askLLM(form); }, 100);
    }
  }

  // scroll to bottom of prompt
  const prompt = document.getElementById('prompt');
  const messages = document.getElementById('messages');
  const container = document.getElementById('container');
  const content = document.getElementById('content');


  
  // Update chat form state based on authentication
  updateChatFormState();
}

// ============================================
// VIDEO FUNCTIONALITY
// ============================================

function getVideos(el) {
  // Check authentication first
  if (!isAuthenticated) {
    alert('Please login to search');
    return false;
  }

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
  }).then(response => {
    if (response.status === 401) {
      throw new Error('Authentication required');
    }
    return response.json();
  })
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
    if (error.message === 'Authentication required') {
      alert('Please login to search videos');
    } else {
      alert('Error: Failed to search videos');
    }
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
    var accountHeader = document.getElementById("account-header");
    var loginHeader = document.getElementById("login-header");
    var mailHeader = document.getElementById("mail-header");
    var mailBadge = document.getElementById("mail-badge");
    var microFab = document.getElementById("micro-fab");
    
    if (sess.type == "account") {
      isAuthenticated = true;
      if (accountHeader) accountHeader.style.display = 'inline-block';
      if (mailHeader) mailHeader.style.display = 'inline-block';
      if (microFab) microFab.style.display = 'flex';
      if (loginHeader) loginHeader.style.display = 'none';
      // Fetch unread mail count for badge
      fetch('/mail?unread=count')
        .then(res => res.json())
        .then(data => {
          if (data.count > 0 && mailBadge) {
            mailBadge.textContent = data.count > 9 ? '9+' : data.count;
            mailBadge.style.display = 'inline';
          }
        })
        .catch(() => {});
      // Initialize voice assistant for authenticated users
      tryInitVoiceAssistant();
      // Initialize card customization for home page
      if (window.location.pathname === '/home') {
        initCardCustomization();
      }
    } else {
      isAuthenticated = false;
      if (accountHeader) accountHeader.style.display = 'none';
      if (mailHeader) mailHeader.style.display = 'none';
      if (microFab) microFab.style.display = 'none';
      if (loginHeader) {
        loginHeader.style.display = 'inline-block';
        // Update login link to include redirect parameter
        if (window.location.pathname !== '/login' && window.location.pathname !== '/signup' && window.location.pathname !== '/') {
          const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
          loginHeader.href = '/login?redirect=' + redirectUrl;
          console.log('Updated login header to:', loginHeader.href);
        }
      }
    }
    updateChatFormState();
    updateSearchFormsState();
  })
  .catch(error => {
    console.error('Error:', error);
    isAuthenticated = false;
    var accountHeader = document.getElementById("account-header");
    var mailHeader = document.getElementById("mail-header");
    var loginHeader = document.getElementById("login-header");
    var microFab = document.getElementById("micro-fab");
    if (accountHeader) accountHeader.style.display = 'none';
    if (mailHeader) mailHeader.style.display = 'none';
    if (microFab) microFab.style.display = 'none';
    if (loginHeader) {
      loginHeader.style.display = 'block';
      // Update login link to include redirect parameter for unauthenticated users
      if (window.location.pathname !== '/login' && window.location.pathname !== '/signup' && window.location.pathname !== '/') {
        const redirectUrl = encodeURIComponent(window.location.pathname + window.location.search);
        loginHeader.href = '/login?redirect=' + redirectUrl;
        console.log('Updated login header to:', loginHeader.href);
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
      chatPrompt.placeholder = 'Ask a question';
      chatPrompt.disabled = false;
      chatButton.disabled = false;
      chatPrompt.style.cursor = '';
      chatButton.style.cursor = '';
      chatPrompt.onclick = null;
      chatButton.onclick = null;
    } else {
      chatPrompt.placeholder = 'Login to chat';
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

  // load chat
  if (window.location.pathname == CHAT_PATH) {
    loadChat();
    
    // Add click handlers for chat topics - always switch rooms
    document.querySelectorAll(CHAT_TOPIC_SELECTOR).forEach(link => {
      link.addEventListener('click', function(e) {
        e.preventDefault();
        const topicName = this.textContent;
        switchTopic(topicName);
      });
    });
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
        alert('Please login to search news');
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
        alert('Please login to search videos');
        return false;
      }
    });
  }

  // Check session status on page load
  setSession();
});

// Flag a post
function flagPost(postId) {
  if (!confirm('Flag this post as inappropriate? It will be hidden after 3 flags.')) {
    return;
  }
  
  fetch('/flag', {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: 'type=post&id=' + encodeURIComponent(postId)
  })
  .then(response => response.json())
  .then(data => {
    if (data.success) {
      alert('Post flagged. Flag count: ' + data.count);
      if (data.count >= 3) {
        location.reload();
      }
    } else {
      alert(data.message || 'Failed to flag post');
    }
  })
  .catch(error => {
    console.error('Error flagging post:', error);
    alert('Failed to flag post');
  });
}




// ============================================
// CHAT ROOM WEBSOCKET
// ============================================

let roomWs;
let currentRoomId = null;

function getSessionStorageKey(roomId) {
  return 'chat_room_' + roomId;
}

function saveMessageToSession(roomId, msg) {
  const key = getSessionStorageKey(roomId);
  const messages = JSON.parse(sessionStorage.getItem(key) || '[]');
  messages.push(msg);
  // Keep last 50 messages in session
  if (messages.length > 50) {
    messages.shift();
  }
  sessionStorage.setItem(key, JSON.stringify(messages));
}

function loadMessagesFromSession(roomId) {
  const key = getSessionStorageKey(roomId);
  return JSON.parse(sessionStorage.getItem(key) || '[]');
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
  
  // Only clear messages on initial connect, not on reconnect
  if (!isReconnect) {
    const messagesDiv = document.getElementById('messages');
    if (messagesDiv) {
      // Keep only the context message if it exists
      const contextMsg = messagesDiv.querySelector('.context-message');
      messagesDiv.innerHTML = '';
      if (contextMsg) {
        messagesDiv.appendChild(contextMsg);
      }
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
      saveMessageToSession(roomId, msg);
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
    '<span class="llm">micro</span>' : 
    '<span class="you"><a href="/@' + msg.username + '">' + msg.username + '</a></span>';
  
  let content;
  if (msg.is_llm) {
    // Render markdown for AI messages
    content = renderMarkdown(msg.content);
  } else {
    // Escape HTML and linkify URLs for user messages
    content = msg.content.replace(/</g, '&lt;').replace(/>/g, '&gt;');
    content = linkifyText(content);
    content = content.replace(/\n/g, '<br>');
  }
  
  msgDiv.innerHTML = userSpan + '<p>' + content + '</p>';
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

// Simple markdown renderer for common patterns
function renderMarkdown(text) {
  return text
    // Bold
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    // Italic
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    // Code blocks
    .replace(/```(.+?)```/gs, '<pre><code>$1</code></pre>')
    // Inline code
    .replace(/`(.+?)`/g, '<code>$1</code>')
    // Links
    .replace(/\[(.+?)\]\((.+?)\)/g, '<a href="$2" target="_blank">$1</a>')
    // Line breaks
    .replace(/\n/g, '<br>');
}

function updateUserList(users) {
  // Disabled - was causing layout shifts on mobile
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

// Initialize room chat on page load and when switching topics
document.addEventListener('DOMContentLoaded', function() {
  // Check if we're in a room (from roomData injected by server)
  // Use a local variable to avoid pollution across page loads
  const currentRoomData = (typeof roomData !== 'undefined' && roomData && roomData.id) ? roomData : null;
  
  if (currentRoomData) {
    // Set the topic to the room title and display context like regular topics
    topic = currentRoomData.title;
    
    // Update hidden input if it exists
    const topicInput = document.getElementById('topic');
    if (topicInput) {
      topicInput.value = currentRoomData.title;
    }
    

    
    // Connect WebSocket first (this will clear messages and load sessionStorage)
    connectRoomWebSocket(currentRoomData.id);
    
    // Then add context message to messages area with room summary
    // Do this after a brief delay to ensure WebSocket connection is established
    setTimeout(() => {
      const messages = document.getElementById('messages');
      if (messages) {
        // Check if context message already exists
        if (!messages.querySelector('.context-message')) {
          const contextMsg = document.createElement('div');
          contextMsg.className = 'context-message';
          
          // Extract first sentence from summary (up to first period or 200 chars)
          let shortSummary = currentRoomData.summary;
          const firstPeriod = shortSummary.indexOf('. ');
          if (firstPeriod !== -1) {
            shortSummary = shortSummary.substring(0, firstPeriod + 1);
          } else if (shortSummary.length > 200) {
            shortSummary = shortSummary.substring(0, 200) + '...';
          }
          
          contextMsg.innerHTML = 'Discussion: <strong>' + currentRoomData.title + '</strong><br>' + 
            '<span style="color: #666;">' + shortSummary + '</span>' +
            (currentRoomData.url ? '<br><a href="' + currentRoomData.url + '" target="_blank" style="color: #0066cc; font-size: 13px;">→ View Original</a>' : '');
          // Insert at the top
          messages.insertBefore(contextMsg, messages.firstChild);
        }
      }
    }, 100);
    
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
});

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
      alert('Post must be at least 50 characters long');
      textarea.focus();
      return false;
    }
    if (textarea.value.length > 10000) {
      e.preventDefault();
      alert('Post must not exceed 10,000 characters');
      textarea.focus();
      return false;
    }
  });
  
  updateCharCount();
});

// ============================================
// VOICE ASSISTANT ("Hey Micro")
// ============================================

let voiceRecognition = null;
let voiceListening = false;
let voiceWakeDetected = false;
let voiceCommandBuffer = '';
let voiceTimeout = null;
let voiceIndicator = null;

function initVoiceAssistant() {
  // Check browser support
  const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
  if (!SpeechRecognition) {
    console.log('Voice assistant: Speech recognition not supported');
    return;
  }

  // Only init for authenticated users
  if (!isAuthenticated) {
    console.log('Voice assistant: Not authenticated');
    return;
  }

  voiceRecognition = new SpeechRecognition();
  voiceRecognition.continuous = true;
  voiceRecognition.interimResults = true;
  voiceRecognition.lang = 'en-US';

  // Use the header voice button in brand area
  voiceIndicator = document.getElementById('voice-header');
  const voiceIcon = document.getElementById('voice-icon');
  
  if (!voiceIndicator || !voiceIcon) {
    console.log('Voice assistant: Voice header element not found');
    return;
  }
  
  // Clicking the mic icon activates voice, clicking @micro text goes to /agent
  voiceIcon.style.cursor = 'pointer';
  voiceIcon.onclick = (e) => {
    e.preventDefault();
    e.stopPropagation();
    if (voiceWakeDetected) {
      // Already listening, ignore
      return;
    }
    // Tap activates command mode directly
    startVoiceListening();
    setTimeout(activateVoiceCommand, 100); // Small delay to let recognition start
  };

  voiceRecognition.onresult = (event) => {
    let transcript = '';
    let isFinal = false;

    for (let i = event.resultIndex; i < event.results.length; i++) {
      transcript += event.results[i][0].transcript;
      if (event.results[i].isFinal) {
        isFinal = true;
      }
    }

    transcript = transcript.trim();
    console.log('Voice heard:', transcript, 'final:', isFinal);

    if (!voiceWakeDetected) {
      // Not activated yet, ignore
      return;
    }
    
    // Collecting command
    if (transcript) {
      voiceCommandBuffer = transcript;
      const voiceIcon = document.getElementById('voice-icon');
      if (voiceIcon) voiceIcon.textContent = '...';
      if (voiceIndicator) voiceIndicator.title = 'Hearing: ' + transcript;
      
      // Reset timeout for end of speech
      if (voiceTimeout) clearTimeout(voiceTimeout);
      voiceTimeout = setTimeout(() => {
        if (voiceCommandBuffer) {
          executeVoiceCommand(voiceCommandBuffer);
        }
      }, 1500); // Wait 1.5s of silence before executing
    }

    if (isFinal && voiceCommandBuffer) {
      // Execute immediately on final result
      if (voiceTimeout) clearTimeout(voiceTimeout);
      executeVoiceCommand(voiceCommandBuffer);
    }
  };

  voiceRecognition.onerror = (event) => {
    console.log('Voice error:', event.error);
    const voiceIcon = document.getElementById('voice-icon');
    if (event.error === 'not-allowed') {
      if (voiceIcon) {
        voiceIcon.textContent = '🎤';
        voiceIcon.style.opacity = '0.3';
      }
      if (voiceIndicator) {
        voiceIndicator.title = 'Microphone access denied';
      }
    } else if (event.error !== 'no-speech' && event.error !== 'aborted') {
      // Only restart on certain recoverable errors, and only if we were actively listening
      if (voiceWakeDetected) {
        setTimeout(startVoiceListening, 1000);
      }
    }
  };

  voiceRecognition.onend = () => {
    console.log('Voice recognition ended');
    voiceListening = false;
    // Only auto-restart if we were in the middle of a command
    if (voiceWakeDetected && voiceCommandBuffer) {
      setTimeout(startVoiceListening, 500);
    }
  };

  // Don't auto-start - wait for tap or wake word detection via another method
  // For now, voice is tap-to-activate only
}

function startVoiceListening() {
  if (voiceListening || !voiceRecognition) return;
  
  try {
    voiceRecognition.start();
    voiceListening = true;
    voiceWakeDetected = false;
    voiceCommandBuffer = '';
    const voiceIcon = document.getElementById('voice-icon');
    if (voiceIcon) {
      voiceIcon.textContent = '🎤';
      voiceIcon.style.opacity = '1';
      voiceIcon.style.background = '';
    }
    if (voiceIndicator) {
      voiceIndicator.title = 'Tap mic to speak';
    }
    console.log('Voice listening started');
  } catch (e) {
    console.log('Voice start error:', e);
  }
}

function activateVoiceCommand() {
  voiceWakeDetected = true;
  voiceCommandBuffer = '';
  const voiceIcon = document.getElementById('voice-icon');
  if (voiceIcon) {
    voiceIcon.textContent = '🎤';
    voiceIcon.style.background = '#f5a623';
    voiceIcon.style.color = 'white';
    voiceIcon.style.borderRadius = '50%';
    voiceIcon.style.padding = '3px 5px';
  }
  if (voiceIndicator) {
    voiceIndicator.title = 'Listening...';
  }
  // Audio feedback
  try {
    const audio = new AudioContext();
    const osc = audio.createOscillator();
    const gain = audio.createGain();
    osc.connect(gain);
    gain.connect(audio.destination);
    osc.frequency.value = 800;
    gain.gain.value = 0.1;
    osc.start();
    osc.stop(audio.currentTime + 0.1);
  } catch(e) {}
  
  // Timeout if no command received
  if (voiceTimeout) clearTimeout(voiceTimeout);
  voiceTimeout = setTimeout(() => {
    if (voiceWakeDetected && !voiceCommandBuffer) {
      resetVoiceState();
    }
  }, 5000);
}

function resetVoiceState() {
  voiceWakeDetected = false;
  voiceCommandBuffer = '';
  const voiceIcon = document.getElementById('voice-icon');
  if (voiceIcon) {
    voiceIcon.textContent = '🎤';
    voiceIcon.style.background = '';
    voiceIcon.style.color = '';
    voiceIcon.style.borderRadius = '';
    voiceIcon.style.padding = '';
  }
  if (voiceIndicator) {
    voiceIndicator.title = 'Tap mic to speak';
  }
}

function executeVoiceCommand(command) {
  console.log('Executing voice command:', command);
  
  const voiceIcon = document.getElementById('voice-icon');
  if (voiceIcon) {
    voiceIcon.textContent = '...';
    voiceIcon.style.background = '';
    voiceIcon.style.color = '';
  }
  if (voiceIndicator) {
    voiceIndicator.title = 'Processing: ' + command;
  }

  // Send to agent
  fetch('/agent/run', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ task: command })
  })
  .then(r => r.json())
  .then(result => {
    console.log('Agent result:', result);
    const voiceIcon = document.getElementById('voice-icon');
    
    if (result.success) {
      // Handle navigation actions (like video play)
      if (result.action === 'navigate' && result.url) {
        if (voiceIcon) voiceIcon.textContent = '✓';
        if (voiceIndicator) voiceIndicator.title = result.answer || 'Done!';
        // Navigate to the URL (video will autoplay)
        setTimeout(() => {
          window.location.href = result.url;
        }, 500);
        return;
      }
      
      // Show success feedback
      if (voiceIcon) voiceIcon.textContent = '✓';
      if (voiceIndicator) voiceIndicator.title = result.answer || 'Done!';
      
      // Speak the response if available
      if (result.answer && 'speechSynthesis' in window) {
        const utterance = new SpeechSynthesisUtterance(result.answer.substring(0, 200));
        utterance.rate = 1.1;
        speechSynthesis.speak(utterance);
      }
    } else {
      if (voiceIcon) voiceIcon.textContent = '!';
      if (voiceIndicator) voiceIndicator.title = result.answer || 'Something went wrong';
    }
    
    // Reset after a moment
    setTimeout(resetVoiceState, 3000);
  })
  .catch(err => {
    console.error('Voice command error:', err);
    const voiceIcon = document.getElementById('voice-icon');
    if (voiceIcon) voiceIcon.textContent = '!';
    if (voiceIndicator) voiceIndicator.title = 'Error processing command';
    setTimeout(resetVoiceState, 3000);
  });

  // Reset state for next command
  voiceWakeDetected = false;
  voiceCommandBuffer = '';
}

// Initialize voice assistant after session is established
function tryInitVoiceAssistant() {
  if (isAuthenticated && !voiceRecognition) {
    initVoiceAssistant();
  }
}

// ============================================
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
  
  if (count === 0) {
    presenceContent.innerHTML = '<span class="info">No one else is here right now</span>';
  } else if (count === 1) {
    const userLink = '<a href="/@' + users[0] + '" style="color: inherit;">@' + users[0] + '</a>';
    presenceContent.innerHTML = userLink + ' is here';
  } else if (count <= 5) {
    const userLinks = users.map(u => '<a href="/@' + u + '" style="color: inherit;">@' + u + '</a>').join(', ');
    presenceContent.innerHTML = userLinks + ' are here';
  } else {
    // Show first 3 users and count of others
    const firstThree = users.slice(0, 3).map(u => '<a href="/@' + u + '" style="color: inherit;">@' + u + '</a>').join(', ');
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
  { id: 'apps', title: 'Apps' },
  { id: 'news', title: 'News' },
  { id: 'reminder', title: 'Reminder' },
  { id: 'markets', title: 'Markets' },
  { id: 'blog', title: 'Blog' },
  { id: 'video', title: 'Video' }
];

function initCardCustomization() {
  // Add customize link below page title
  const pageTitle = document.getElementById('page-title');
  if (pageTitle && pageTitle.textContent === 'Home' && !document.getElementById('customize-link')) {
    const link = document.createElement('a');
    link.id = 'customize-link';
    link.href = '#';
    link.textContent = 'Customize';
    link.style.cssText = 'font-size: 13px; font-weight: normal; color: var(--text-muted); display: block; text-align: right; margin-top: -20px; margin-bottom: 15px;';
    link.onclick = (e) => { e.preventDefault(); showCardModal(); };
    pageTitle.insertAdjacentElement('afterend', link);
  }
}

function showCardModal() {
  const hidden = JSON.parse(localStorage.getItem('mu_hidden_cards') || '[]');
  
  // Build checkbox list from available cards
  let checkboxes = '';
  availableCards.forEach(card => {
    const checked = !hidden.includes(card.id) ? 'checked' : '';
    checkboxes += `<label style="display: block; margin: 12px 0; cursor: pointer;"><input type="checkbox" ${checked} data-card-id="${card.id}" style="margin-right: 8px;"> ${card.title}</label>`;
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

} // End of window context check
