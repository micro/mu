package agent

import (
	"encoding/json"
	"net/http"
	"strings"

	"mu/app"
	"mu/auth"
)

// Handler handles /agent routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/agent")
	path = strings.TrimPrefix(path, "/")

	// Get session (required)
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login?redirect=/agent", 302)
		return
	}

	switch {
	case path == "" || path == "/":
		handleAgentUI(w, r, sess)
	case path == "run":
		handleRun(w, r, sess)
	default:
		http.NotFound(w, r)
	}
}

// handleAgentUI serves the agent interface
func handleAgentUI(w http.ResponseWriter, r *http.Request, sess *auth.Session) {
	content := `
<style>
.agent-container {
	max-width: 700px;
	margin: 0 auto;
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

async function runAgent() {
	const task = input.value.trim();
	if (!task) return;
	
	btn.disabled = true;
	output.innerHTML = '<div class="loading">Working...</div>';
	output.classList.add('loading');
	
	try {
		const resp = await fetch('/agent/run', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({task: task})
		});
		
		const result = await resp.json();
		
		let html = '';
		
		if (result.success) {
			html += '<div class="answer">' + formatMarkdown(result.answer) + '</div>';
			
			if (result.html) {
				html += result.html;
			}
			
			// Auto-navigate for video play actions
			if (result.action === 'navigate' && result.url) {
				if (result.url.includes('/video?id=')) {
					// For videos, show play button prominently and auto-redirect
					html += '<p style="margin-top:15px"><a href="' + result.url + '" class="action-link" style="display:inline-block;padding:12px 24px;background:var(--accent-color,#0d7377);color:white;text-decoration:none;border-radius:6px;">▶ Play Video</a></p>';
					// Auto-redirect after a short delay
					setTimeout(() => { window.location.href = result.url; }, 1500);
				} else {
					html += '<p><a href="' + result.url + '" class="action-link">→ Go there now</a></p>';
				}
			}
		} else {
			html += '<div class="answer error">' + (result.answer || 'Something went wrong') + '</div>';
		}
		
		// Show steps (collapsed by default for successful results)
		if (result.steps && result.steps.length > 0) {
			html += '<details class="steps"><summary>Steps (' + result.steps.length + ')</summary>';
			for (const step of result.steps) {
				html += '<div class="step">';
				if (step.thought) {
					html += '<div class="thought">' + step.thought + '</div>';
				}
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
		output.classList.remove('loading');
		
	} catch (err) {
		output.innerHTML = '<div class="answer error">Error: ' + err.message + '</div>';
		output.classList.remove('loading');
	}
	
	btn.disabled = false;
}

function formatMarkdown(text) {
	if (!text) return '';
	// Basic markdown formatting
	return text
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

	// Create and run agent
	agent := New(sess.Account)
	result := agent.Run(req.Task)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
