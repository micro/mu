package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// GenerateDigestFunc is set by main to trigger digest generation (avoids import cycle)
var GenerateDigestFunc func() bool

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

	// GET — render server page with embedded status
	statusHTML := app.RenderInternalStatusHTML()

	content := `<p><a href="/admin">← Admin</a></p>
	<h2>Server</h2>
	` + statusHTML + storesTable() + `
	<h3>Deploy</h3>
	<p><strong>Source:</strong> <code>` + sourceDir() + `</code></p>
	<div id="deploy-controls">
		<button id="update-btn" onclick="runAction('update')">Update</button>
		<button id="restart-btn" onclick="runAction('restart')">Restart</button>
		<button id="digest-btn" onclick="runAction('digest')">Generate Digest</button>
		` + sweepButton() + `
	</div>
	<pre id="deploy-output" style="background:#1e1e1e;color:#d4d4d4;padding:16px;border-radius:6px;min-height:200px;max-height:500px;overflow-y:auto;font-size:13px;line-height:1.6;white-space:pre-wrap;display:none;"></pre>
	<script>
	// One table, so a fourth action is a row rather than another branch in three
	// places. It was written as three named variables and an if/else chain whose
	// last arm was the default, so any action but update or restart relabelled
	// the digest button — with an undefined label, for anything not in the map.
	var actions = {
		update:  {id: 'update-btn', idle: 'Update',           busy: 'Updating...',  done: 'Update'},
		restart: {id: 'restart-btn', idle: 'Restart',         busy: 'Restarting...', done: 'Restart'},
		digest:  {id: 'digest-btn', idle: 'Generate Digest',  busy: 'Generating...', done: 'Digest'},
		sweep:   {id: 'sweep-btn',  idle: 'Remove superseded stores', busy: 'Removing...', done: 'Removal'}
	};
	function actionButtons() {
		var out = [];
		for (var name in actions) {
			var el = document.getElementById(actions[name].id);
			if (el) out.push({el: el, spec: actions[name]});
		}
		return out;
	}
	function runAction(action) {
		var output = document.getElementById('deploy-output');
		var spec = actions[action];
		if (!spec) return;
		actionButtons().forEach(function(b) { b.el.disabled = true; });
		var self = document.getElementById(spec.id);
		if (self) self.textContent = spec.busy;
		output.style.display = 'block';
		output.textContent = '';

		fetch('/admin/server', {
			method: 'POST',
			headers: {'Content-Type': 'application/json'},
			body: JSON.stringify({action: action})
		}).then(function(res) { return res.json(); })
		.then(function(data) {
			var lines = '';
			if (data.logs) {
				data.logs.forEach(function(entry) {
					var color = entry.success ? '#6a9955' : '#f44747';
					var icon = entry.success ? '✓' : '✗';
					lines += '<span style="color:' + color + ';">' + icon + ' ' + entry.step + '</span>\n';
					if (entry.output) {
						lines += entry.output + '\n';
					}
				});
			}
			if (data.message) {
				lines += '<span style="color:#6a9955;">' + data.message + '</span>\n';
			}
			if (data.success) {
				lines += '\n<span style="color:#6a9955;font-weight:bold;">' + spec.done + ' complete.</span>\n';
			} else if (!data.message) {
				lines += '\n<span style="color:#f44747;font-weight:bold;">' + spec.done + ' failed.</span>\n';
			}
			output.innerHTML = lines;
			actionButtons().forEach(function(b) { b.el.disabled = false; b.el.textContent = b.spec.idle; });
		}).catch(function(err) {
			output.innerHTML = '<span style="color:#f44747;">Error: ' + err.message + '</span>';
			actionButtons().forEach(function(b) { b.el.disabled = false; b.el.textContent = b.spec.idle; });
		});
	}
	</script>`

	app.Respond(w, r, app.Response{Title: "Admin", Description: "Server", HTML: content})
}

// storesShown is how much of the data directory the table lists. The question
// it answers is "has anything got big", and the answer is at the top.
const storesShown = 12

