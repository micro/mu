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

	// Inline post form for logged-in users
	if sess != nil {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(renderPostForm("show", ""))
		sb.WriteString(`</div>`)
	} else {
		sb.WriteString(`<div class="card"><p><a href="/login">Login</a> to post or interact.</p></div>`)
	}

	// Filter tabs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<div style="display:flex;gap:6px;flex-wrap:wrap">`)
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
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	if len(allPosts) == 0 {
		sb.WriteString(`<div class="card"><p class="text-muted">No posts yet.</p></div>`)
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

		meta := fmt.Sprintf(`%s · <a href="/@%s">%s</a> · %s`, kindLabel, post.Author, post.Author, post.CreatedAt.Format("2 Jan 2006"))
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

	// Post detail
	sb.WriteString(`<div class="card">`)

	kindLabel := "Show"
	if post.Kind == KindTask {
		kindLabel = "Task"
	}

	detailMeta := fmt.Sprintf(`%s · Posted by <a href="/@%s">%s</a> · %s`,
		kindLabel, post.Author, post.Author, post.CreatedAt.Format("2 Jan 2006 15:04"))
	detailMeta += app.ItemControls(userID, isAdmin, "work", post.ID, post.AuthorID, "", "/work/"+post.ID+"/delete")
	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%s</p>`, detailMeta))

	if post.Kind == KindTask {
		if post.Cost > 0 {
			sb.WriteString(fmt.Sprintf(`<p><strong>Budget:</strong> %d credits · <strong>Spent:</strong> %d credits</p>`, post.Cost, post.Spent))
		}
		if post.Status != "" {
			statusLabel := post.Status
			if post.Status == StatusClaimed {
				statusLabel = "building"
			}
			sb.WriteString(fmt.Sprintf(`<p><strong>Status:</strong> %s</p>`, statusLabel))
		}
	}
	if post.Tips > 0 {
		sb.WriteString(fmt.Sprintf(`<p><strong>Tips:</strong> %d credits</p>`, post.Tips))
	}
	if post.Link != "" {
		sb.WriteString(fmt.Sprintf(`<p><strong>Link:</strong> <a href="%s">%s</a></p>`, post.Link, post.Link))
	}
	if post.Worker == "agent" {
		sb.WriteString(`<p><strong>Assigned to:</strong> Agent</p>`)
	} else if post.Worker != "" {
		sb.WriteString(fmt.Sprintf(`<p><strong>Assigned to:</strong> <a href="/@%s">%s</a></p>`, post.Worker, post.Worker))
	}
	sb.WriteString(`</div>`)

	// Description
	sb.WriteString(`<div class="card">`)
	for _, para := range strings.Split(post.Description, "\n") {
		para = strings.TrimSpace(para)
		if para != "" {
			sb.WriteString(fmt.Sprintf(`<p>%s</p>`, para))
		}
	}
	sb.WriteString(`</div>`)

	// Delivery (tasks)
	if post.Delivery != "" {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h4>Delivery</h4>`)

		// Parse delivery format: "AppName — /apps/slug/run"
		if parts := strings.SplitN(post.Delivery, " — ", 2); len(parts) == 2 && strings.HasPrefix(parts[1], "/apps/") {
			appURL := parts[1]
			appName := parts[0]
			sb.WriteString(fmt.Sprintf(`<p><a href="%s">%s</a></p>`, appURL, appName))
			sb.WriteString(fmt.Sprintf(`<iframe src="%s?raw=1" style="width:100%%;min-height:400px;border:1px solid #eee;border-radius:8px;margin-top:8px" sandbox="allow-scripts"></iframe>`, appURL))
		} else {
			sb.WriteString(fmt.Sprintf(`<p>%s</p>`, post.Delivery))
		}
		sb.WriteString(`</div>`)
	}

	// Retry with feedback (for delivered tasks — author can request changes)
	if post.Status == StatusDelivered && userID == post.AuthorID {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/retry">`, post.ID))
		sb.WriteString(`<label class="text-sm">What needs to change?</label>`)
		sb.WriteString(`<textarea name="feedback" rows="3" placeholder="Describe what to fix or improve..." required class="form-input w-full mt-1"></textarea>`)
		sb.WriteString(`<div class="d-flex gap-2 mt-3">`)
		sb.WriteString(`<button type="submit" class="btn">Retry</button>`)
		sb.WriteString(`</form>`)
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/accept">`, post.ID))
		sb.WriteString(`<button type="submit" class="btn btn-secondary">Accept</button>`)
		sb.WriteString(`</form>`)
		sb.WriteString(`</div>`)
		sb.WriteString(`</div>`)
	}

	// Agent log
	if len(post.Log) > 0 {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h4>Agent Log</h4>`)
		for _, entry := range post.Log {
			color := "#555"
			switch entry.Step {
			case "error", "budget":
				color = "#c00"
			case "complete":
				color = "#28a745"
			}
			credits := ""
			if entry.Credits > 0 {
				credits = fmt.Sprintf(` · %d credits`, entry.Credits)
			}
			sb.WriteString(fmt.Sprintf(`<p style="font-size:13px;margin:4px 0"><span style="color:%s;font-weight:600">%s</span> %s%s <span class="text-muted">%s</span></p>`,
				color, entry.Step, entry.Message, credits, entry.CreatedAt.Format("15:04:05")))
		}
		sb.WriteString(`</div>`)
	}

	// Cancel (for open/building tasks)
	if sess != nil && post.Kind == KindTask && userID == post.AuthorID {
		if post.Status == StatusOpen || post.Status == StatusClaimed {
			sb.WriteString(`<div class="card">`)
			sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel" onsubmit="return confirm('Cancel this task?')">`, post.ID))
			sb.WriteString(`<button type="submit" class="btn btn-secondary">Cancel</button>`)
			sb.WriteString(`</form>`)
			sb.WriteString(`</div>`)
		}
	}

	// Feedback section
	sb.WriteString(`<div class="card">`)
	sb.WriteString(fmt.Sprintf(`<h4>Feedback (%d)</h4>`, len(post.Feedback)))

	if len(post.Feedback) > 0 {
		for _, fb := range post.Feedback {
			sb.WriteString(fmt.Sprintf(`<div class="mt-3"><p><strong><a href="/@%s">%s</a></strong> <span class="text-sm text-muted">%s</span></p>`,
				fb.Author, fb.Author, fb.CreatedAt.Format("2 Jan 15:04")))
			sb.WriteString(fmt.Sprintf(`<p>%s</p></div>`, fb.Text))
		}
	}

	// Feedback form
	if sess != nil {
		sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/feedback" class="mt-4">`, post.ID))
		sb.WriteString(`<textarea name="text" rows="3" placeholder="Share your thoughts..." required class="form-input w-full"></textarea>`)
		sb.WriteString(`<button type="submit" class="btn mt-2">Send Feedback</button>`)
		sb.WriteString(`</form>`)
	}
	sb.WriteString(`</div>`)

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

	// Validate budget
	if kind == KindTask && cost > 0 && sess.Account != "micro" {
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
			fmt.Sprintf(`Your delivery was accepted and %d credits have been released.

<a href="/work/%s">View task →</a>`, post.Cost, id))
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
	RetryWithFeedback(post, feedback)

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
func notifyWork(toID, subject, body string) {
	acc, err := auth.GetAccount(toID)
	if err != nil {
		return
	}
	mail.SendMessage("Mu", "micro", acc.Name, toID, subject, body, "", "")
}
