package work

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/mail"
	"mu/wallet"
)

// Handler handles work-related HTTP requests
func Handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/work" && r.Method == "GET":
		handleList(w, r)
	case path == "/work/post" && r.Method == "GET":
		handlePostForm(w, r)
	case path == "/work/post" && r.Method == "POST":
		handlePost(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/accept") && r.Method == "POST":
		handleAccept(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/retry") && r.Method == "POST":
		handleRetry(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/cancel") && r.Method == "POST":
		handleCancel(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/tip") && r.Method == "POST":
		handleTip(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/feedback") && r.Method == "POST":
		handleFeedback(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/delete") && r.Method == "POST":
		handleDelete(w, r)
	case strings.HasPrefix(path, "/work/") && r.Method == "GET":
		handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleList(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	status := r.URL.Query().Get("status")

	sess, acc := auth.TrySession(r)
	var userID string
	var isAdmin bool
	if sess != nil {
		userID = sess.Account
		isAdmin = acc.Admin
	}

	// "mine" is a virtual filter — show posts by the current user
	actualKind := kind
	if kind == "mine" {
		actualKind = ""
	}
	allPosts := ListPosts(actualKind, status, 50)

	if kind == "mine" && userID != "" {
		var mine []*Post
		for _, p := range allPosts {
			if p.AuthorID == userID || p.WorkerID == userID {
				mine = append(mine, p)
			}
		}
		allPosts = mine
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"posts": allPosts})
		return
	}

	var sb strings.Builder

	// Filter tabs + new post link
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<div style="display:flex;gap:6px;flex-wrap:wrap;align-items:center">`)
	for _, f := range []struct{ val, label string }{
		{"", "All"},
		{"show", "Show"},
		{"task", "Tasks"},
		{"mine", "Mine"},
	} {
		style := "padding:4px 12px;border-radius:12px;font-size:13px;text-decoration:none;color:#555"
		if f.val == kind {
			style = "padding:4px 12px;border-radius:12px;font-size:13px;text-decoration:none;background:#000;color:#fff"
		}
		href := "/work"
		if f.val != "" {
			href += "?kind=" + f.val
		}
		sb.WriteString(fmt.Sprintf(`<a href="%s" style="%s">%s</a>`, href, style, f.label))
	}
	if sess != nil {
		sb.WriteString(`<a href="/work/post" class="btn" style="margin-left:auto">+ New</a>`)
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	if len(allPosts) == 0 {
		if sess != nil {
			sb.WriteString(`<div class="card"><p class="text-muted">No posts yet. <a href="/work/post">Create one →</a></p></div>`)
		} else {
			sb.WriteString(`<div class="card"><p class="text-muted">No posts yet. <a href="/login">Login</a> to create one.</p></div>`)
		}
	}

	for _, post := range allPosts {
		if userID != "" && (app.IsBlocked(userID, post.AuthorID) || app.IsDismissed(userID, "work", post.ID)) {
			continue
		}
		sb.WriteString(`<div class="card">`)

		kindLabel := "Show"
		if post.Kind == KindTask {
			kindLabel = "Task"
			switch post.Status {
			case StatusClaimed:
				kindLabel += " · building"
			case StatusDelivered:
				kindLabel += " · delivered"
			case StatusCompleted:
				kindLabel += " · completed"
			case StatusCancelled:
				kindLabel += " · cancelled"
			case StatusOpen:
				kindLabel += " · open"
			}
		}

		sb.WriteString(fmt.Sprintf(`<h4><a href="/work/%s">%s</a></h4>`, post.ID, post.Title))

		meta := fmt.Sprintf(`%s · <a href="/@%s">%s</a> · %s`, kindLabel, post.AuthorID, post.Author, post.CreatedAt.Format("2 Jan 2006"))
		if post.Kind == KindTask && post.Cost > 0 {
			if post.Spent > 0 {
				meta += fmt.Sprintf(` · %d/%d credits`, post.Spent, post.Cost)
			} else {
				meta += fmt.Sprintf(` · %d credits`, post.Cost)
			}
		}
		if len(post.Feedback) > 0 {
			meta += fmt.Sprintf(` · %d feedback`, len(post.Feedback))
		}
		meta += app.ItemControls(userID, isAdmin, "work", post.ID, post.AuthorID, "", "/work/"+post.ID+"/delete")
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%s</p>`, meta))

		sb.WriteString(`</div>`)
	}

	html := app.RenderHTMLForRequest("Work", "Share work, get feedback, post tasks", sb.String(), r)
	w.Write([]byte(html))
}

func handleDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/work/")
	post := GetPost(id)
	if post == nil {
		http.NotFound(w, r)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, post)
		return
	}

	sess, acc := auth.TrySession(r)
	var userID string
	var isAdmin bool
	if sess != nil {
		userID = sess.Account
		isAdmin = acc.Admin
	}

	var sb strings.Builder

	// Error/success messages
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-error">%s</p></div>`, errMsg))
	}
	if msg := r.URL.Query().Get("success"); msg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-success">%s</p></div>`, msg))
	}

	// === Single task card ===
	sb.WriteString(`<div class="card">`)

	// Meta line
	kindLabel := "Show"
	if post.Kind == KindTask {
		kindLabel = "Task"
	}
	detailMeta := fmt.Sprintf(`%s · <a href="/@%s">%s</a> · %s`,
		kindLabel, post.AuthorID, post.Author, post.CreatedAt.Format("2 Jan 2006 15:04"))
	detailMeta += app.ItemControls(userID, isAdmin, "work", post.ID, post.AuthorID, "", "/work/"+post.ID+"/delete")
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%s</p>`, detailMeta))

	// Description
	for _, para := range strings.Split(post.Description, "\n") {
		para = strings.TrimSpace(para)
		if para != "" {
			sb.WriteString(fmt.Sprintf(`<p>%s</p>`, para))
		}
	}

	// Task info
	if post.Kind == KindTask {
		statusLabel := post.Status
		if post.Status == StatusClaimed {
			statusLabel = "building"
		}
		info := fmt.Sprintf(`<strong>Status:</strong> %s`, statusLabel)
		if post.Cost > 0 {
			info += fmt.Sprintf(` · <strong>Budget:</strong> %d · <strong>Spent:</strong> %d`, post.Cost, post.Spent)
		}
		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted" style="margin-top:12px">%s</p>`, info))
	}
	if post.Link != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-sm"><a href="%s">%s</a></p>`, post.Link, post.Link))
	}

	// Cancel button (inline, not a separate card)
	if sess != nil && post.Kind == KindTask && userID == post.AuthorID {
		if post.Status == StatusOpen || post.Status == StatusClaimed {
			sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel" onsubmit="return confirm('Cancel this task?')" style="margin-top:12px">`, post.ID))
			sb.WriteString(`<button type="submit" class="btn btn-secondary">Cancel</button>`)
			sb.WriteString(`</form>`)
		}
	}

	sb.WriteString(`</div>`) // end task card

	// === App preview ===
	if post.AppSlug != "" {
		appURL := "/apps/" + post.AppSlug + "/run"
		sb.WriteString(fmt.Sprintf(`<div class="card">
			<p><a href="%s">Launch App →</a></p>
			<iframe src="%s?raw=1" style="width:100%%;min-height:400px;border:1px solid #eee;border-radius:8px;margin-top:8px" sandbox="allow-scripts"></iframe>
		</div>`, appURL, appURL))
	}

	// === Result (markdown delivery) ===
	if post.Delivery != "" {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(app.RenderString(post.Delivery))
		sb.WriteString(`</div>`)
	}

	// === Retry / Accept (delivered tasks) ===
	if post.Status == StatusDelivered && userID == post.AuthorID {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/retry">`, post.ID))
		sb.WriteString(`<textarea name="feedback" rows="2" placeholder="What needs to change?" required class="form-input w-full"></textarea>`)
		sb.WriteString(`<div class="d-flex gap-2 mt-3">`)
		sb.WriteString(`<button type="submit" class="btn">Retry</button>`)
		sb.WriteString(`</form>`)
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/accept">`, post.ID))
		sb.WriteString(`<button type="submit" class="btn btn-secondary">Accept</button>`)
		sb.WriteString(`</form>`)
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// === Agent log (collapsible) ===
	if len(post.Log) > 0 || (post.Status == StatusClaimed && post.WorkerID == "agent") {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<details>`)
		sb.WriteString(fmt.Sprintf(`<summary style="cursor:pointer;font-weight:600;font-size:14px">Agent Log (%d)</summary>`, len(post.Log)))
		sb.WriteString(`<div id="agent-log" style="margin-top:8px">`)
		for _, entry := range post.Log {
			color := "#555"
			switch entry.Step {
			case "error", "budget":
				color = "#c00"
			case "complete":
				color = "#1a7f37"
			case "info":
				color = "#888"
			}
			credits := ""
			if entry.Credits > 0 {
				credits = fmt.Sprintf(` · %dc`, entry.Credits)
			}
			sb.WriteString(fmt.Sprintf(`<p style="font-size:13px;margin:2px 0;color:#888"><span style="color:%s;font-weight:600">%s</span> %s%s</p>`,
				color, entry.Step, entry.Message, credits))
		}
		sb.WriteString(`</div>`)
		sb.WriteString(`</details>`)
		sb.WriteString(`</div>`)
	}

	// === Feedback ===
	if len(post.Feedback) > 0 || sess != nil {
		sb.WriteString(`<div class="card">`)
		if len(post.Feedback) > 0 {
			for _, fb := range post.Feedback {
				sb.WriteString(fmt.Sprintf(`<div style="margin-bottom:8px"><strong><a href="/@%s">%s</a></strong> <span class="text-sm text-muted">%s</span><p>%s</p></div>`,
					fb.Author, fb.Author, fb.CreatedAt.Format("2 Jan 15:04"), fb.Text))
			}
		}
		if sess != nil {
			sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/feedback">`, post.ID))
			sb.WriteString(`<textarea name="text" rows="2" placeholder="Add a comment..." required class="form-input w-full"></textarea>`)
			sb.WriteString(`<button type="submit" class="btn mt-2">Comment</button>`)
			sb.WriteString(`</form>`)
		}
		sb.WriteString(`</div>`)
	}

	// Live polling while building
	if post.Status == StatusClaimed && post.WorkerID == "agent" {
		sb.WriteString(fmt.Sprintf(`<script>
(function(){
  var logEl = document.getElementById('agent-log');
  var statusEl = document.querySelector('[data-status]');
  var lastCount = %d;
  function poll() {
    fetch('/work/%s', {headers:{'Accept':'application/json'}})
    .then(function(r){return r.json()})
    .then(function(p){
      if (p.log && p.log.length > lastCount) {
        for (var i = lastCount; i < p.log.length; i++) {
          var e = p.log[i];
          var color = e.step==='error'||e.step==='budget'?'#c00':e.step==='complete'?'#1a7f37':'#555';
          var credits = e.credits > 0 ? ' · '+e.credits+' credits' : '';
          var ts = e.created_at ? new Date(e.created_at).toLocaleTimeString() : '';
          if (logEl.children.length > 0) {
            var hr = document.createElement('hr');
            hr.style.cssText = 'border:none;border-top:1px solid #f0f0f0;margin:8px 0';
            logEl.appendChild(hr);
          }
          var el = document.createElement('div');
          el.style.cssText = 'font-size:13px;padding:4px 0';
          el.innerHTML = '<div><span style="color:'+color+';font-weight:600">'+e.step+'</span> <span class="text-muted">'+ts+'</span>'+credits+'</div><div style="margin-top:2px">'+e.message+'</div>';
          logEl.appendChild(el);
        }
        lastCount = p.log.length;
      }
      if (p.status !== 'claimed') {
        location.reload();
      } else {
        setTimeout(poll, 3000);
      }
    })
    .catch(function(){setTimeout(poll, 5000)});
  }
  setTimeout(poll, 3000);
})();
</script>`, len(post.Log), post.ID))
	}

	html := app.RenderHTMLForRequest(post.Title, "Work", sb.String(), r)
	w.Write([]byte(html))
}

