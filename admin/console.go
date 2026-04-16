package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"mu/apps"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/flag"
	"mu/user"
	"mu/wallet"
	"mu/work"
)

// InviteHandler serves the admin invite page at /admin/invite.
func InviteHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			app.BadRequest(w, r, "Email is required")
			return
		}
		code, err := auth.CreateInvite(email, sess.Account)
		if err != nil {
			app.ServerError(w, r, "Failed to create invite: "+err.Error())
			return
		}
		base := app.PublicURL()
		link := base + "/signup?invite=" + code

		// Try to email the invite. If mail isn't configured, show the link.
		if app.EmailSender != nil {
			plain := fmt.Sprintf("You've been invited to join Mu.\n\nSign up here: %s\n\nThis link is single-use.", link)
			html := fmt.Sprintf(`<p>You've been invited to join Mu.</p><p><a href="%s">Sign up here</a></p><p>This link is single-use.</p>`, link)
			if err := app.EmailSender(email, "You're invited to Mu", plain, html); err != nil {
				app.Log("admin", "Failed to email invite to %s: %v", email, err)
			}
		}

		content := fmt.Sprintf(`<div class="card">
<h4>Invite sent</h4>
<p>Invite created for <strong>%s</strong></p>
<p><a href="%s">%s</a></p>
<p class="text-muted text-sm">Link has been emailed (if mail is configured). Single use.</p>
<p><a href="/admin/invite">Invite another →</a> · <a href="/home">Home →</a></p>
</div>`, email, link, link)
		w.Write([]byte(app.RenderHTML("Invite Sent", "Invite sent", content)))
		return
	}

	content := `<div class="card">
<h4>Invite a user</h4>
<p class="text-sm">Enter their email address. They'll receive a single-use signup link.</p>
<form method="POST" action="/admin/invite" class="mt-4">
	<input type="email" name="email" placeholder="user@example.com" required class="form-input">
	<button type="submit" class="mt-2">Send invite</button>
</form>
</div>`
	w.Write([]byte(app.RenderHTML("Invite User", "Invite a user", content)))
}

