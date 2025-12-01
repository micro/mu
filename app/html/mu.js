// ============================================
// SERVICE WORKER CONFIGURATION
// ============================================
var APP_PREFIX = 'mu_';
var VERSION = 'v64';
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
  
  // Add context change message and summary to chat
  const messages = document.getElementById('messages');
  if (messages) {
    const contextMsg = document.createElement('div');
    contextMsg.className = 'context-message';
    contextMsg.textContent = `Context set to ${t} - questions will be enhanced with ${t}-related information`;
    messages.appendChild(contextMsg);
    
    // Show summary for this topic if available
    if (typeof summaries !== 'undefined' && summaries[t]) {
      const summaryMsg = document.createElement('div');
      summaryMsg.className = 'message';
      summaryMsg.innerHTML = `<span class="llm">AI Brief</span>${summaries[t]}`;
      messages.appendChild(summaryMsg);
    }
    
    messages.scrollTop = messages.scrollHeight;
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
  responseDiv.innerHTML = `<span class="llm">AI</span><div class="llm-response"></div>`;
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
  
  // Only load local conversation history if NOT in a room
  // Rooms use WebSocket and server-side message history
  const isRoom = (typeof roomData !== 'undefined' && roomData && roomData.id);
  if (!isRoom) {
    loadContext();
    loadMessages();
  }
  
  // If no conversation exists and not in a room, show general summaries
  if (!isRoom && context.length === 0 && typeof summaries !== 'undefined') {
    const messages = document.getElementById('messages');
    const topics = Object.keys(summaries).sort();
    
    topics.forEach(topic => {
      if (summaries[topic]) {
        const summaryMsg = document.createElement('div');
        summaryMsg.className = 'message';
        summaryMsg.innerHTML = `<span class="llm">${topic}</span>${summaries[topic]}`;
        messages.appendChild(summaryMsg);
      }
    });
  }
  
  // Check if there's a hash in the URL to set active topic
  if (window.location.hash) {
    const hash = window.location.hash.substring(1);
    switchToTopicIfExists(hash);
  }
  // Otherwise no topic is selected by default

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
    var navLoggedIn = document.getElementById("nav-logged-in");
    var navLoggedOut = document.getElementById("nav-logged-out");
    var accountHeader = document.getElementById("account-header");
    if (sess.type == "account") {
      if (navLoggedIn) navLoggedIn.style.display = '';
      if (navLoggedOut) navLoggedOut.style.display = 'none';
      if (accountHeader) accountHeader.style.display = 'block';
    } else {
      if (navLoggedIn) navLoggedIn.style.display = 'none';
      if (navLoggedOut) navLoggedOut.style.display = '';
      if (accountHeader) accountHeader.style.display = 'none';
      // If we're on a protected page but not logged in, redirect
      const protectedPaths = ['/home', '/chat', '/blog', '/news', '/video', '/posts'];
      if (protectedPaths.includes(window.location.pathname)) {
        window.location.href = '/';
      }
    }
  })
  .catch(error => {
    console.error('Error:', error);
    var navLoggedIn = document.getElementById("nav-logged-in");
    var navLoggedOut = document.getElementById("nav-logged-out");
    if (navLoggedIn) navLoggedIn.style.display = 'none';
    if (navLoggedOut) navLoggedOut.style.display = '';
    // On error, redirect to home (but not if viewing public pages)
    const publicPaths = ['/post', '/membership', '/donate'];
    const isPublicPath = publicPaths.some(path => window.location.pathname.startsWith(path));
    if (!isPublicPath) {
      window.location.href = '/';
    }
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
        
        // If we're in a room, navigate to regular chat with the topic hash
        const urlParams = new URLSearchParams(window.location.search);
        if (urlParams.has('id')) {
          window.location.href = '/chat#' + topicName;
        } else {
          // Regular chat - just switch topic and update hash
          switchTopic(topicName);
          history.pushState(null, null, '#' + topicName);
        }
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
  if (roomWs) {
    roomWs.close();
  }
  
  currentRoomId = roomId;
  
  // Load and display messages from sessionStorage
  const messagesDiv = document.getElementById('messages');
  if (messagesDiv) {
    messagesDiv.innerHTML = '';
    const savedMessages = loadMessagesFromSession(roomId);
    savedMessages.forEach(msg => displayRoomMessage(msg, false));
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
    if (currentRoomId === roomId) {
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
    '<span class="llm">AI</span>' : 
    '<span class="you">' + msg.username + '</span>';
  
  let content;
  if (msg.is_llm) {
    // Render markdown for AI messages
    content = renderMarkdown(msg.content);
  } else {
    // Escape HTML for user messages
    content = msg.content.replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\n/g, '<br>');
  }
  
  msgDiv.innerHTML = userSpan + '<p>' + content + '</p>';
  messagesDiv.appendChild(msgDiv);
  
  if (shouldScroll) {
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
  }
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
  const topicSelector = document.getElementById('topic-selector') || document.getElementById('topics');
  if (!topicSelector) return;
  
  let userListDiv = topicSelector.querySelector('.user-list');
  if (!userListDiv) {
    userListDiv = document.createElement('div');
    userListDiv.className = 'user-list';
    userListDiv.style.cssText = 'padding: 5px 0; color: #666; font-size: small;';
    topicSelector.appendChild(userListDiv);
  }
  
  if (users && users.length > 0) {
    userListDiv.innerHTML = '<strong>In room:</strong> ' + users.map(u => '@' + u).join(', ');
  } else {
    userListDiv.innerHTML = '';
  }
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
    
    // Add context message to messages area with room summary
    const messages = document.getElementById('messages');
    if (messages) {
      const contextMsg = document.createElement('div');
      contextMsg.className = 'context-message';
      contextMsg.innerHTML = 'Discussion: <strong>' + currentRoomData.title + '</strong><br>' + 
        '<span style="color: #666;">' + currentRoomData.summary + '</span>' +
        (currentRoomData.url ? '<br><a href="' + currentRoomData.url + '" target="_blank" style="color: #0066cc; font-size: 13px;">→ View Original</a>' : '');
      messages.appendChild(contextMsg);
    }
    
    // Connect WebSocket
    connectRoomWebSocket(currentRoomData.id);
    
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

// Validate blog post form on /posts page
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
      charCount.textContent = 'Minimum 50 characters (' + remaining + ' more needed)';
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
  });
  
  updateCharCount();
});

} // End of window context check