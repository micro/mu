package work

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
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
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/claim") && r.Method == "POST":
		handleClaim(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/deliver") && r.Method == "POST":
		handleDeliver(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/accept") && r.Method == "POST":
		handleAccept(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/cancel") && r.Method == "POST":
		handleCancel(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/tip") && r.Method == "POST":
		handleTip(w, r)
	case strings.HasPrefix(path, "/work/") && strings.HasSuffix(path, "/feedback") && r.Method == "POST":
		handleFeedback(w, r)
	case strings.HasPrefix(path, "/work/") && r.Method == "GET":
		handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleList(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	status := r.URL.Query().Get("status")

	// Default: show all, or filter by tab
	allPosts := ListPosts(kind, status, 50)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{"posts": allPosts})
		return
	}

	sess, _ := auth.TrySession(r)

	var sb strings.Builder

	// Header
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Work</h3>`)
	sb.WriteString(`<p>Share your work, get feedback, or post tasks with credit bounties.</p>`)
	if sess != nil {
		sb.WriteString(`<p><a href="/work/post" class="btn">Post</a></p>`)
	} else {
		sb.WriteString(`<p><a href="/login">Login</a> to post or interact.</p>`)
	}
	sb.WriteString(`</div>`)

	// Filter tabs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<div class="d-flex gap-2">`)
	for _, f := range []struct{ val, label string }{
		{"", "All"},
		{"show", "Show"},
		{"task", "Tasks"},
	} {
		cls := "btn btn-secondary"
		if f.val == kind {
			cls = "btn"
		}
		href := "/work"
		if f.val != "" {
			href += "?kind=" + f.val
		}
		sb.WriteString(fmt.Sprintf(`<a href="%s" class="%s">%s</a>`, href, cls, f.label))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Posts
	if len(allPosts) == 0 {
		sb.WriteString(`<div class="card"><p class="text-muted">No posts yet.</p></div>`)
	}

	for _, post := range allPosts {
		sb.WriteString(`<div class="card">`)

		// Kind label
		kindLabel := "Show"
		if post.Kind == KindTask {
			kindLabel = "Task"
			if post.Status != "" {
				kindLabel += " · " + post.Status
			}
		}

		sb.WriteString(fmt.Sprintf(`<h4><a href="/work/%s">%s</a></h4>`, post.ID, post.Title))

		// Tags
		if post.Tags != "" {
			for _, tag := range strings.Split(post.Tags, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					sb.WriteString(fmt.Sprintf(`<span class="tag">%s</span> `, tag))
				}
			}
		}

		// Meta line
		meta := fmt.Sprintf(`%s · <a href="/@%s">%s</a> · %s`, kindLabel, post.Author, post.Author, post.CreatedAt.Format("2 Jan 2006"))
		if post.Kind == KindTask && post.Bounty > 0 {
			meta += fmt.Sprintf(` · %d credits`, post.Bounty)
		}
		if post.Tips > 0 {
			meta += fmt.Sprintf(` · %d tipped`, post.Tips)
		}
		if len(post.Feedback) > 0 {
			meta += fmt.Sprintf(` · %d feedback`, len(post.Feedback))
		}
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

	sess, _ := auth.TrySession(r)
	var userID string
	if sess != nil {
		userID = sess.Account
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
	sb.WriteString(fmt.Sprintf(`<h3>%s</h3>`, post.Title))

	kindLabel := "Show"
	if post.Kind == KindTask {
		kindLabel = "Task"
	}

	sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%s · Posted by <a href="/@%s">%s</a> · %s</p>`,
		kindLabel, post.Author, post.Author, post.CreatedAt.Format("2 Jan 2006 15:04")))

	if post.Kind == KindTask {
		sb.WriteString(fmt.Sprintf(`<p><strong>Bounty:</strong> %d credits (%s)</p>`, post.Bounty, wallet.FormatCredits(post.Bounty)))
		if post.Status != "" {
			sb.WriteString(fmt.Sprintf(`<p><strong>Status:</strong> %s</p>`, post.Status))
		}
	}
	if post.Tips > 0 {
		sb.WriteString(fmt.Sprintf(`<p><strong>Tips:</strong> %d credits</p>`, post.Tips))
	}
	if post.Link != "" {
		sb.WriteString(fmt.Sprintf(`<p><strong>Link:</strong> <a href="%s">%s</a></p>`, post.Link, post.Link))
	}
	if post.Tags != "" {
		sb.WriteString(`<p>`)
		for _, tag := range strings.Split(post.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				sb.WriteString(fmt.Sprintf(`<span class="tag">%s</span> `, tag))
			}
		}
		sb.WriteString(`</p>`)
	}
	if post.Worker != "" {
		sb.WriteString(fmt.Sprintf(`<p><strong>Claimed by:</strong> <a href="/@%s">%s</a></p>`, post.Worker, post.Worker))
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
		sb.WriteString(fmt.Sprintf(`<p>%s</p>`, post.Delivery))
		sb.WriteString(`</div>`)
	}

	// Actions
	if sess != nil {
		actions := false

		if post.Kind == KindTask {
			switch post.Status {
			case StatusOpen:
				actions = true
				sb.WriteString(`<div class="card">`)
				if userID != post.AuthorID {
					sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/claim">`, post.ID))
					sb.WriteString(`<button type="submit" class="btn">Claim This Task</button>`)
					sb.WriteString(`</form>`)
				} else {
					sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel" onsubmit="return confirm('Cancel this task? Your bounty will be refunded.')">`, post.ID))
					sb.WriteString(`<button type="submit" class="btn btn-secondary">Cancel Task</button>`)
					sb.WriteString(`</form>`)
				}
				sb.WriteString(`</div>`)

			case StatusClaimed:
				actions = true
				sb.WriteString(`<div class="card">`)
				if userID == post.WorkerID {
					sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/deliver">`, post.ID))
					sb.WriteString(`<label for="delivery" class="text-sm">Deliverable (app slug, URL, or description)</label>`)
					sb.WriteString(`<input type="text" id="delivery" name="delivery" placeholder="e.g. /apps/my-app or a URL" required class="form-input w-full mt-1">`)
					sb.WriteString(`<button type="submit" class="btn mt-3">Submit Delivery</button>`)
					sb.WriteString(`</form>`)
				} else if userID == post.AuthorID {
					sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel">`, post.ID))
					sb.WriteString(`<button type="submit" class="btn btn-secondary">Release Claim</button>`)
					sb.WriteString(`</form>`)
				}
				sb.WriteString(`</div>`)

			case StatusDelivered:
				if userID == post.AuthorID {
					actions = true
					sb.WriteString(`<div class="card">`)
					sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/accept">`, post.ID))
					sb.WriteString(`<button type="submit" class="btn">Accept &amp; Pay</button>`)
					sb.WriteString(`</form>`)
					sb.WriteString(`</div>`)
				}
			}
		}

		// Tip (anyone can tip any post, not their own)
		if userID != post.AuthorID {
			if actions {
				// Add some spacing
			}
			sb.WriteString(`<div class="card">`)
			sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/tip" class="d-flex gap-2">`, post.ID))
			sb.WriteString(`<input type="number" name="amount" min="1" max="50000" placeholder="credits" required class="form-input" style="width:120px">`)
			sb.WriteString(`<button type="submit" class="btn btn-secondary">Tip</button>`)
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

func handlePostForm(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	errMsg := r.URL.Query().Get("error")
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "show"
	}

	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Share Work</h3>`)
	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}

	sb.WriteString(`<form method="POST" action="/work/post">`)

	// Kind selector
	sb.WriteString(`<div class="d-flex gap-2 mb-3">`)
	for _, k := range []struct{ val, label, desc string }{
		{"show", "Show", "Share something you built"},
		{"task", "Task", "Post a task with a bounty"},
	} {
		checked := ""
		if k.val == kind {
			checked = " checked"
		}
		sb.WriteString(fmt.Sprintf(`<label class="btn btn-secondary"><input type="radio" name="kind" value="%s"%s onchange="document.getElementById('bounty-field').style.display=this.value==='task'?'block':'none'"> %s</label>`, k.val, checked, k.label))
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div>`)
	sb.WriteString(`<label for="title" class="text-sm">Title</label>`)
	sb.WriteString(`<input type="text" id="title" name="title" placeholder="What did you build?" required class="form-input w-full mt-1" maxlength="200">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="description" class="text-sm">Description</label>`)
	sb.WriteString(`<textarea id="description" name="description" rows="6" placeholder="Tell people about it..." required class="form-input w-full mt-1"></textarea>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="link" class="text-sm">Link (optional)</label>`)
	sb.WriteString(`<input type="text" id="link" name="link" placeholder="URL, app slug, or repo" class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	bountyDisplay := "none"
	if kind == "task" {
		bountyDisplay = "block"
	}
	sb.WriteString(fmt.Sprintf(`<div class="mt-3" id="bounty-field" style="display:%s">`, bountyDisplay))
	sb.WriteString(`<label for="bounty" class="text-sm">Bounty (credits)</label>`)
	sb.WriteString(`<input type="number" id="bounty" name="bounty" min="1" max="50000" placeholder="e.g. 500" class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="tags" class="text-sm">Tags (optional)</label>`)
	sb.WriteString(`<input type="text" id="tags" name="tags" placeholder="e.g. app, go, design" class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<button type="submit" class="btn mt-4">Post</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Share Work", "Post your work or a task", sb.String(), r)
	w.Write([]byte(html))
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	var kind, title, description, link, tags string
	var bounty int

	if app.SendsJSON(r) {
		var body struct {
			Kind        string `json:"kind"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Link        string `json:"link"`
			Bounty      int    `json:"bounty"`
			Tags        string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			app.RespondJSON(w, map[string]string{"error": "invalid request body"})
			return
		}
		kind = body.Kind
		title = body.Title
		description = body.Description
		link = body.Link
		bounty = body.Bounty
		tags = body.Tags
	} else {
		r.ParseForm()
		kind = r.FormValue("kind")
		title = r.FormValue("title")
		description = r.FormValue("description")
		link = r.FormValue("link")
		tags = r.FormValue("tags")
		fmt.Sscanf(r.FormValue("bounty"), "%d", &bounty)
	}

	kind = strings.TrimSpace(kind)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	link = strings.TrimSpace(link)
	tags = strings.TrimSpace(tags)

	if kind == "" {
		kind = KindShow
	}

	// Hold bounty in escrow for tasks
	if kind == KindTask && bounty > 0 && sess.Account != "mu" {
		if err := wallet.HoldEscrow(sess.Account, bounty, "pending"); err != nil {
			respondError(w, r, "/work/post?kind=task", "Insufficient credits for bounty")
			return
		}
	}

	post, err := CreatePost(sess.Account, acc.Name, kind, title, description, link, tags, bounty)
	if err != nil {
		if kind == KindTask && bounty > 0 && sess.Account != "mu" {
			wallet.RefundEscrow(sess.Account, bounty, "failed")
		}
		respondError(w, r, "/work/post?kind="+kind, err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, post)
		return
	}

	http.Redirect(w, r, "/work/"+post.ID, http.StatusSeeOther)
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

func handleClaim(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/claim")
	if err := ClaimTask(id, sess.Account, acc.Name); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "claimed"})
		return
	}

	http.Redirect(w, r, "/work/"+id+"?success=Task+claimed", http.StatusSeeOther)
}

func handleDeliver(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractPostID(r.URL.Path, "/deliver")

	var delivery string
	if app.SendsJSON(r) {
		var body struct {
			Delivery string `json:"delivery"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		delivery = body.Delivery
	} else {
		r.ParseForm()
		delivery = r.FormValue("delivery")
	}
	delivery = strings.TrimSpace(delivery)

	if err := DeliverTask(id, sess.Account, delivery); err != nil {
		respondPostError(w, r, id, err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "delivered"})
		return
	}

	http.Redirect(w, r, "/work/"+id+"?success=Delivery+submitted", http.StatusSeeOther)
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

	// Release escrow to worker
	if post.AuthorID != "mu" {
		wallet.ReleaseEscrow(post.WorkerID, post.Bounty, post.ID)
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

	// Refund escrow to poster
	if post.AuthorID != "mu" {
		wallet.RefundEscrow(post.AuthorID, post.Bounty, post.ID)
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "cancelled"})
		return
	}

	http.Redirect(w, r, "/work?success=Task+cancelled", http.StatusSeeOther)
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
