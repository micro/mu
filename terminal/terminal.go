package terminal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"mu/app"
	"mu/auth"
)

// Template renders the terminal UI page
var Template = `
<div id="terminal-container">
  <div id="terminal-output"></div>
  <div id="terminal-input-line">
    <span id="terminal-prompt">$ </span>
    <input id="terminal-input" type="text" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" autofocus>
  </div>
</div>
<style>
  #terminal-container {
    background: #1a1a1a;
    color: #e0e0e0;
    font-family: 'Courier New', Courier, monospace;
    font-size: 14px;
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    display: flex;
    flex-direction: column;
    min-height: 400px;
    max-height: 70vh;
    overflow: hidden;
  }
  #terminal-output {
    flex: 1;
    overflow-y: auto;
    white-space: pre-wrap;
    word-wrap: break-word;
    line-height: 1.4;
    padding-bottom: var(--spacing-sm);
  }
  #terminal-output .cmd-line {
    color: #4fc3f7;
  }
  #terminal-output .error-line {
    color: #ef5350;
  }
  #terminal-input-line {
    display: flex;
    align-items: center;
    border-top: 1px solid #333;
    padding-top: var(--spacing-sm);
    flex-shrink: 0;
  }
  #terminal-prompt {
    color: #4fc3f7;
    margin-right: 4px;
    flex-shrink: 0;
  }
  #terminal-input {
    flex: 1;
    background: transparent;
    border: none;
    color: #e0e0e0;
    font-family: inherit;
    font-size: inherit;
    outline: none;
    padding: 4px 0;
    caret-color: #4fc3f7;
  }
  @media (max-width: 600px) {
    #terminal-container {
      font-size: 13px;
      min-height: 300px;
      max-height: 60vh;
      padding: var(--spacing-sm);
    }
  }
</style>
<script>
(function() {
  var ws;
  var output = document.getElementById('terminal-output');
  var input = document.getElementById('terminal-input');
  var prompt = document.getElementById('terminal-prompt');
  var history = [];
  var historyIndex = -1;

  function connect() {
    var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(proto + '//' + location.host + '/terminal/ws');

    ws.onopen = function() {
      appendOutput('Connected to terminal.\n', '');
    };

    ws.onmessage = function(e) {
      try {
        var msg = JSON.parse(e.data);
        if (msg.type === 'output') {
          appendOutput(msg.data, '');
        } else if (msg.type === 'error') {
          appendOutput(msg.data, 'error-line');
        } else if (msg.type === 'prompt') {
          prompt.textContent = msg.data;
        }
      } catch(err) {
        appendOutput(e.data, '');
      }
      scrollToBottom();
    };

    ws.onclose = function() {
      appendOutput('\nConnection closed.\n', 'error-line');
    };

    ws.onerror = function() {
      appendOutput('Connection error.\n', 'error-line');
    };
  }

  function appendOutput(text, className) {
    if (!text) return;
    var span = document.createElement('span');
    if (className) span.className = className;
    span.textContent = text;
    output.appendChild(span);
    scrollToBottom();
  }

  function scrollToBottom() {
    output.scrollTop = output.scrollHeight;
  }

  function sendCommand(cmd) {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      appendOutput('Not connected.\n', 'error-line');
      return;
    }
    appendOutput('$ ' + cmd + '\n', 'cmd-line');
    ws.send(JSON.stringify({type: 'input', data: cmd}));
  }

  input.addEventListener('keydown', function(e) {
    if (e.key === 'Enter') {
      var cmd = input.value;
      input.value = '';
      if (cmd.trim()) {
        history.push(cmd);
        historyIndex = history.length;
      }
      sendCommand(cmd);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (historyIndex > 0) {
        historyIndex--;
        input.value = history[historyIndex];
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIndex < history.length - 1) {
        historyIndex++;
        input.value = history[historyIndex];
      } else {
        historyIndex = history.length;
        input.value = '';
      }
    }
  });

  // Focus input on tap/click anywhere in terminal
  document.getElementById('terminal-container').addEventListener('click', function() {
    input.focus();
  });

  connect();
})();
</script>
`

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler serves the terminal page (admin only)
func Handler(w http.ResponseWriter, r *http.Request) {
	// Require admin access
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "ok"})
		return
	}

	html := app.RenderHTMLForRequest("Terminal", "Web terminal", Template, r)
	w.Write([]byte(html))
}

// wsMessage represents a WebSocket message
type wsMessage struct {
	Type string `json:"type"` // "input", "output", "error", "prompt"
	Data string `json:"data"`
}

// WSHandler handles WebSocket connections for the terminal
func WSHandler(w http.ResponseWriter, r *http.Request) {
	// Require admin access
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.Log("terminal", "WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	app.Log("terminal", "New terminal session")

	shell := getShell()

	// Start the shell process
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=dumb")

	// Set working directory to user home or a temp directory
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	} else {
		cmd.Dir = os.TempDir()
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		sendError(conn, "Failed to create stdin pipe")
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sendError(conn, "Failed to create stdout pipe")
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		sendError(conn, "Failed to create stderr pipe")
		return
	}

	if err := cmd.Start(); err != nil {
		sendError(conn, fmt.Sprintf("Failed to start shell: %v", err))
		return
	}

	var wg sync.WaitGroup

	// Read stdout and send to WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				sendOutput(conn, string(buf[:n]))
			}
			if err != nil {
				if err != io.EOF {
					app.Log("terminal", "stdout read error: %v", err)
				}
				return
			}
		}
	}()

	// Read stderr and send to WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				sendError(conn, string(buf[:n]))
			}
			if err != nil {
				if err != io.EOF {
					app.Log("terminal", "stderr read error: %v", err)
				}
				return
			}
		}
	}()

	// Read WebSocket messages and write to stdin
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				app.Log("terminal", "WebSocket read error: %v", err)
			}
			break
		}

		if msg.Type == "input" {
			command := msg.Data + "\n"
			if _, err := io.WriteString(stdin, command); err != nil {
				app.Log("terminal", "stdin write error: %v", err)
				break
			}
		}
	}

	// Cleanup
	stdin.Close()

	// Kill the process if still running
	if cmd.Process != nil {
		cmd.Process.Kill()
	}

	// Wait with timeout
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		app.Log("terminal", "Shell process did not exit in time")
	}

	wg.Wait()
	app.Log("terminal", "Terminal session ended")
}

// getShell returns the shell to use
func getShell() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	// Try common shells
	for _, sh := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	return "sh"
}

// sendOutput sends output to the WebSocket client
func sendOutput(conn *websocket.Conn, data string) {
	msg := wsMessage{Type: "output", Data: data}
	if err := conn.WriteJSON(msg); err != nil {
		app.Log("terminal", "WebSocket write error: %v", err)
	}
}

// sendError sends an error message to the WebSocket client
func sendError(conn *websocket.Conn, data string) {
	msg := wsMessage{Type: "error", Data: data}
	if err := conn.WriteJSON(msg); err != nil {
		app.Log("terminal", "WebSocket write error: %v", err)
	}
}

// RenderPage generates the terminal page HTML (exported for testing)
func RenderPage() string {
	var b strings.Builder
	b.WriteString(Template)
	return b.String()
}
