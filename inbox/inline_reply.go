package inbox

import (
	"crypto/rand"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// inlineReply uses the existing send handler and keeps failed drafts in the reader.
func inlineReply(r *http.Request, owner string, t *thread.Thread, messages []thread.Message, draft ...form) string {
	if t.Client == thread.WebClient {
		return `<form class="form ib-compose" method="POST" action="/agent">` + app.CSRFField(auth.CSRFToken(r)) +
			`<input type="hidden" name="message_id" value="` + rand.Text() + `"><input type="hidden" name="inbox_reply" value="1"><input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `">` +
			`<label class="field-label">Message Micro<textarea name="ask" rows="2" maxlength="8000" required></textarea></label>` +
			`<div class="form-actions"><button type="submit">Send</button><span class="text-muted text-sm" role="status">The reply will appear in this conversation.</span></div></form>`
	}
	to := replyTo(owner, t, messages)
	if to == "" {
		return ""
	}
	subject := strings.TrimSpace(t.Subject)
	if subject == "" {
		subject = "your message"
	}
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	body, problem := "", ""
	if len(draft) > 0 {
		body = draft[0].Body
		problem = app.Problem(draft[0].Problem)
	}
	return `<section id="inbox-reply">` + problem +
		`<form class="form" method="POST" action="/inbox/new">` + app.CSRFField(auth.CSRFToken(r)) +
		`<input type="hidden" name="inline" value="1">` +
		`<input type="hidden" name="on" value="` + html.EscapeString(t.ID) + `">` +
		`<input type="hidden" name="to" value="` + html.EscapeString(to) + `">` +
		`<input type="hidden" name="subject" value="` + html.EscapeString(subject) + `">` +
		`<label class="field-label">Message<textarea name="body" rows="2" maxlength="40000" required>` + html.EscapeString(body) + `</textarea></label>` +
		`<div class="form-actions"><button type="submit">Send</button></div></form></section>`
}
