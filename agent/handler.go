package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"mu/app"
	"mu/auth"
	"mu/wallet"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Handler handles /agent routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/agent")
	path = strings.TrimPrefix(path, "/")

	// Get session (required)
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	switch {
	case path == "" || path == "/":
		handleAgentUI(w, r, sess)
	case path == "run":
		handleRun(w, r, sess)
	case path == "ws":
		handleWebSocket(w, r, sess)
	default:
		http.NotFound(w, r)
	}
}

// handleAgentUI serves the agent interface
func handleAgentUI(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	content := `
<div class="agent-container">
	<div class="agent-input">
		<input type="text" id="task-input" placeholder="What would you like me to do?" autofocus>
		<button id="run-btn" onclick="runAgent()">Go</button>
	</div>
	
	<div class="agent-output" id="output">
		<div class="examples">
			<strong>Try saying:</strong>
			<ul>
				<li onclick="setTask(this)"><code>Play bingo songs</code></li>
				<li onclick="setTask(this)"><code>Find news about AI</code></li>
				<li onclick="setTask(this)"><code>What's the price of Bitcoin?</code></li>
				<li onclick="setTask(this)"><code>Create an app that tracks my water intake</code></li>
				<li onclick="setTask(this)"><code>Show my apps</code></li>
			</ul>
		</div>
	</div>
</div>

<script>
const input = document.getElementById('task-input');
const output = document.getElementById('output');
const btn = document.getElementById('run-btn');
let ws = null;
let steps = [];
let actionData = null;

function setTask(el) {
	input.value = el.querySelector('code').textContent;
	input.focus();
}

input.addEventListener('keydown', (e) => {
	if (e.key === 'Enter' && !e.shiftKey) {
		e.preventDefault();
		runAgent();
	}
});

function runAgent() {
	const task = input.value.trim();
	if (!task) return;
	
	btn.disabled = true;
	steps = [];
	actionData = null;
	output.innerHTML = '<div class="loading"><span class="spinner"></span> Working...</div>';
	output.classList.add('loading');
	
	// Use WebSocket for streaming
	const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
	ws = new WebSocket(wsProtocol + '//' + location.host + '/agent/ws');
	
	ws.onopen = () => {
		ws.send(JSON.stringify({task: task}));
	};
	
	ws.onmessage = (event) => {
		const msg = JSON.parse(event.data);
		
		if (msg.type === 'step') {
			steps.push(msg.step);
			updateProgress();
		} else if (msg.type === 'done') {
			showResult(msg.result);
			ws.close();
		} else if (msg.type === 'error') {
			output.innerHTML = '<div class="answer error">' + msg.error + '</div>';
			output.classList.remove('loading');
			btn.disabled = false;
			ws.close();
		}
	};
	
	ws.onerror = () => {
		output.innerHTML = '<div class="answer error">Connection error</div>';
		output.classList.remove('loading');
		btn.disabled = false;
	};
	
	ws.onclose = () => {
		btn.disabled = false;
	};
}

function updateProgress() {
	let html = '<div class="agent-progress">';
	for (const step of steps) {
		const icon = step.result ? (step.result.success ? '✓' : '✗') : '...';
		const status = step.result ? '' : ' running';
		html += '<div class="step-progress' + status + '">';
		html += '<span class="icon">' + icon + '</span> ';
		html += '<span class="tool">' + step.tool + '</span>';
		if (step.reasoning) {
			html += '<div class="reasoning">' + step.reasoning + '</div>';
		}
		// Show HTML results inline (like video thumbnails)
		if (step.result && step.result.html) {
			html += step.result.html;
		}
		html += '</div>';
	}
	html += '</div>';
	output.innerHTML = html;
	output.classList.remove('loading');
}

function showResult(result) {
	let html = '';
	
	if (result.success) {
		html += '<div class="answer">' + formatMarkdown(result.answer) + '</div>';
		
		if (result.html) {
			html += result.html;
		}
		
		if (result.action === 'navigate' && result.url) {
			if (result.url.includes('/video?id=')) {
				html += '<p style="margin-top:15px"><a href="' + result.url + '" class="action-link" style="display:inline-block;padding:8px 12px;background:var(--btn-primary,#000);color:white;text-decoration:none;border-radius:6px;">▶ Play Video</a></p>';
				setTimeout(() => { window.location.href = result.url; }, 1500);
			} else {
				html += '<p><a href="' + result.url + '" class="action-link">→ Go there now</a></p>';
			}
		}
	} else {
		html += '<div class="answer error">' + (result.answer || 'Something went wrong') + '</div>';
	}
	
	if (steps.length > 0) {
		html += '<details class="steps"><summary>Steps (' + steps.length + ')</summary>';
		for (const step of steps) {
			html += '<div class="step">';
			if (step.reasoning) html += '<div class="reasoning">' + step.reasoning + '</div>';
			html += '<span class="tool">' + step.tool + '</span>';
			if (step.result && step.result.error) {
				html += ' <span style="color:red">Error: ' + step.result.error + '</span>';
			}
			html += '</div>';
		}
		html += '</details>';
	}
	
	if (result.duration) {
		html += '<div class="duration">Completed in ' + result.duration + '</div>';
	}
	
	output.innerHTML = html;
}

function formatMarkdown(text) {
	if (!text) return '';
	return text
		.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
		.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
		.replace(/\*(.*?)\*/g, '<em>$1</em>')
		.replace(/\n/g, '<br>');
}

// Auto-run if task param is present
const urlParams = new URLSearchParams(window.location.search);
const taskParam = urlParams.get('task');
if (taskParam) {
	input.value = taskParam;
	runAgent();
}
</script>
`

	html := app.RenderHTML("Agent", "Mu Agent", content)
	w.Write([]byte(html))
}

// handleRun executes an agent task
func handleRun(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}

	var req struct {
		Task string `json:"task"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.BadRequest(w, r, "Invalid request")
		return
	}

	if req.Task == "" {
		app.BadRequest(w, r, "Task is required")
		return
	}

	// Check quota
	canProceed, _, cost, _ := wallet.CheckQuota(sess.Account, wallet.OpAgentRun)
	if !canProceed {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Insufficient credits. This operation costs %d credits.", cost),
		})
		return
	}

	// Create and run agent
	agent := New(sess.Account)
	result := agent.Run(req.Task)

	// Consume quota on success
	if result.Success {
		wallet.ConsumeQuota(sess.Account, wallet.OpAgentRun)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleWebSocket handles WebSocket connections for streaming agent results
func handleWebSocket(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		app.Log("agent", "WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Read the task from the client
	var req struct {
		Task string `json:"task"`
	}
	if err := conn.ReadJSON(&req); err != nil {
		conn.WriteJSON(map[string]interface{}{"type": "error", "error": "Invalid request"})
		return
	}

	if req.Task == "" {
		conn.WriteJSON(map[string]interface{}{"type": "error", "error": "Task is required"})
		return
	}

	app.Log("agent", "WebSocket task from %s: %s", sess.Account, req.Task)

	// Create agent and run with streaming callback
	agent := New(sess.Account)

	result := agent.RunStreaming(req.Task, func(step *Step, final bool) {
		// Send each step as it completes
		conn.WriteJSON(map[string]interface{}{
			"type": "step",
			"step": step,
		})
	})

	// Send final result
	conn.WriteJSON(map[string]interface{}{
		"type":   "done",
		"result": result,
	})
}
