var topic="";
let roomWs;
let currentRoomId = null;

function connectRoomWebSocket(roomId) {
  if (roomWs && roomWs.readyState === WebSocket.OPEN && currentRoomId === roomId) {
    return;
  }
  
  if (roomWs) {
    roomWs.close();
  }
  
  const isReconnect = currentRoomId === roomId;
  currentRoomId = roomId;
  
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
      displayRoomMessage(msg, true);
    }
  };
  
  roomWs.onclose = function(event) {
    console.log('Disconnected from room');
    if (event.code === 1013 && event.reason) {
      showToast(event.reason, 'error');
    }
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

  const when = msg.timestamp ? Math.floor(Date.parse(msg.timestamp) / 1000) : 0;
  const timeSpan = when > 0 ?
    ' <span class="msg-when" data-timestamp="' + when + '">' + timeAgo(when) + '</span>' : '';

  if (msg.system) {
    msgDiv.className = 'message message-system';
    msgDiv.textContent = msg.content;
    if (timeSpan) msgDiv.innerHTML = msgDiv.innerHTML + timeSpan;
    messagesDiv.appendChild(msgDiv);
    if (shouldScroll) messagesDiv.scrollTop = messagesDiv.scrollHeight;
    return;
  }

  const userSpan = msg.is_llm ?
    '<span class="llm"><a href="/@micro" style="color:inherit;text-decoration:none;">micro</a>' + timeSpan + '</span>' :
    '<span class="you"><a href="/@' + msg.username + '">' + msg.username + '</a>' + timeSpan + '</span>';

  let content;
  if (msg.is_llm) {
    content = renderMarkdown(msg.content);
  } else {
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

function linkifyText(text) {
  const urlRegex = /(https?:\/\/[^\s]+)/g;
  return text.replace(urlRegex, '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>');
}

function renderMarkdown(text) {
  var esc = function (s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  };

  var fences = [];
  var src = esc(text).replace(/```[a-z]*\n?([\s\S]*?)```/g, function (_, code) {
    fences.push(code);
    return '\u0000FENCE' + (fences.length - 1) + '\u0000';
  });

  var inline = function (s) {
    return s
      .replace(/`([^`]+?)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+?)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*([^*]+?)\*/g, '$1<em>$2</em>')
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
    var fence = line.match(/^\u0000FENCE(\d+)\u0000$/);
    if (fence) {
      flushPara(); closeList();
      out.push('<pre><code>' + fences[+fence[1]] + '</code></pre>');
      continue;
    }
    if (!line.trim()) { flushPara(); closeList(); continue; }

    var heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      flushPara(); closeList();
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
    var messagesDiv = document.getElementById('messages');
    if (!messagesDiv) return;
    container = document.createElement('div');
    container.id = 'chat-users';
    messagesDiv.insertBefore(container, messagesDiv.firstChild);
  }
  container.replaceChildren();
  (users || []).forEach(function(u) {
    var link = document.createElement('a');
    link.href = '/@' + encodeURIComponent(u);
    link.title = 'View profile';
    link.textContent = '@' + u;
    container.appendChild(link);
  });
}

function sendRoomMessage(form) {
  const input = form.querySelector('[name="prompt"]');
  if (!input) return;
  
  const content = input.value.trim();
  
  if (content && roomWs && roomWs.readyState === WebSocket.OPEN) {
    roomWs.send(JSON.stringify({ content: content }));
    input.value = '';
  }
}

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

function initRoomChat() {
  const currentRoomData = roomFromPage();

  if (!currentRoomData) {
    if (roomWs) { roomWs.close(); roomWs = null; }
    currentRoomId = null;
    return;
  }

  {
    topic = currentRoomData.title;
    
    const topicInput = document.getElementById('topic');
    if (topicInput) {
      topicInput.value = currentRoomData.title;
    }

    connectRoomWebSocket(currentRoomData.id);

    const chatForm = document.getElementById('chat-form');
    if (chatForm) {
      chatForm.onsubmit = function(e) {
        e.preventDefault();
        sendRoomMessage(this);
        return false;
      };
      
      const input = chatForm.querySelector('[name="prompt"]');
      if (input) {
        input.placeholder = 'Type your message...';
      }
    }
  }
}

document.addEventListener('DOMContentLoaded', initRoomChat);

