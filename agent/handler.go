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
		http.Redirect(w, r, "/login?redirect=/agent", 302)
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
<style>
.agent-container {
	width: 100%;
}
.agent-input {
	display: flex;
	gap: 10px;
	margin-bottom: 20px;
}
.agent-input input {
	flex: 1;
	padding: 12px 16px;
	border: 1px solid var(--card-border, #e8e8e8);
	border-radius: var(--border-radius, 6px);
	font-size: 16px;
	font-family: inherit;
}
.agent-input input:focus {
	outline: none;
	border-color: var(--accent-color, #0d7377);
}
.agent-input button {
	padding: 12px 24px;
	background: var(--accent-color, #0d7377);
	color: white;
	border: none;
	border-radius: var(--border-radius, 6px);
	cursor: pointer;
	font-size: 16px;
}
.agent-input button:hover {
	opacity: 0.9;
}
.agent-input button:disabled {
	opacity: 0.5;
	cursor: not-allowed;
}
.agent-output {
	background: var(--card-background, #fff);
	border: 1px solid var(--card-border, #e8e8e8);
	border-radius: var(--border-radius, 6px);
	padding: 20px;
	min-height: 100px;
}
.agent-output.loading {
	display: flex;
	align-items: center;
	justify-content: center;
	color: var(--text-muted, #888);
}
.agent-output .answer {
	font-size: 16px;
	line-height: 1.6;
}
.agent-output .steps {
	margin-top: 15px;
	padding-top: 15px;
	border-top: 1px solid var(--divider, #f0f0f0);
	font-size: 13px;
	color: var(--text-secondary, #555);
}
.agent-output .step {
	margin: 8px 0;
	padding: 8px;
	background: var(--hover-background, #fafafa);
	border-radius: 4px;
}
.agent-output .step .thought {
	font-style: italic;
	color: var(--text-muted, #888);
}
.agent-output .step .tool {
	font-family: monospace;
	background: #eee;
	padding: 2px 6px;
	border-radius: 3px;
}
.agent-output .duration {
	font-size: 12px;
	color: var(--text-muted, #888);
	margin-top: 10px;
}
.progress {
	display: flex;
	flex-direction: column;
	gap: 12px;
}
.step-progress {
	padding: 12px;
	background: var(--hover-background, #fafafa);
	border-radius: 6px;
	border-left: 3px solid var(--accent-color, #0d7377);
}
.step-progress.running {
	animation: pulse 1.5s ease-in-out infinite;
}
.step-progress .icon {
	font-size: 14px;
}
.step-progress .tool {
	font-family: monospace;
	font-weight: bold;
}
.step-progress .thought {
	font-size: 13px;
	color: var(--text-muted, #888);
	margin-top: 4px;
}
@keyframes pulse {
	0%, 100% { opacity: 1; }
	50% { opacity: 0.6; }
}
.spinner {
	display: inline-block;
	width: 16px;
	height: 16px;
	border: 2px solid var(--accent-color, #0d7377);
	border-top-color: transparent;
	border-radius: 50%;
	animation: spin 0.8s linear infinite;
	margin-right: 8px;
	vertical-align: middle;
}
@keyframes spin {
	to { transform: rotate(360deg); }
}
.agent-results {
	margin: 15px 0;
}
.agent-results .video-result.primary img {
	max-width: 100%;
	border-radius: 8px;
	margin-bottom: 10px;
}
.agent-results .video-result {
	margin: 5px 0;
}
.agent-results .news-result.primary {
	padding: 10px;
	background: var(--hover-background, #fafafa);
	border-radius: 6px;
	margin-bottom: 10px;
}
.agent-results .news-result {
	margin: 5px 0;
}
.examples {
	margin-top: 20px;
	font-size: 14px;
	color: var(--text-secondary, #555);
}
.examples ul {
	margin: 10px 0;
	padding-left: 20px;
}
.examples li {
	margin: 5px 0;
	cursor: pointer;
}
.examples li:hover {
	color: var(--accent-color, #0d7377);
}
.examples li code {
	background: var(--hover-background, #fafafa);
	padding: 2px 8px;
	border-radius: 4px;
}
</style>

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
	let html = '<div class="progress">';
	for (const step of steps) {
		const icon = step.result ? (step.result.success ? '✓' : '✗') : '...';
		const status = step.result ? '' : ' running';
		html += '<div class="step-progress' + status + '">';
		html += '<span class="icon">' + icon + '</span> ';
		html += '<span class="tool">' + step.tool + '</span>';
		if (step.thought) {
			html += '<div class="thought">' + step.thought + '</div>';
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
				html += '<p style="margin-top:15px"><a href="' + result.url + '" class="action-link" style="display:inline-block;padding:12px 24px;background:var(--accent-color,#0d7377);color:white;text-decoration:none;border-radius:6px;">▶ Play Video</a></p>';
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
			if (step.thought) html += '<div class="thought">' + step.thought + '</div>';
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
</script>
`

	html := app.RenderHTML("Agent", "Mu Agent", content)
	w.Write([]byte(html))
}

// handleRun executes an agent task
func handleRun(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		Task string `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}

	if req.Task == "" {
		http.Error(w, "Task is required", 400)
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
