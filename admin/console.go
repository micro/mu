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

// ConsoleHandler provides an admin console.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// POST: run command and return result
	if r.Method == "POST" {
		r.ParseForm()
		cmd := strings.TrimSpace(r.FormValue("cmd"))
		output := ""
		if cmd != "" {
			output = runCommand(cmd)
		}
		// If Accept: application/json, return JSON
		if app.WantsJSON(r) || r.Header.Get("Content-Type") == "application/json" {
			app.RespondJSON(w, map[string]string{"output": output})
			return
		}
		// Fallback: redirect
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

	sb.WriteString(`<div style="margin-top:8px;font-size:11px;color:#555">search · delete · user · wallet · types · stats · help</div>`)
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
    fetch('/admin/console',{method:'POST',body:'cmd='+encodeURIComponent(cmd),headers:{'Accept':'application/json','Content-Type':'application/x-www-form-urlencoded'}})
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
		return "search <query>  — search indexed content\ndelete <type> <id>  — delete by type and ID\nuser <id>  — view user details\nwallet <id>  — view wallet balance\ntypes  — list deletable content types\nstats  — index stats"

	default:
		return fmt.Sprintf("Unknown: %s. Type help for commands.", parts[0])
	}
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
