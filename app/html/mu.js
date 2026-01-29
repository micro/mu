// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v126';
var CACHE_NAME = APP_PREFIX + VERSION;

// Minimal caching - only icons
var STATIC_CACHE = [
  '/mu.png',
  '/home.png',
  '/audio.png',
  '/chat.png',
  '/mail.png',
  '/post.png',
  '/news.png',
  '/video.png',
  '/wallet.png',
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
  
  // Build all summaries content with join links inside each
  let allSummariesHtml = '';
  topics.forEach(t => {
    if (summaries[t]) {
      const joinLink = `<a href="/chat?id=chat_${encodeURIComponent(t)}" class="link">Join discussion →</a>`;
      allSummariesHtml += `<div class="summary-item"><span class="category">${t}</span><p>${summaries[t]}</p>${joinLink}</div>`;
    }
  });
  
  // Create a single collapsible summary card
  const summaryCard = document.createElement('div');
  summaryCard.className = 'message summary-card';
  summaryCard.innerHTML = `
    <div class="summary-header" onclick="toggleAllSummaries()">
      <strong>Today's Topics</strong>
      <span id="summary-toggle-icon">▶</span>
    </div>
    <div id="all-summaries" class="all-summaries" style="display: none;">
      ${allSummariesHtml}
    </div>
  `;
  messages.appendChild(summaryCard);
}

// Toggle all summaries visibility
function toggleAllSummaries() {
  const summaries = document.getElementById('all-summaries');
  const icon = document.getElementById('summary-toggle-icon');
  if (summaries.style.display === 'none') {
    summaries.style.display = 'block';
    icon.textContent = '▼';
  } else {
    summaries.style.display = 'none';
    icon.textContent = '▶';
  }
}

// Toggle summary visibility (legacy, for individual topic pages)
function toggleSummary(summaryId) {
  const summary = document.getElementById(summaryId);
  const toggle = summary.previousElementSibling;
  if (summary.style.display === 'none') {
    summary.style.display = 'block';
    toggle.textContent = 'Hide summary';
  } else {
    summary.style.display = 'none';
    toggle.textContent = 'Show summary';
  }
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
  
  // Show context message with summary displayed directly
  const messages = document.getElementById('messages');
  if (messages) {
    messages.innerHTML = '';
    const contextMsg = document.createElement('div');
    contextMsg.className = 'context-message';
    let summaryHtml = '';
    if (typeof summaries !== 'undefined' && summaries[t]) {
      summaryHtml = `<p class="topic-summary">${summaries[t]}</p>`;
    }
    contextMsg.innerHTML = '<strong>' + t + '</strong>' + summaryHtml;
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
        showToast('Please login to chat', 'error');
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
    showToast('Please login to chat', 'error');
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
    var navMail = document.getElementById("nav-mail");
    var navWallet = document.getElementById("nav-wallet");
    var navAccount = document.getElementById("nav-account");
    var navLogout = document.getElementById("nav-logout");
    var navLogin = document.getElementById("nav-login");
    var navMailBadge = document.getElementById("nav-mail-badge");
    var navUsername = document.getElementById("nav-username");
    
    if (sess.type == "account") {
      isAuthenticated = true;
      // Show authenticated nav items
      if (navMail) navMail.style.display = 'flex';
      if (navWallet) navWallet.style.display = 'flex';
      if (navAccount) navAccount.style.display = 'flex';
      if (navLogout) navLogout.style.display = 'flex';
      if (navLogin) navLogin.style.display = 'none';
      if (navUsername && sess.account) {
        navUsername.textContent = 'Signed in as @' + sess.account;
        navUsername.style.display = 'block';
      }
      // Fetch unread mail count for badge
      fetch('/mail?unread=count')
        .then(res => res.json())
        .then(data => {
          if (data.count > 0 && navMailBadge) {
            navMailBadge.textContent = data.count > 9 ? '9+' : data.count;
          }
        })
        .catch(() => {});
      // Initialize card customization for home page
      if (window.location.pathname === '/home') {
        initCardCustomization();
      }
    } else {
      isAuthenticated = false;
      // Hide authenticated nav items, show login
      if (navMail) navMail.style.display = 'none';
      if (navWallet) navWallet.style.display = 'none';
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
    var navMail = document.getElementById("nav-mail");
    var navWallet = document.getElementById("nav-wallet");
    var navAccount = document.getElementById("nav-account");
    var navLogout = document.getElementById("nav-logout");
    var navLogin = document.getElementById("nav-login");
    if (navMail) navMail.style.display = 'none';
    if (navWallet) navWallet.style.display = 'none';
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
  
  // set nav active state
  var nav = document.getElementById("nav");
  var navContainer = document.getElementById("nav-container");
  for (const el of nav.children) {
    // Skip non-link elements (spacer, bottom container)
    if (el.tagName !== 'A') continue;
    if (el.getAttribute("href") == window.location.pathname) {
      el.classList.add("active");
    } else {
      el.classList.remove("active");
    }
  }
  
  // Show scroll indicator if nav has overflow
  function checkNavScroll() {
    if (nav && navContainer && nav.scrollHeight > nav.clientHeight && nav.scrollTop < nav.scrollHeight - nav.clientHeight - 10) {
      navContainer.classList.add('has-scroll');
    } else if (navContainer) {
      navContainer.classList.remove('has-scroll');
    }
  }
  // Check after layout settles and on load
  if (nav) nav.addEventListener('scroll', checkNavScroll);
  window.addEventListener('resize', checkNavScroll);
  window.addEventListener('load', checkNavScroll);
  setTimeout(checkNavScroll, 100);

  // load chat
  if (window.location.pathname == CHAT_PATH) {
    loadChat();
    
    // Add click handlers for chat topics - always switch rooms
    document.querySelectorAll(CHAT_TOPIC_SELECTOR).forEach(link => {
      link.addEventListener('click', function(e) {
        const topicName = this.textContent;
        // "All" link should navigate to /chat (main chat), not create a room
        if (topicName === 'All') {
          // Let the default navigation happen
          return;
        }
        e.preventDefault();
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

// Flag a post
async function flagPost(postId) {
  if (!confirm('Flag this post as inappropriate? It will be hidden after 3 flags.')) {
    return;
  }
  
  const result = await apiCall('/flag', { body: { type: 'post', id: postId } });
  
  if (result.ok && result.data.success) {
    showToast('Post flagged. Flag count: ' + result.data.count, 'success');
    if (result.data.count >= 3) {
      setTimeout(() => location.reload(), 1000);
    }
  }
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
          
          // Show summary expanded by default for item discussions
          let summaryHtml = '';
          if (currentRoomData.summary) {
            const summaryId = 'room-summary';
            summaryHtml = `<br><a href="#" class="summary-toggle" onclick="toggleSummary('${summaryId}'); return false;">Hide summary</a>` +
              `<span id="${summaryId}" class="summary-content" style="display: block; color: #666;"><br>${currentRoomData.summary}</span>`;
          }
          
          contextMsg.innerHTML = 'Discussion: <strong>' + currentRoomData.title + '</strong>' + 
            summaryHtml +
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
