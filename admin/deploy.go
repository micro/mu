package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
)

var (
	deployMu   sync.Mutex
	deploying  bool
	deployLogs []deployLogEntry
)

type deployLogEntry struct {
	Time    time.Time `json:"time"`
	Step    string    `json:"step"`
	Output  string    `json:"output"`
	Success bool      `json:"success"`
}

// sourceDir returns the source directory for the mu project.
// It checks the MU_SOURCE_DIR env var, then falls back to ~/src/mu.
func sourceDir() string {
	if dir := os.Getenv("MU_SOURCE_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "src", "mu")
}

// UpdateHandler shows the update/restart page and handles requests
func UpdateHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		handleDeploy(w, r)
		return
	}

	// GET — render server page
	content := `<p><a href="/admin">← Admin</a></p>
	<h2>Server</h2>
	<p><strong>Source:</strong> <code>` + sourceDir() + `</code></p>
	<div id="deploy-controls">
		<button id="update-btn" onclick="runAction('update')">Update</button>
		<button id="restart-btn" onclick="runAction('restart')">Restart</button>
	</div>
	<pre id="deploy-output" style="background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:6px;min-height:200px;max-height:500px;overflow-y:auto;font-size:13px;line-height:1.6;white-space:pre-wrap;display:none;"></pre>
	<script>
	function runAction(action) {
		var updateBtn = document.getElementById('update-btn');
		var restartBtn = document.getElementById('restart-btn');
		var output = document.getElementById('deploy-output');
		var label = action === 'update' ? 'Updating...' : 'Restarting...';
		updateBtn.disabled = true;
		restartBtn.disabled = true;
		if (action === 'update') updateBtn.textContent = label;
		else restartBtn.textContent = label;
		output.style.display = 'block';
		output.textContent = '';

		fetch('/admin/server', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({action: action})
		}).then(function(res) { return res.json(); })
		.then(function(data) {
			var lines = '';
			data.logs.forEach(function(entry) {
				var color = entry.success ? '#6a9955' : '#f44747';
				var icon = entry.success ? '✓' : '✗';
				lines += '<span style="color:' + color + ';">' + icon + ' ' + entry.step + '</span>\n';
				if (entry.output) {
					lines += entry.output + '\n';
				}
			});
			var doneLabel = action === 'update' ? 'Update' : 'Restart';
			if (data.success) {
				lines += '\n<span style="color:#6a9955;font-weight:bold;">' + doneLabel + ' complete. Restarting...</span>\n';
			} else {
				lines += '\n<span style="color:#f44747;font-weight:bold;">' + doneLabel + ' failed.</span>\n';
			}
			output.innerHTML = lines;
			updateBtn.disabled = false;
			restartBtn.disabled = false;
			updateBtn.textContent = 'Update';
			restartBtn.textContent = 'Restart';
		}).catch(function(err) {
			output.innerHTML = '<span style="color:#f44747;">Error: ' + err.message + '</span>';
			updateBtn.disabled = false;
			restartBtn.disabled = false;
			updateBtn.textContent = 'Update';
			restartBtn.textContent = 'Restart';
		});
	}
	</script>`

	html := app.RenderHTMLForRequest("Admin", "Server", content, r)
	w.Write([]byte(html))
}

func handleDeploy(w http.ResponseWriter, r *http.Request) {
	deployMu.Lock()
	if deploying {
		deployMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"logs":    []deployLogEntry{{Step: "lock", Output: "Already in progress", Success: false}},
		})
		return
	}
	deploying = true
	deployMu.Unlock()

	defer func() {
		deployMu.Lock()
		deploying = false
		deployMu.Unlock()
	}()

	var req struct {
		Action string `json:"action"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	dir := sourceDir()
	var logs []deployLogEntry
	success := true

	type step struct {
		name string
		cmd  string
		args []string
	}

	var steps []step
	switch req.Action {
	case "restart":
		steps = []step{
			{"restart service", "sudo", []string{"-n", "systemctl", "restart", "mu"}},
		}
	default: // "update"
		steps = []step{
			{"git pull", "git", []string{"pull", "origin", "main"}},
			{"go install", "go", []string{"install"}},
			{"restart service", "sudo", []string{"-n", "systemctl", "restart", "mu"}},
		}
	}

	for _, s := range steps {
		entry := runStep(dir, s.name, s.cmd, s.args)
		logs = append(logs, entry)
		if !entry.Success {
			success = false
			break
		}
	}

	// Store logs
	deployMu.Lock()
	deployLogs = logs
	deployMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": success,
		"logs":    logs,
	})
}

func runStep(dir, name, cmdName string, args []string) deployLogEntry {
	cmd := exec.Command(cmdName, args...)
	cmd.Dir = dir

	// Inherit env and ensure Go/snap paths are available
	home := os.Getenv("HOME")
	path := os.Getenv("PATH")
	goPath := filepath.Join(home, "go", "bin")
	goRoot := "/usr/local/go/bin"
	if !strings.Contains(path, goPath) {
		path = goPath + ":" + path
	}
	if !strings.Contains(path, goRoot) {
		path = goRoot + ":" + path
	}
	cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+path)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	output := strings.TrimSpace(stdout.String())
	if errOut := strings.TrimSpace(stderr.String()); errOut != "" {
		if output != "" {
			output += "\n"
		}
		output += errOut
	}
	output += fmt.Sprintf("\n(%s)", duration.Round(time.Millisecond))

	return deployLogEntry{
		Time:    start,
		Step:    name,
		Output:  output,
		Success: err == nil,
	}
}
