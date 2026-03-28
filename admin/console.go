package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/wallet"
)

// ConsoleHandler provides an admin console for managing the system.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// Handle command
	cmd := ""
	output := ""
	if r.Method == "POST" {
		r.ParseForm()
		cmd = strings.TrimSpace(r.FormValue("cmd"))
		if cmd != "" {
			output = runCommand(cmd)
		}
		// Redirect with results to prevent form resubmission
		http.Redirect(w, r, "/admin/console?cmd="+url.QueryEscape(cmd)+"&output="+url.QueryEscape(output), http.StatusSeeOther)
		return
	}

	// GET — show form + results from redirect
	prevCmd := r.URL.Query().Get("cmd")
	prevOutput := r.URL.Query().Get("output")

	var sb strings.Builder

	// Form
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<form method="POST" action="/admin/console" style="display:flex;gap:8px">`)
	sb.WriteString(fmt.Sprintf(`<input type="text" name="cmd" value="%s" placeholder="Type a command..." class="form-input" style="flex:1" autocomplete="off" autofocus>`, htmlEsc(prevCmd)))
	sb.WriteString(`<button type="submit">Run</button>`)
	sb.WriteString(`</form>`)

	// Output
	if prevOutput != "" {
		sb.WriteString(fmt.Sprintf(`<pre style="margin-top:12px;font-size:13px;white-space:pre-wrap;background:#f9f9f9;padding:12px;border-radius:6px">%s</pre>`, htmlEsc(prevOutput)))
	}
	sb.WriteString(`</div>`)

	// Help
	sb.WriteString(`<div class="card">
<p class="text-sm text-muted">search &lt;query&gt; · delete &lt;type&gt; &lt;id&gt; · user &lt;id&gt; · wallet &lt;id&gt; · types · stats</p>
</div>`)

	html := app.RenderHTMLForRequest("Console", "Admin Console", sb.String(), r)
	w.Write([]byte(html))
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func runCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	switch parts[0] {
	case "search":
		if len(parts) < 2 {
			return "usage: search <query>"
		}
		query := strings.Join(parts[1:], " ")
		results := data.Search(query, 20)
		if len(results) == 0 {
			return "No results."
		}
		var sb strings.Builder
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("[%s] %s — %s\n", r.Type, r.ID, r.Title))
		}
		return sb.String()

	case "delete":
		if len(parts) < 3 {
			return "usage: delete <type> <id>"
		}
		contentType := parts[1]
		id := strings.Join(parts[2:], " ")
		if err := data.Delete(contentType, id); err != nil {
			return "Error: " + err.Error()
		}
		return fmt.Sprintf("Deleted %s %s", contentType, id)

	case "user":
		if len(parts) < 2 {
			return "usage: user <id>"
		}
		acc, err := auth.GetAccount(parts[1])
		if err != nil {
			return "User not found"
		}
		return fmt.Sprintf("ID: %s\nName: %s\nAdmin: %v\nCreated: %s",
			acc.ID, acc.Name, acc.Admin, acc.Created.Format("2 Jan 2006 15:04"))

	case "wallet":
		if len(parts) < 2 {
			return "usage: wallet <user_id>"
		}
		w := wallet.GetWallet(parts[1])
		usage := wallet.GetDailyUsage(parts[1])
		return fmt.Sprintf("Balance: %d credits\nDaily usage: %d / %d free",
			w.Balance, usage.Used, wallet.FreeDailyQuota)

	case "types":
		types := data.DeleteTypes()
		if len(types) == 0 {
			return "No deletable types registered."
		}
		return strings.Join(types, ", ")

	case "stats":
		stats := data.GetStats()
		return fmt.Sprintf("Index entries: %d\nSQLite: %v", stats.TotalEntries, stats.UsingSQLite)

	case "help":
		return "search <query> · delete <type> <id> · user <id> · wallet <id> · types · stats"

	default:
		return fmt.Sprintf("Unknown command: %s", parts[0])
	}
}
