package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// This was the tail of a file called console.go, which held an admin console —
// a text box that took `users`, `user <id>`, `apps`, `search`, `stats`,
// `credit <user> <amount>` and printed the answer.
//
// Every one of those is a page. The users list is /admin/users, apps are
// /apps, stats are /admin/traffic and /admin/usage, and the last one — the
// only command that did something rather than reported — became a button on
// /admin/users for a reason written down there: it "existed for somebody who
// already knew it was there". A second way to ask a question the sidebar
// already answers is not a power tool, it is a place for the two answers to
// disagree. On an instance with a shell, an admin who wants a REPL has one.

// pendingInvites is the count for the link on /admin/users, or nothing when
// there is nobody waiting. A badge that is always there stops being read.
func pendingInvites() string {
	n := 0
	for _, req := range auth.ListInviteRequests() {
		if !req.Invited {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(` <span class="count">%d</span>`, n)
}

// InviteHandler serves the admin invite page at /admin/invite.
func InviteHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		action := r.FormValue("action")
		email := strings.TrimSpace(r.FormValue("email"))
		if email == "" {
			app.BadRequest(w, r, "Email is required")
			return
		}
		if action == "reject" {
			auth.DeleteInviteRequest(email)
			http.Redirect(w, r, "/admin/invite", http.StatusSeeOther)
			return
		}
		code, err := auth.CreateInvite(email, sess.Account)
		if err != nil {
			app.ServerError(w, r, "Failed to create invite: "+err.Error())
			return
		}
		base := app.PublicURL()
		link := base + "/signup?invite=" + code

		emailSent := false
		if app.EmailSender != nil {
			plain := fmt.Sprintf("You've been invited to join Mu.\n\nSign up here: %s\n\nThis link is single-use.", link)
			html := fmt.Sprintf(`<p>You've been invited to join Mu.</p><p><a href="%s">Sign up here</a></p><p>This link is single-use.</p>`, link)
			if err := app.EmailSender(email, "You're invited to Mu", plain, html); err != nil {
				app.Log("admin", "Failed to email invite to %s: %v", email, err)
			} else {
				emailSent = true
			}
		}
		// Mark any pending request as fulfilled.
		auth.MarkInviteRequestSent(email)

		emailedMsg := `<p class="text-muted text-sm">Mail is not configured — copy the link above and send it manually.</p>`
		if emailSent {
			emailedMsg = `<p class="text-muted text-sm">Link has been emailed to them. Single use.</p>`
		}
		content := fmt.Sprintf(`<div class="card">
<h4>Invite sent</h4>
<p>Invite created for <strong>%s</strong></p>
<p><a href="%s">%s</a></p>
%s
<p><a href="/admin/invite">Back to invites →</a></p>
</div>`, email, link, link, emailedMsg)
		app.Respond(w, r, app.Response{Title: "Invite Sent", Description: "Invite sent", HTML: content})
		return
	}

	// GET: show pending requests + ad-hoc invite form.
	var sb strings.Builder
	sb.WriteString(`<p><a href="/admin">← Admin</a></p>`)

	requests := auth.ListInviteRequests()
	pending := 0
	for _, req := range requests {
		if !req.Invited {
			pending++
		}
	}

	sb.WriteString(fmt.Sprintf(`<div class="card"><h4>Invite requests (%d pending)</h4>`, pending))
	if len(requests) == 0 {
		sb.WriteString(`<p class="text-muted">No requests yet.</p>`)
	} else {
		for _, req := range requests {
			reason := ""
			if req.Reason != "" {
				reason = fmt.Sprintf(`<p class="text-sm text-secondary my-1">%s</p>`, req.Reason)
			}
			status := `<span class="text-warn text-xs">pending</span>`
			if req.Invited {
				status = fmt.Sprintf(`<span class="text-success text-xs">invited %s</span>`, req.InvitedAt.Format("2 Jan"))
			}
			actions := ""
			if !req.Invited {
				actions = fmt.Sprintf(
					`<div class="d-flex gap-1 mt-1"><form method="POST"><input type="hidden" name="email" value="%s"><button type="submit" class="mini-btn good">Send invite</button></form><form method="POST" onsubmit="return confirm('Reject?')"><input type="hidden" name="action" value="reject"><input type="hidden" name="email" value="%s"><button type="submit" class="mini-btn danger">Reject</button></form></div>`,
					req.Email, req.Email)
			} else {
				actions = fmt.Sprintf(
					`<div class="mt-1"><form method="POST" onsubmit="return confirm('Resend?')"><input type="hidden" name="email" value="%s"><button type="submit" class="text-xs p-tight">Resend</button></form></div>`,
					req.Email)
			}
			sb.WriteString(fmt.Sprintf(`<div class="thin-row row-pad"><div class="baseline-row flex-wrap"><strong class="text-base">%s</strong>%s<span class="text-faint text-xs">%s</span></div>%s%s</div>`,
				req.Email, status, req.RequestedAt.Format("2 Jan 15:04"), reason, actions))
		}
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="card mt-4">
<h4>Invite someone directly</h4>
<p class="text-sm">Enter an email — they'll get a single-use signup link.</p>
<form method="POST" action="/admin/invite" class="mt-4">
	<input type="email" name="email" placeholder="user@example.com" required class="form-input">
	<button type="submit" class="mt-2">Send invite</button>
</form>
</div>`)

	app.Respond(w, r, app.Response{Title: "Invites", Description: "Invite requests and send invites", HTML: sb.String()})
}