// storesTable is what is on disk, largest first.
//
// Every store is a whole-file blob rewritten in one go, so a file that has
// quietly become the largest thing in the directory is a page or a background
// loop paying for its whole size on every write. Nothing said so anywhere, and
// the only way to find out was to go and look with ls on the box.
func storesTable() string {
	stores := data.Stores()
	if len(stores) == 0 {
		return ""
	}

	var total int64
	for _, s := range stores {
		total += s.Size
	}
	if len(stores) > storesShown {
		stores = stores[:storesShown]
	}

	// Which files the live backend does not write, so a large one can be read
	// as an archive rather than as a cost. Two index implementations leave two
	// files, and from outside a hot index and a dead one look the same.
	stale := map[string]bool{}
	for _, name := range data.Stale() {
		stale[name] = true
	}

	var b strings.Builder
	b.WriteString(`<h3>Stores</h3><table class="stats-table">`)
	for _, s := range stores {
		where := html.EscapeString(s.Name)
		if s.Files > 1 {
			where += fmt.Sprintf(` <span class="text-muted text-sm">%d files</span>`, s.Files)
		}
		if stale[strings.TrimSuffix(s.Name, "/")] {
			where += ` <span class="text-muted text-sm">not written</span>`
		}
		b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td></tr>`, where, app.Bytes(s.Size)))
	}
	b.WriteString(`</table>`)
	b.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">%s in the data directory. `+
		`Each of these is rewritten whole when it changes, except where marked. `+
		`Search index: %s.</p>`, app.Bytes(total), html.EscapeString(data.SearchBackend())))
	return b.String()
}

// sweepButton offers to remove the stores the live index has superseded, and
// only appears when there are any.
//
// A button that is usually a no-op teaches people to ignore it, and this one is
// a delete — it should be absent on the ordinary day and present on the one
// where a hundred megabytes of six-month-old file is sitting in every backup.
func sweepButton() string {
	names := data.Superseded()
	if len(names) == 0 {
		return ""
	}
	var freed int64
	for _, s := range data.Stores() {
		for _, name := range names {
			if s.Name == name {
				freed += s.Size
			}
		}
	}
	label := "Remove superseded stores"
	if freed > 0 {
		label += " (" + app.Bytes(freed) + ")"
	}
	return `<button id="sweep-btn" onclick="if(confirm('Delete ` +
		html.EscapeString(strings.Join(names, " and ")) +
		`? Everything in them is already in the live index.')){runAction('sweep')}">` +
		html.EscapeString(label) + `</button>`
}

func handleDeploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Removing what the migration superseded is not a deploy step: no build, no
	// restart, and it must never run as a side effect of one.
	if req.Action == "sweep" {
		removed, freed, err := data.RemoveSuperseded()
		message := "Nothing to remove — every store on disk is one the live index writes."
		switch {
		case err != nil:
			message = "Could not remove them: " + err.Error()
		case len(removed) > 0:
			message = fmt.Sprintf("Removed %s, freeing %s.",
				strings.Join(removed, " and "), app.Bytes(freed))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": err == nil,
			"message": message,
		})
		return
	}

	// Digest is handled separately — it runs in the background
	if req.Action == "digest" {
		if GenerateDigestFunc != nil {
			if !GenerateDigestFunc() {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"message": "Digest generation already in progress.",
				})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Digest generation started.",
		})
		return
	}

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

	dir := sourceDir()
	var logs []deployLogEntry
	success := true

	type step struct {
		name string
		cmd  string
		args []string
	}

	var steps []step
	restartSteps := []step{
		{"restart service", "sudo", []string{"systemctl", "restart", "mu"}},
	}

	switch req.Action {
	case "restart":
		steps = restartSteps
	default: // "update"
		steps = append([]step{
			{"git pull", "git", []string{"pull", "origin", "main"}},
			{"go install", "go", []string{"install", "."}},
		}, restartSteps...)
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
