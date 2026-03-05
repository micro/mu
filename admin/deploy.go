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

// DeployHandler shows the deploy page and handles deploy requests
func DeployHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		handleDeploy(w, r)
		return
	}

	// GET — render deploy page
	content := `<p><a href="/admin">← Admin</a></p>
	<h2>Deploy</h2>
	<p>Pull latest code, build, and restart the service.</p>
	<p><strong>Source:</strong> <code>` + sourceDir() + `</code></p>
	<div id="deploy-controls">
		<button id="deploy-btn" onclick="startDeploy()">Deploy</button>
	</div>
	<pre id="deploy-output" style="background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:6px;min-height:200px;max-height:500px;overflow-y:auto;font-size:13px;line-height:1.6;white-space:pre-wrap;display:none;"></pre>
	<script>
	function startDeploy() {
		var btn = document.getElementById('deploy-btn');
		var output = document.getElementById('deploy-output');
		btn.disabled = true;
		btn.textContent = 'Deploying...';
		output.style.display = 'block';
		output.textContent = '';

		fetch('/admin/deploy', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'}
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
			if (data.success) {
				lines += '\n<span style="color:#6a9955;font-weight:bold;">Deploy complete. Restarting...</span>\n';
			} else {
				lines += '\n<span style="color:#f44747;font-weight:bold;">Deploy failed.</span>\n';
			}
			output.innerHTML = lines;
			btn.disabled = false;
			btn.textContent = 'Deploy';
		}).catch(function(err) {
			output.innerHTML = '<span style="color:#f44747;">Error: ' + err.message + '</span>';
			btn.disabled = false;
			btn.textContent = 'Deploy';
		});
	}
	</script>`

	html := app.RenderHTMLForRequest("Admin", "Deploy", content, r)
	w.Write([]byte(html))
}

func handleDeploy(w http.ResponseWriter, r *http.Request) {
	deployMu.Lock()
	if deploying {
		deployMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"logs":    []deployLogEntry{{Step: "lock", Output: "Deploy already in progress", Success: false}},
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

	dir := sourceDir()
	var logs []deployLogEntry
	success := true

	steps := []struct {
		name string
		cmd  string
		args []string
	}{
		{"git pull", "git", []string{"pull", "origin", "main"}},
		{"go build", "go", []string{"build", "-o", "mu"}},
		{"install binary", "sudo", []string{"-n", "cp", "mu", "/usr/local/bin/mu"}},
		{"restart service", "sudo", []string{"-n", "systemctl", "restart", "mu"}},
	}

	for _, step := range steps {
		entry := runStep(dir, step.name, step.cmd, step.args)
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

	// Inherit env so go build picks up GOPATH etc
	cmd.Env = append(os.Environ(), "HOME="+os.Getenv("HOME"))

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