// ConsoleHandler provides an admin console.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// POST: run command and return result
	if r.Method == "POST" {
		var cmd string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var req struct{ Cmd string `json:"cmd"` }
			json.NewDecoder(r.Body).Decode(&req)
			cmd = strings.TrimSpace(req.Cmd)
		} else {
			r.ParseForm()
			cmd = strings.TrimSpace(r.FormValue("cmd"))
		}
		output := ""
		if cmd != "" {
			output = runCommand(cmd)
		}
		if app.WantsJSON(r) || strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			app.RespondJSON(w, map[string]string{"output": output})
			return
		}
		http.Redirect(w, r, "/admin/console?cmd="+url.QueryEscape(cmd)+"&output="+url.QueryEscape(output), http.StatusSeeOther)
		return
	}

	// GET: render page
	prevCmd := r.URL.Query().Get("cmd")
	prevOutput := r.URL.Query().Get("output")

	var sb strings.Builder
	sb.WriteString(`<div class="card" style="background:#1a1a1a;color:#e0e0e0;font-family:'SF Mono','Fira Code',monospace;padding:16px;border:none">`)

	// Output area
	sb.WriteString(`<div id="cout" style="font-size:13px;white-space:pre-wrap;max-height:60vh;overflow-y:auto;margin-bottom:12px">`)
	if prevOutput != "" {
		sb.WriteString(fmt.Sprintf(`<span style="color:#888">&gt; %s</span>
%s`, esc(prevCmd), esc(prevOutput)))
	}
	sb.WriteString(`</div>`)

	// Input — form for fallback, JS for interactive
	sb.WriteString(`<form method="POST" action="/admin/console" id="cf" style="display:flex;gap:8px">`)
	sb.WriteString(`<span style="color:#888;line-height:32px">&gt;</span>`)
	sb.WriteString(`<input type="text" name="cmd" id="ci" autocomplete="off" autofocus style="flex:1;background:transparent;border:none;color:#e0e0e0;font-family:inherit;font-size:13px;outline:none;padding:6px 0">`)
	sb.WriteString(`<button type="submit" id="cb" style="background:#333;color:#e0e0e0;border:none;border-radius:4px;padding:4px 12px;font-family:inherit;font-size:12px;cursor:pointer">run</button>`)
	sb.WriteString(`</form>`)

	sb.WriteString(`<div style="margin-top:8px;font-size:11px;color:#555">help · users · apps · tasks · search · stats</div>`)
	sb.WriteString(`</div>`)

	// JS: intercept form, use fetch, append output inline
	sb.WriteString(`<script>
(function(){
  var form=document.getElementById('cf');
  var input=document.getElementById('ci');
  var out=document.getElementById('cout');
  var hist=[];
  var hi=-1;

  function run(){
    var cmd=input.value.trim();
    if(!cmd)return;
    hist.unshift(cmd);
    hi=-1;
    out.innerHTML+='<span style="color:#888">&gt; '+esc(cmd)+'</span>\n';
    input.value='';
    fetch('/admin/console',{method:'POST',body:JSON.stringify({cmd:cmd}),headers:{'Content-Type':'application/json'}})
    .then(function(r){return r.json()})
    .then(function(j){
      out.innerHTML+=esc(j.output)+'\n';
      out.scrollTop=out.scrollHeight;
    })
    .catch(function(e){
      out.innerHTML+='<span style="color:#c00">Error: '+esc(e.message)+'</span>\n';
    });
  }

  form.addEventListener('submit',function(e){
    e.preventDefault();
    run();
  });

  input.addEventListener('keydown',function(e){
    if(e.key==='ArrowUp'&&hist.length>0){
      hi=Math.min(hi+1,hist.length-1);
      input.value=hist[hi];
      e.preventDefault();
    }else if(e.key==='ArrowDown'){
      hi=Math.max(hi-1,-1);
      input.value=hi>=0?hist[hi]:'';
      e.preventDefault();
    }
  });

  function esc(s){
    var d=document.createElement('div');
    d.textContent=s;
    return d.innerHTML;
  }
})();
</script>`)

	html := app.RenderHTMLForRequest("Console", "Admin Console", sb.String(), r)
	w.Write([]byte(html))
}

func runCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	rest := func(i int) string {
		if i < len(parts) {
			return strings.Join(parts[i:], " ")
		}
		return ""
	}

	switch parts[0] {

	// --- Users ---
	case "users":
		accounts := auth.GetAllAccounts()
		sort.Slice(accounts, func(i, j int) bool { return accounts[i].Created.After(accounts[j].Created) })
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d users\n", len(accounts)))
		for _, a := range accounts {
			admin := ""
			if a.Admin {
				admin = " [admin]"
			}
			sb.WriteString(fmt.Sprintf("  %s (%s) — %s%s\n", a.ID, a.Name, a.Created.Format("2 Jan 2006"), admin))
		}
		return sb.String()

	case "user":
		if arg(1) == "" {
			return "usage: user <id>"
		}
		acc, err := auth.GetAccount(arg(1))
		if err != nil {
			return "User not found"
		}
		w := wallet.GetWallet(acc.ID)
		emailLine := "Email: (not set)"
		if acc.Email != "" {
			verified := "unverified"
			if acc.EmailVerified {
				verified = "verified"
			}
			emailLine = fmt.Sprintf("Email: %s (%s)", acc.Email, verified)
		}
		banLine := ""
		if acc.Banned {
			banLine = "\nBanned: YES"
		}
		return fmt.Sprintf("ID: %s\nName: %s\nAdmin: %v\nApproved: %v\n%s%s\nCreated: %s\nBalance: %d credits",
			acc.ID, acc.Name, acc.Admin, acc.Approved, emailLine, banLine, acc.Created.Format("2 Jan 2006 15:04"), w.Balance)

	case "approve":
		if arg(1) == "" {
			return "usage: approve <user_id>  (bypasses email verification)"
		}
		if err := auth.ApproveAccount(arg(1)); err != nil {
			return "approve failed: " + err.Error()
		}
		return fmt.Sprintf("Approved %s", arg(1))

	case "unapprove":
		if arg(1) == "" {
			return "usage: unapprove <user_id>"
		}
		acc, err := auth.GetAccount(arg(1))
		if err != nil {
			return "User not found"
		}
		acc.Approved = false
		if err := auth.UpdateAccount(acc); err != nil {
			return "unapprove failed: " + err.Error()
		}
		return fmt.Sprintf("Unapproved %s", arg(1))

	case "approve-old":
		// Bulk-approve accounts older than the given number of days.
		// Useful after enabling email verification to grandfather users
		// who joined before the change. Defaults to 7 days.
		days := 7
		if arg(1) != "" {
			fmt.Sscanf(arg(1), "%d", &days)
		}
		if days < 1 {
			return "days must be >= 1"
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		accounts := auth.GetAllAccounts()
		count := 0
		for _, a := range accounts {
			if a.Approved || a.Admin {
				continue
			}
			if a.Created.Before(cutoff) {
				a.Approved = true
				if err := auth.UpdateAccount(a); err == nil {
					count++
				}
			}
		}
		return fmt.Sprintf("Approved %d accounts older than %d days", count, days)

	case "ban":
		if arg(1) == "" {
			return "usage: ban <user_id>  (silently mutes — they don't know)"
		}
		if err := auth.BanAccount(arg(1)); err != nil {
			return "ban failed: " + err.Error()
		}
		return fmt.Sprintf("Banned %s — their content is now invisible to everyone else", arg(1))

	case "unban":
		if arg(1) == "" {
			return "usage: unban <user_id>"
		}
		if err := auth.UnbanAccount(arg(1)); err != nil {
			return "unban failed: " + err.Error()
		}
		return fmt.Sprintf("Unbanned %s", arg(1))

	case "clear-status":
		if arg(1) == "" {
			return "usage: clear-status <user_id|all>  (clears status + full history)"
		}
		if arg(1) == "all" {
			user.ClearAllStatuses()
			return "Cleared all status history for all users"
		}
		user.ClearStatusHistory(arg(1))
		return fmt.Sprintf("Cleared all status history for %s", arg(1))

	case "invite":
		if arg(1) == "" {
			return "usage: invite <email>"
		}
		email := arg(1)
		// Use "admin" as the admin ID for console-created invites
		code, err := auth.CreateInvite(email, "admin")
		if err != nil {
			return "invite failed: " + err.Error()
		}
		url := ""
		if v := os.Getenv("PUBLIC_URL"); v != "" {
			url = v
		} else if v := os.Getenv("MAIL_DOMAIN"); v != "" {
			url = "https://" + v
		}
		link := url + "/signup?invite=" + code
		return fmt.Sprintf("Invite created for %s\nCode: %s\nLink: %s", email, code, link)

	case "invites":
		list := auth.ListInvites()
		if len(list) == 0 {
			return "No invites"
		}
		var sb strings.Builder
		for _, inv := range list {
			used := "unused"
			if inv.UsedBy != "" {
				used = "used by " + inv.UsedBy
			}
			sb.WriteString(fmt.Sprintf("  %s → %s (%s, %s)\n", inv.Code[:8]+"...", inv.Email, used, inv.CreatedAt.Format("2 Jan 15:04")))
		}
		return sb.String()

	// --- Wallet ---
	case "wallet":
		if arg(1) == "" {
			return "usage: wallet <user_id>"
		}
		w := wallet.GetWallet(arg(1))
		txns := wallet.GetTransactions(arg(1), 10)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Balance: %d credits\n", w.Balance))
		if len(txns) > 0 {
			sb.WriteString("\nRecent transactions:\n")
			for _, tx := range txns {
				sb.WriteString(fmt.Sprintf("  %s  %+d  %s  bal:%d\n", tx.CreatedAt.Format("2 Jan 15:04"), tx.Amount, tx.Operation, tx.Balance))
			}
		}
		return sb.String()

	case "credit":
		if arg(1) == "" || arg(2) == "" {
			return "usage: credit <user_id> <amount>"
		}
		var amount int
		fmt.Sscanf(arg(2), "%d", &amount)
		if amount <= 0 {
			return "Amount must be positive"
		}
		wallet.AddCredits(arg(1), amount, "admin_grant", nil)
		return fmt.Sprintf("Added %d credits to %s", amount, arg(1))

	// --- Apps ---
	case "apps":
		allApps := apps.GetPublicApps()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d public apps\n", len(allApps)))
		for _, a := range allApps {
			sb.WriteString(fmt.Sprintf("  %s — %s (by %s, %d launches)\n", a.Slug, a.Name, a.Author, a.Installs))
		}
		return sb.String()

	case "app":
		if arg(1) == "" {
			return "usage: app <slug>"
		}
		a := apps.GetApp(arg(1))
		if a == nil {
			return "App not found"
		}
		return fmt.Sprintf("Slug: %s\nName: %s\nAuthor: %s (%s)\nPublic: %v\nInstalls: %d\nCreated: %s\nHTML: %d bytes",
			a.Slug, a.Name, a.Author, a.AuthorID, a.Public, a.Installs, a.CreatedAt.Format("2 Jan 2006"), len(a.HTML))

	// --- Work ---
	case "tasks":
		posts := work.ListPosts("task", "", 20)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d tasks\n", len(posts)))
		for _, p := range posts {
			sb.WriteString(fmt.Sprintf("  [%s] %s — %s (budget:%d spent:%d)\n", p.Status, p.ID[:8], p.Title, p.Cost, p.Spent))
		}
		return sb.String()

	case "task":
		if arg(1) == "" {
			return "usage: task <id>"
		}
		p := work.GetPost(arg(1))
		if p == nil {
			return "Task not found"
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("ID: %s\nTitle: %s\nStatus: %s\nAuthor: %s\nBudget: %d\nSpent: %d\nApp: %s\n",
			p.ID, p.Title, p.Status, p.AuthorID, p.Cost, p.Spent, p.AppSlug))
		if len(p.Log) > 0 {
			sb.WriteString(fmt.Sprintf("\nLog (%d entries):\n", len(p.Log)))
			for _, e := range p.Log {
				sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", e.CreatedAt.Format("15:04:05"), e.Step, e.Message))
			}
		}
		return sb.String()

	// --- Content ---
	case "search":
		if arg(1) == "" {
			return "usage: search <query>"
		}
		results := data.Search(rest(1), 20)
		if len(results) == 0 {
			return "No results."
		}
		var sb strings.Builder
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("[%s] %s — %s\n", r.Type, r.ID, r.Title))
		}
		return sb.String()

	case "delete":
		if arg(1) == "" || arg(2) == "" {
			return "usage: delete <type> <id>"
		}
		if err := data.Delete(arg(1), rest(2)); err != nil {
			return "Error: " + err.Error()
		}
		return fmt.Sprintf("Deleted %s %s", arg(1), rest(2))

	case "flags":
		flagged := flag.GetAll()
		if len(flagged) == 0 {
			return "No flagged content."
		}
		var sb strings.Builder
		for _, f := range flagged {
			sb.WriteString(fmt.Sprintf("[%s] %s — %d flags, hidden: %v\n", f.ContentType, f.ContentID, f.FlagCount, f.Flagged))
		}
		return sb.String()

	// --- System ---
	case "stats":
		stats := data.GetStats()
		accounts := auth.GetAllAccounts()
		allApps := apps.GetPublicApps()
		tasks := work.ListPosts("task", "", 100)
		return fmt.Sprintf("Users: %d\nApps: %d\nTasks: %d\nIndex: %d entries\nSQLite: %v",
			len(accounts), len(allApps), len(tasks), stats.TotalEntries, stats.UsingSQLite)

	case "types":
		return strings.Join(data.DeleteTypes(), ", ")

	case "help":
		return `Users:    users · user <id> · credit <id> <amount>
Wallet:   wallet <id>
Apps:     apps · app <slug>
Tasks:    tasks · task <id>
Content:  search <query> · delete <type> <id> · flags
System:   stats · types · help`

	default:
		return fmt.Sprintf("Unknown: %s. Type help.", parts[0])
	}
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
