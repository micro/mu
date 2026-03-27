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
	case strings.HasPrefix(path, "/work/") && r.Method == "GET":
		handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = StatusOpen
	}

	tasks := ListTasks(status, 50)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]interface{}{
			"tasks": tasks,
		})
		return
	}

	sess, _ := auth.TrySession(r)

	var sb strings.Builder

	// Header
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Work</h3>`)
	sb.WriteString(`<p>Tasks with credit bounties. Claim a task, do the work, get paid.</p>`)
	if sess != nil {
		sb.WriteString(`<p><a href="/work/post" class="btn">Post a Task</a></p>`)
	} else {
		sb.WriteString(`<p><a href="/login">Login</a> to post or claim tasks.</p>`)
	}
	sb.WriteString(`</div>`)

	// Filter tabs
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<div class="d-flex gap-2">`)
	for _, s := range []struct{ val, label string }{
		{"open", "Open"},
		{"claimed", "In Progress"},
		{"delivered", "Delivered"},
		{"completed", "Completed"},
	} {
		cls := "btn btn-secondary"
		if s.val == status {
			cls = "btn"
		}
		sb.WriteString(fmt.Sprintf(`<a href="/work?status=%s" class="%s">%s</a>`, s.val, cls, s.label))
	}
	sb.WriteString(`</div>`)
	sb.WriteString(`</div>`)

	// Tasks
	if len(tasks) == 0 {
		sb.WriteString(`<div class="card"><p class="text-muted">No tasks found.</p></div>`)
	}

	for _, task := range tasks {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(fmt.Sprintf(`<h4><a href="/work/%s">%s</a></h4>`, task.ID, task.Title))

		// Tags
		if task.Tags != "" {
			sb.WriteString(`<p>`)
			for _, tag := range strings.Split(task.Tags, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					sb.WriteString(fmt.Sprintf(`<span class="tag">%s</span> `, tag))
				}
			}
			sb.WriteString(`</p>`)
		}

		sb.WriteString(fmt.Sprintf(`<p class="text-sm text-muted">%d credits · Posted by %s · %s</p>`,
			task.Bounty, task.Poster, task.CreatedAt.Format("2 Jan 2006")))

		sb.WriteString(`</div>`)
	}

	html := app.RenderHTMLForRequest("Work", "Tasks with credit bounties", sb.String(), r)
	w.Write([]byte(html))
}

func handleDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/work/")
	task := GetTask(id)
	if task == nil {
		http.NotFound(w, r)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, task)
		return
	}

	sess, _ := auth.TrySession(r)
	var userID string
	if sess != nil {
		userID = sess.Account
	}

	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(fmt.Sprintf(`<h3>%s</h3>`, task.Title))

	// Status badge
	statusLabel := task.Status
	sb.WriteString(fmt.Sprintf(`<p><strong>Status:</strong> %s</p>`, statusLabel))
	sb.WriteString(fmt.Sprintf(`<p><strong>Bounty:</strong> %d credits (%s)</p>`, task.Bounty, wallet.FormatCredits(task.Bounty)))
	sb.WriteString(fmt.Sprintf(`<p><strong>Posted by:</strong> <a href="/@%s">%s</a></p>`, task.Poster, task.Poster))
	sb.WriteString(fmt.Sprintf(`<p><strong>Posted:</strong> %s</p>`, task.CreatedAt.Format("2 Jan 2006 15:04")))

	if task.Tags != "" {
		sb.WriteString(`<p><strong>Tags:</strong> `)
		for _, tag := range strings.Split(task.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				sb.WriteString(fmt.Sprintf(`<span class="tag">%s</span> `, tag))
			}
		}
		sb.WriteString(`</p>`)
	}

	if task.Worker != "" {
		sb.WriteString(fmt.Sprintf(`<p><strong>Claimed by:</strong> <a href="/@%s">%s</a></p>`, task.Worker, task.Worker))
	}

	sb.WriteString(`</div>`)

	// Description
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h4>Description</h4>`)
	// Render description as paragraphs
	for _, para := range strings.Split(task.Description, "\n") {
		para = strings.TrimSpace(para)
		if para != "" {
			sb.WriteString(fmt.Sprintf(`<p>%s</p>`, para))
		}
	}
	sb.WriteString(`</div>`)

	// Delivery
	if task.Delivery != "" {
		sb.WriteString(`<div class="card">`)
		sb.WriteString(`<h4>Delivery</h4>`)
		sb.WriteString(fmt.Sprintf(`<p>%s</p>`, task.Delivery))
		sb.WriteString(`</div>`)
	}

	// Actions
	if sess != nil {
		sb.WriteString(`<div class="card">`)

		switch task.Status {
		case StatusOpen:
			if userID != task.PosterID {
				// Claim button
				sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/claim">`, task.ID))
				sb.WriteString(`<button type="submit" class="btn">Claim This Task</button>`)
				sb.WriteString(`</form>`)
			} else {
				// Cancel button for poster
				sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel" onsubmit="return confirm('Cancel this task? Your bounty will be refunded.')">`, task.ID))
				sb.WriteString(`<button type="submit" class="btn btn-secondary">Cancel Task</button>`)
				sb.WriteString(`</form>`)
			}

		case StatusClaimed:
			if userID == task.WorkerID {
				// Deliver form
				sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/deliver">`, task.ID))
				sb.WriteString(`<label for="delivery" class="text-sm">Deliverable (app slug, URL, or description)</label>`)
				sb.WriteString(`<input type="text" id="delivery" name="delivery" placeholder="e.g. /apps/my-app or a URL" required class="form-input w-full mt-1">`)
				sb.WriteString(`<button type="submit" class="btn mt-3">Submit Delivery</button>`)
				sb.WriteString(`</form>`)
			} else if userID == task.PosterID {
				sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/cancel" onsubmit="return confirm('Release this claim? The task will be reopened.')">`, task.ID))
				sb.WriteString(`<button type="submit" class="btn btn-secondary">Release Claim</button>`)
				sb.WriteString(`</form>`)
			}

		case StatusDelivered:
			if userID == task.PosterID {
				// Accept button
				sb.WriteString(fmt.Sprintf(`<form method="POST" action="/work/%s/accept">`, task.ID))
				sb.WriteString(`<button type="submit" class="btn">Accept &amp; Pay</button>`)
				sb.WriteString(`</form>`)
			}
		}

		sb.WriteString(`</div>`)
	}

	// Error/success messages
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-error">%s</p></div>`, errMsg))
	}
	if msg := r.URL.Query().Get("success"); msg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-success">%s</p></div>`, msg))
	}

	html := app.RenderHTMLForRequest(task.Title, "Work task", sb.String(), r)
	w.Write([]byte(html))
}

