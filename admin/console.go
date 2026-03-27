package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/wallet"
)

// ConsoleHandler provides an admin REPL for managing the system.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		handleConsoleCommand(w, r)
		return
	}

	prevOutput := r.URL.Query().Get("output")
	prevCmd := r.URL.Query().Get("cmd")

	var sb strings.Builder
	sb.WriteString(`<div class="card">
<form method="POST" action="/admin/console" id="console-form" style="display:flex;gap:8px">
<input type="text" name="cmd" id="console-input" placeholder="Type a command..." class="form-input" style="flex:1" autocomplete="off" autofocus>
<button type="submit">Run</button>
</form>`)
	if prevOutput != "" {
		sb.WriteString(fmt.Sprintf(`<pre style="margin-top:12px;font-size:13px;white-space:pre-wrap">&gt; %s
%s</pre>`, prevCmd, prevOutput))
	}
	sb.WriteString(`<pre id="console-output" style="margin-top:12px;font-size:13px;white-space:pre-wrap;max-height:500px;overflow-y:auto"></pre>
</div>

<div class="card">
<h4>Commands</h4>
<table style="font-size:13px;width:100%">
<tr><td style="padding:4px 8px"><code>search &lt;query&gt;</code></td><td style="padding:4px 8px;color:#888">Search indexed content</td></tr>
<tr><td style="padding:4px 8px"><code>delete &lt;type&gt; &lt;id&gt;</code></td><td style="padding:4px 8px;color:#888">Delete content by type and ID</td></tr>
<tr><td style="padding:4px 8px"><code>user &lt;id&gt;</code></td><td style="padding:4px 8px;color:#888">View user details</td></tr>
<tr><td style="padding:4px 8px"><code>wallet &lt;id&gt;</code></td><td style="padding:4px 8px;color:#888">View wallet balance</td></tr>
<tr><td style="padding:4px 8px"><code>types</code></td><td style="padding:4px 8px;color:#888">List deletable content types</td></tr>
<tr><td style="padding:4px 8px"><code>stats</code></td><td style="padding:4px 8px;color:#888">Index stats</td></tr>
</table>
</div>

<script>
var form = document.getElementById('console-form');
var input = document.getElementById('console-input');
var output = document.getElementById('console-output');
var history = [];
var histIdx = -1;

form.addEventListener('submit', function(e) {
  e.preventDefault();
  var cmd = input.value.trim();
  if (!cmd) return;
  history.unshift(cmd);
  histIdx = -1;
  output.textContent += '> ' + cmd + '\n';
  fetch('/admin/console', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({cmd: cmd})
  }).then(function(r) { return r.json(); }).then(function(j) {
    output.textContent += j.output + '\n';
    output.scrollTop = output.scrollHeight;
  }).catch(function(err) {
    output.textContent += 'Error: ' + err.message + '\n';
  });
  input.value = '';
});

input.addEventListener('keydown', function(e) {
  if (e.key === 'ArrowUp' && history.length > 0) {
    histIdx = Math.min(histIdx + 1, history.length - 1);
    input.value = history[histIdx];
    e.preventDefault();
  } else if (e.key === 'ArrowDown') {
    histIdx = Math.max(histIdx - 1, -1);
    input.value = histIdx >= 0 ? history[histIdx] : '';
    e.preventDefault();
  }
});
</script>`)

	html := app.RenderHTMLForRequest("Console", "Admin Console", sb.String(), r)
	w.Write([]byte(html))
}

func handleConsoleCommand(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		w.WriteHeader(403)
		return
	}

	var cmd string
	if r.Header.Get("Content-Type") == "application/json" {
		var req struct {
			Cmd string `json:"cmd"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		cmd = req.Cmd
	} else {
		r.ParseForm()
		cmd = r.FormValue("cmd")
	}
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		app.RespondJSON(w, map[string]string{"output": ""})
		return
	}

	var output string

	switch parts[0] {
	case "search":
		if len(parts) < 2 {
			output = "usage: search <query>"
			break
		}
		query := strings.Join(parts[1:], " ")
		results := data.Search(query, 20)
		if len(results) == 0 {
			output = "No results."
		} else {
			var sb strings.Builder
			for _, r := range results {
				sb.WriteString(fmt.Sprintf("[%s] %s — %s\n", r.Type, r.ID, r.Title))
			}
			output = sb.String()
		}

	case "delete":
		if len(parts) < 3 {
			output = "usage: delete <type> <id>"
			break
		}
		contentType := parts[1]
		id := strings.Join(parts[2:], " ")
		if err := data.Delete(contentType, id); err != nil {
			output = "Error: " + err.Error()
		} else {
			output = fmt.Sprintf("Deleted %s %s", contentType, id)
		}

	case "user":
		if len(parts) < 2 {
			output = "usage: user <id>"
			break
		}
		acc, err := auth.GetAccount(parts[1])
		if err != nil {
			output = "User not found"
		} else {
			output = fmt.Sprintf("ID: %s\nName: %s\nAdmin: %v\nCreated: %s",
				acc.ID, acc.Name, acc.Admin, acc.Created.Format("2 Jan 2006 15:04"))
		}

	case "wallet":
		if len(parts) < 2 {
			output = "usage: wallet <user_id>"
			break
		}
		w := wallet.GetWallet(parts[1])
		usage := wallet.GetDailyUsage(parts[1])
		output = fmt.Sprintf("Balance: %d credits\nDaily usage: %d / %d free",
			w.Balance, usage.Used, wallet.FreeDailyQuota)

	case "types":
		types := data.DeleteTypes()
		if len(types) == 0 {
			output = "No deletable types registered."
		} else {
			output = strings.Join(types, ", ")
		}

	case "stats":
		stats := data.GetStats()
		output = fmt.Sprintf("Index entries: %d\nSQLite: %v", stats.TotalEntries, stats.UsingSQLite)

	case "help":
		output = "Commands: search, delete, user, wallet, types, stats, help"

	default:
		output = fmt.Sprintf("Unknown command: %s. Type 'help' for commands.", parts[0])
	}

	if r.Header.Get("Content-Type") == "application/json" {
		app.RespondJSON(w, map[string]string{"output": output})
	} else {
		// Regular form submit — redirect back with output
		http.Redirect(w, r, "/admin/console?output="+strings.ReplaceAll(output, "\n", "%0A")+"&cmd="+cmd, http.StatusSeeOther)
	}
}