// renderPostForm returns the HTML for the work post form.
// kind is the default kind ("show" or "task"), errMsg shows an error if set.
func renderPostForm(kind, errMsg string) string {
	var sb strings.Builder

	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}

	isTask := kind == "task"

	sb.WriteString(`<form method="POST" action="/work/post">`)

	// Kind selector
	sb.WriteString(`<div class="d-flex gap-2 mb-3">`)
	for _, k := range []struct{ val, label string }{
		{"show", "Show"},
		{"task", "Task"},
	} {
		checked := ""
		active := ""
		if k.val == kind {
			checked = " checked"
			active = "background:#000;color:#fff;"
		}
		sb.WriteString(fmt.Sprintf(`<label style="padding:6px 16px;border-radius:6px;font-size:13px;cursor:pointer;border:1px solid #ddd;%s"><input type="radio" name="kind" value="%s"%s style="display:none" onchange="workFormToggle(this.value)">%s</label>`, active, k.val, checked, k.label))
	}
	sb.WriteString(`</div>`)

	titlePlaceholder := "What did you build?"
	descPlaceholder := "Tell people about it..."
	if isTask {
		titlePlaceholder = "What needs to be done?"
		descPlaceholder = "Describe the task..."
	}

	sb.WriteString(`<input type="text" id="work-title" name="title" placeholder="` + titlePlaceholder + `" required class="form-input w-full" maxlength="200">`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<textarea id="work-desc" name="description" rows="3" placeholder="` + descPlaceholder + `" required class="form-input w-full"></textarea>`)
	sb.WriteString(`</div>`)

	// Show-only: link field
	linkDisplay := "block"
	if isTask {
		linkDisplay = "none"
	}
	sb.WriteString(fmt.Sprintf(`<div class="mt-3" id="link-field" style="display:%s">`, linkDisplay))
	sb.WriteString(`<input type="text" id="link" name="link" placeholder="Link (optional)" class="form-input w-full">`)
	sb.WriteString(`</div>`)

	// Task-only: budget
	costDisplay := "none"
	if isTask {
		costDisplay = "block"
	}
	sb.WriteString(fmt.Sprintf(`<div class="mt-3" id="cost-field" style="display:%s">`, costDisplay))
	sb.WriteString(`<input type="number" id="cost" name="cost" min="1" max="50000" placeholder="Budget (max credits)" class="form-input w-full">`)
	sb.WriteString(`</div>`)

	btnLabel := "Post"
	if isTask {
		btnLabel = "Start"
	}
	sb.WriteString(fmt.Sprintf(`<button type="submit" class="btn mt-3" id="work-submit">%s</button>`, btnLabel))
	sb.WriteString(`</form>`)

	// JS to toggle fields and placeholders
	sb.WriteString(`<script>
function workFormToggle(kind) {
  var isTask = kind === 'task';
  document.getElementById('cost-field').style.display = isTask ? 'block' : 'none';
  document.getElementById('link-field').style.display = isTask ? 'none' : 'block';
  document.getElementById('work-title').placeholder = isTask ? 'What needs to be done?' : 'What did you build?';
  document.getElementById('work-desc').placeholder = isTask ? 'Describe the task...' : 'Tell people about it...';
  document.getElementById('work-submit').textContent = isTask ? 'Start' : 'Post';
  var labels = document.querySelectorAll('input[name="kind"]');
  for (var i = 0; i < labels.length; i++) {
    var lbl = labels[i].parentElement;
    if (labels[i].value === kind) { lbl.style.background='#000';lbl.style.color='#fff'; }
    else { lbl.style.background='';lbl.style.color=''; }
  }
}
</script>`)

	return sb.String()
}

func handlePostForm(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "show"
	}
	errMsg := r.URL.Query().Get("error")

	content := `<div class="card">` + renderPostForm(kind, errMsg) + `</div>`
	html := app.RenderHTMLForRequest("Share Work", "Post your work or a task", content, r)
	w.Write([]byte(html))
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	var kind, title, description, link string
	var cost int

	if app.SendsJSON(r) {
		var body struct {
			Kind        string `json:"kind"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Link        string `json:"link"`
			Cost        int    `json:"cost"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			app.RespondJSON(w, map[string]string{"error": "invalid request body"})
			return
		}
		kind = body.Kind
		title = body.Title
		description = body.Description
		link = body.Link
		cost = body.Cost
	} else {
		r.ParseForm()
		kind = r.FormValue("kind")
		title = r.FormValue("title")
		description = r.FormValue("description")
		link = r.FormValue("link")
		fmt.Sscanf(r.FormValue("cost"), "%d", &cost)
	}

	kind = strings.TrimSpace(kind)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	link = strings.TrimSpace(link)

	if kind == "" {
		kind = KindShow
	}

	// Validate budget (skip for admin)
	if kind == KindTask && cost > 0 && !acc.Admin {
		wal := wallet.GetWallet(sess.Account)
		if wal.Balance < cost {
			respondError(w, r, "/work?kind=task", fmt.Sprintf("Insufficient credits (%d available, %d budget)", wal.Balance, cost))
			return
		}
	}

	post, err := CreatePost(sess.Account, acc.Name, kind, title, description, link, "", cost)
	if err != nil {
		respondError(w, r, "/work?kind="+kind, err.Error())
		return
	}

	// Auto-assign agent for tasks
	if kind == KindTask {
		AssignToAgent(post.ID, sess.Account)
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, post)
		return
	}

	successMsg := ""
	if kind == KindTask {
		successMsg = "?success=Task+started"
	}
	http.Redirect(w, r, "/work/"+post.ID+successMsg, http.StatusSeeOther)
}

func handleTip(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/tip")
	post := GetPost(id)
	if post == nil {
		respondPostError(w, r, id, "Post not found")
		return
	}

	if sess.Account == post.AuthorID {
		respondPostError(w, r, id, "Cannot tip your own work")
		return
	}

	var amount int
	if app.SendsJSON(r) {
		var body struct {
			Amount int `json:"amount"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		amount = body.Amount
	} else {
		r.ParseForm()
		fmt.Sscanf(r.FormValue("amount"), "%d", &amount)
	}

	if amount < 1 {
		respondPostError(w, r, id, "Tip must be at least 1 credit")
		return
	}
	if amount > 50000 {
		respondPostError(w, r, id, "Maximum tip is 50,000 credits")
		return
	}

	// Transfer credits from tipper to author
	if err := wallet.TransferCredits(sess.Account, post.AuthorID, amount); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	// Record the tip on the post
	TipPost(id, amount)

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"status": "tipped", "amount": amount})
		return
	}

	http.Redirect(w, r, "/work/"+id+"?success=Tipped+"+fmt.Sprintf("%d", amount)+"+credits", http.StatusSeeOther)
}