func handlePostForm(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	balance := wallet.GetBalance(sess.Account)
	errMsg := r.URL.Query().Get("error")

	var sb strings.Builder

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Post a Task</h3>`)
	if errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<p class="text-error">%s</p>`, errMsg))
	}
	sb.WriteString(fmt.Sprintf(`<p>Your balance: <strong>%d credits</strong></p>`, balance))
	sb.WriteString(`<p class="text-sm text-muted">The bounty will be held in escrow until you accept delivery or cancel.</p>`)

	sb.WriteString(`<form method="POST" action="/work/post">`)

	sb.WriteString(`<div>`)
	sb.WriteString(`<label for="title" class="text-sm">Title</label>`)
	sb.WriteString(`<input type="text" id="title" name="title" placeholder="e.g. Build a Pomodoro Timer App" required class="form-input w-full mt-1" maxlength="200">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="description" class="text-sm">Description</label>`)
	sb.WriteString(`<textarea id="description" name="description" rows="6" placeholder="What needs to be built? Be specific about requirements and deliverables." required class="form-input w-full mt-1"></textarea>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="bounty" class="text-sm">Bounty (credits)</label>`)
	sb.WriteString(`<input type="number" id="bounty" name="bounty" min="1" max="50000" placeholder="e.g. 500" required class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label for="tags" class="text-sm">Tags (comma-separated, optional)</label>`)
	sb.WriteString(`<input type="text" id="tags" name="tags" placeholder="e.g. app, design, developer" class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<button type="submit" class="btn mt-4">Post Task</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Post a Task", "Create a new task with a credit bounty", sb.String(), r)
	w.Write([]byte(html))
}

func handlePost(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		if app.SendsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"authentication required"}`))
			return
		}
		app.RedirectToLogin(w, r)
		return
	}

	var title, description, tags string
	var bounty int

	if app.SendsJSON(r) {
		var body struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Bounty      int    `json:"bounty"`
			Tags        string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			app.RespondJSON(w, map[string]string{"error": "invalid request body"})
			return
		}
		title = body.Title
		description = body.Description
		bounty = body.Bounty
		tags = body.Tags
	} else {
		r.ParseForm()
		title = r.FormValue("title")
		description = r.FormValue("description")
		tags = r.FormValue("tags")
		fmt.Sscanf(r.FormValue("bounty"), "%d", &bounty)
	}

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	tags = strings.TrimSpace(tags)

	if title == "" || description == "" || bounty < 1 {
		respondError(w, r, "/work/post", "Title, description, and bounty are required")
		return
	}

	// Hold bounty in escrow (skip for seed tasks from "mu" account)
	if sess.Account != "mu" {
		if err := wallet.HoldEscrow(sess.Account, bounty, "pending"); err != nil {
			respondError(w, r, "/work/post", "Insufficient credits for bounty")
			return
		}
	}

	task, err := CreateTask(sess.Account, acc.Name, title, description, tags, bounty)
	if err != nil {
		// Refund escrow if task creation fails
		if sess.Account != "mu" {
			wallet.RefundEscrow(sess.Account, bounty, "failed")
		}
		respondError(w, r, "/work/post", err.Error())
		return
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, task)
		return
	}

	http.Redirect(w, r, "/work/"+task.ID, http.StatusSeeOther)
}

func handleClaim(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		handleAuthError(w, r)
		return
	}

	id := extractTaskID(r.URL.Path, "/claim")
	if err := ClaimTask(id, sess.Account, acc.Name); err != nil {
		respondTaskError(w, r, id, err.Error())
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

	id := extractTaskID(r.URL.Path, "/deliver")

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
		respondTaskError(w, r, id, err.Error())
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

	id := extractTaskID(r.URL.Path, "/accept")

	task := GetTask(id)
	if task == nil {
		respondTaskError(w, r, id, "Task not found")
		return
	}

	if err := AcceptTask(id, sess.Account); err != nil {
		respondTaskError(w, r, id, err.Error())
		return
	}

	// Release escrow to worker
	if task.PosterID != "mu" {
		wallet.ReleaseEscrow(task.WorkerID, task.Bounty, task.ID)
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

	id := extractTaskID(r.URL.Path, "/cancel")

	task := GetTask(id)
	if task == nil {
		respondTaskError(w, r, id, "Task not found")
		return
	}

	// If task was claimed but not delivered, release back to open
	if task.Status == StatusClaimed {
		if err := ReleaseTask(id, sess.Account); err != nil {
			respondTaskError(w, r, id, err.Error())
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
		respondTaskError(w, r, id, err.Error())
		return
	}

	// Refund escrow to poster
	if task.PosterID != "mu" {
		wallet.RefundEscrow(task.PosterID, task.Bounty, task.ID)
	}

	if app.SendsJSON(r) || app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "cancelled"})
		return
	}

	http.Redirect(w, r, "/work?success=Task+cancelled", http.StatusSeeOther)
}

// extractTaskID extracts the task ID from a path like /work/{id}/action
func extractTaskID(path, suffix string) string {
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

func respondTaskError(w http.ResponseWriter, r *http.Request, id, msg string) {
	respondError(w, r, "/work/"+id, msg)
}
