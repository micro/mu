package admin

import (
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/service/mail"
)

// MailHandler is the role-protected view of the instance's operator mailbox.
// It never impersonates admin or creates a session for the system identity.
func MailHandler(w http.ResponseWriter, r *http.Request) {
	_, actor, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Administrator access required")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	messages, err := mail.OperatorMessages(actor.ID)
	if err != nil {
		app.Forbidden(w, r, err.Error())
		return
	}
	id := r.URL.Query().Get("id")
	var selected *mail.Message
	for i := range messages {
		if messages[i].ID == id {
			selected = &messages[i]
			break
		}
	}
	if id != "" && selected == nil {
		app.NotFound(w, r, "Message not found")
		return
	}
	if r.Method == http.MethodPost {
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form token")
			return
		}
		if selected == nil || r.FormValue("action") != "read" {
			app.BadRequest(w, r, "Select a message to mark read")
			return
		}
		if err := mail.OperatorMarkRead(actor.ID, id); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		app.Log("admin", "Operator mail %s marked read by %s", id, actor.ID)
		http.Redirect(w, r, "/admin/mail?id="+url.QueryEscape(id), http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	if selected != nil && r.URL.Query().Get("download") == "1" {
		raw, err := base64.StdEncoding.DecodeString(selected.Attachment)
		if err != nil || len(raw) == 0 {
			app.NotFound(w, r, "Attachment not found")
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": selected.AttachmentName}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(raw)
		return
	}
	auth.SetCSRFCookie(w, r)
	var b strings.Builder
	b.WriteString(`<div class="page-stack"><p class="text-muted">Shared mail for the instance’s administrators.</p><details class="disclosure"><summary>Addresses and access</summary><p>Sign in with your own administrator account. The built-in admin identity cannot sign in or be deleted.</p>`)
	b.WriteString(`<p class="text-muted text-sm">Mail to admin, support, postmaster, abuse and security reaches this inbox. Tags such as admin+dmarc categorise messages. Existing inbound-mail filters still apply. No automatic agent replies.</p></details>`)
	if selected != nil {
		b.WriteString(`<a href="/admin/mail">All operator mail</a><h2>` + html.EscapeString(selected.Subject) + `</h2><div class="metadata-row"><span>` + html.EscapeString(selected.From) + `</span><time>` + selected.CreatedAt.Format("2 Jan 2006 15:04 MST") + `</time><span>` + html.EscapeString(selected.Tag) + `</span></div>`)
		if !selected.Read {
			b.WriteString(`<form method="POST" class="form-actions">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="action" value="read"><button>Mark as read</button></form>`)
		}
		if selected.Attachment != "" {
			b.WriteString(`<a href="/admin/mail?id=` + url.QueryEscape(id) + `&amp;download=1">Download original attachment</a>`)
		}
		b.WriteString(`<div class="reader-content">` + mail.Rendered(selected) + `</div>`)
	} else {
		view := r.URL.Query().Get("view")
		b.WriteString(`<nav class="view-tabs" aria-label="Operator mail">`)
		for _, tab := range []struct{ value, label string }{{"", "Inbox"}, {"dmarc", "DMARC"}, {"filtered", "Filtered"}} {
			current := ""
			if view == tab.value {
				current = ` aria-current="page"`
			}
			b.WriteString(`<a href="/admin/mail?view=` + tab.value + `"` + current + `>` + tab.label + `</a>`)
		}
		b.WriteString(`</nav>`)
		var rows []string
		seen := map[string]bool{}
		for i := range messages {
			m := &messages[i]
			if (view == "filtered") != m.Spam {
				continue
			}
			label := m.Subject
			detail := m.From + " · " + m.CreatedAt.Format("2 Jan 2006 15:04")
			if view == "dmarc" {
				report := mail.MessageReport(m)
				if report == nil {
					continue
				}
				key := fmt.Sprintf("%s/%s/%s/%d/%d", report.ReportMetadata.OrgName, report.ReportMetadata.ReportID, report.PolicyPublished.Domain, report.ReportMetadata.DateRange.Begin, report.ReportMetadata.DateRange.End)
				if seen[key] {
					continue
				}
				seen[key] = true
				total, failed := 0, 0
				for _, record := range report.Records {
					total += record.Row.Count
					if record.Row.PolicyEvaluated.DKIM != "pass" && record.Row.PolicyEvaluated.SPF != "pass" {
						failed += record.Row.Count
					}
				}
				label = report.PolicyPublished.Domain + " · " + report.ReportMetadata.OrgName
				detail = fmt.Sprintf("%s–%s · %d messages · %d failed DMARC alignment", time.Unix(report.ReportMetadata.DateRange.Begin, 0).UTC().Format("2 Jan"), time.Unix(report.ReportMetadata.DateRange.End, 0).UTC().Format("2 Jan"), total, failed)
			}
			unread := ""
			if !m.Read {
				unread = " · Unread"
			}
			rows = append(rows, `<a class="collection-item" href="/admin/mail?id=`+url.QueryEscape(m.ID)+`"><span class="collection-title">`+html.EscapeString(label)+`</span><span class="collection-preview">`+html.EscapeString(detail+unread)+`</span></a>`)
		}
		page := app.Paginate(r, len(rows), 30)
		if len(rows) == 0 {
			b.WriteString(`<p class="text-muted">No messages here yet.</p>`)
			if view == "dmarc" && mail.Domain() != "" {
				b.WriteString(`<p>To collect DMARC reports here, set <code>rua=mailto:admin+dmarc@` + html.EscapeString(mail.Domain()) + `</code> in your domain’s DMARC record.</p>`)
			}
		}
		b.WriteString(`<div class="collection-list">` + strings.Join(rows[page.From:page.To], "") + `</div>` + page.Nav("/admin/mail?view="+url.QueryEscape(view)))
	}
	b.WriteString(`</div>`)
	app.Respond(w, r, app.Response{Title: "Operator mail", HTML: b.String()})
}