func handleFeedback(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/feedback")

	var text string
	if app.SendsJSON(r) {
		var body struct {
			Text string `json:"text"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		text = body.Text
	} else {
		r.ParseForm()
		text = r.FormValue("text")
	}
	text = strings.TrimSpace(text)

	if err := AddFeedback(id, sess.Account, acc.Name, text); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "ok"})
		return
	}

	http.Redirect(w, r, "/work/"+id, http.StatusSeeOther)
}

func handleAccept(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/accept")
	post := GetPost(id)
	if post == nil {
		respondPostError(w, r, id, "Post not found")
		return
	}

	if err := AcceptTask(id, sess.Account); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	// Credits already consumed during agent work — nothing to release

	// Notify the worker (if human)
	if post.WorkerID != "agent" && post.WorkerID != "" {
		notifyWork(post.WorkerID, "Task accepted: "+post.Title,
			fmt.Sprintf("Your delivery was accepted and %d credits have been released.\n\n[View task →](/work/%s)", post.Cost, id), id)
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "completed"})
		return
	}

	http.Redirect(w, r, "/work/"+id+"?success=Accepted+and+paid", http.StatusSeeOther)
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/cancel")
	post := GetPost(id)
	if post == nil {
		respondPostError(w, r, id, "Post not found")
		return
	}

	// If task was claimed but not delivered, release back to open
	if post.Status == StatusClaimed {
		if err := ReleaseTask(id, sess.Account); err != nil {
			respondPostError(w, r, id, err.Error())
			return
		}
		if app.SendsJSON(r) || app.WantsJSON(r) {
			app.RespondJSON(w, map[string]string{"status": "released"})
			return
		}
		http.Redirect(w, r, "/work/"+id+"?success=Claim+released", http.StatusSeeOther)
		return
	}

	if err := CancelTask(id, sess.Account); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	// Credits already consumed — no refund

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "cancelled"})
		return
	}

	http.Redirect(w, r, "/work?success=Task+cancelled", http.StatusSeeOther)
}

func handleRetry(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/retry")
	post := GetPost(id)
	if post == nil {
		respondPostError(w, r, id, "Post not found")
		return
	}
	if post.AuthorID != sess.Account {
		respondPostError(w, r, id, "Only the poster can retry")
		return
	}
	if post.Status != StatusDelivered {
		respondPostError(w, r, id, "Task is not in delivered state")
		return
	}

	r.ParseForm()
	feedback := strings.TrimSpace(r.FormValue("feedback"))
	if feedback == "" {
		respondPostError(w, r, id, "Feedback is required for retry")
		return
	}

	// Reset to claimed and re-run agent with the feedback
	RetryWithFeedback(id, feedback)

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "retrying"})
		return
	}

	http.Redirect(w, r, "/work/"+id+"?success=Retrying+with+feedback", http.StatusSeeOther)
}

func extractPostID(path, suffix string) string {
	path = strings.TrimPrefix(path, "/work/")
	path = strings.TrimSuffix(path, suffix)
	return path
}

func handleAuthError(w http.ResponseWriter, r *http.Request) {
	if app.SendsJSON(r) || app.WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required"}`))
		return
	}
	app.RedirectToLogin(w, r)
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/delete")
	post := GetPost(id)
	if post == nil {
		respondPostError(w, r, id, "Post not found")
		return
	}

	// Only admin or the author can delete
	if !acc.Admin && sess.Account != post.AuthorID {
		respondPostError(w, r, id, "You can only delete your own posts")
		return
	}

	if err := DeletePost(id); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "deleted"})
		return
	}

	http.Redirect(w, r, "/work?success=Post+deleted", http.StatusSeeOther)
}

func respondError(w http.ResponseWriter, r *http.Request, redirect, msg string) {
	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"error": msg})
		return
	}
	http.Redirect(w, r, redirect+"?error="+strings.ReplaceAll(msg, " ", "+"), http.StatusSeeOther)
}

func respondPostError(w http.ResponseWriter, r *http.Request, id, msg string) {
	respondError(w, r, "/work/"+id, msg)
}

// notifyWork sends an internal mail notification for work events.
// postID is used for threading — all notifications for the same task are grouped.
func notifyWork(toID, subject, body, postID string) {
	acc, err := auth.GetAccount(toID)
	if err != nil {
		return
	}
	mail.SendMessage("Mu", "micro", acc.Name, toID, subject, body, postID, "")
}
