package forms

import (
	"errors"
	"fmt"
	"html"
	"mu/internal/userdb"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
)

type bucket struct {
	n     int
	until time.Time
}

var limits = struct {
	sync.Mutex
	entries map[string]bucket
}{entries: map[string]bucket{}}

func allow(key string, max int, window time.Duration) bool {
	limits.Lock()
	defer limits.Unlock()
	now := time.Now()
	for k, b := range limits.entries {
		if now.After(b.until) {
			delete(limits.entries, k)
		}
	}
	b, ok := limits.entries[key]
	if !ok {
		if len(limits.entries) >= 4096 {
			return false
		}
		b.until = now.Add(window)
	}
	if b.n >= max {
		return false
	}
	b.n++
	limits.entries[key] = b
	return true
}
func formBody(f *Form, values map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<p>%s</p><form class="form record-editor" method="POST" action="/forms/submit?id=%s">`, html.EscapeString(f.Description), html.EscapeString(f.ID))
	for _, field := range f.Fields {
		required := ""
		if field.Required {
			required = " required"
		}
		fmt.Fprintf(&b, `<label class="field-label"><span>%s`, html.EscapeString(field.Label))
		if field.Required {
			b.WriteString(` <span class="text-muted">(required)</span>`)
		}
		b.WriteString(`</span>`)
		if field.Type == "textarea" {
			fmt.Fprintf(&b, `<textarea name="%s" rows="5" maxlength="8000"%s>%s</textarea>`, field.Name, required, html.EscapeString(values[field.Name]))
		} else {
			fmt.Fprintf(&b, `<input type="%s" name="%s" maxlength="1000" value="%s"%s>`, field.Type, field.Name, html.EscapeString(values[field.Name]), required)
		}
		b.WriteString(`</label>`)
	}
	b.WriteString(`<p class="text-sm text-muted">Your response is sent privately to the form owner. No account is required.</p><div class="form-actions"><button type="submit">Submit</button></div></form>`)
	return b.String()
}
func PublicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	_, f, e := public(r.URL.Query().Get("id"))
	if e != nil {
		app.Error(w, r, 404, ErrUnavailable.Error())
		return
	}
	app.Respond(w, r, app.Response{Title: f.Title, HTML: formBody(f, nil)})
}

// SubmissionHandler is deliberately cookie-independent. Public forms can POST
// from any website; there is no credential to expose and no account to act as.
func SubmissionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	id := r.URL.Query().Get("id")
	if _, _, e := public(id); e != nil {
		app.Error(w, r, 404, ErrUnavailable.Error())
		return
	}
	if !allow("ip:"+app.ClientIP(r), 20, time.Minute) || !allow("form:"+id, 100, time.Hour) {
		w.Header().Set("Retry-After", "60")
		app.TooManyRequests(w, r, "Too many submissions. Please try again later.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	if e := r.ParseForm(); e != nil {
		app.BadRequest(w, r, "Could not read the response. Maximum size is 32 KB.")
		return
	}
	answers := map[string]string{}
	for k, v := range r.PostForm {
		if k == "_csrf" {
			continue
		}
		if len(v) != 1 {
			app.BadRequest(w, r, "Each field must have one value.")
			return
		}
		answers[k] = v[0]
	}
	_, e := Submit(id, answers)
	if e != nil {
		var invalid inputError
		if !errors.As(e, &invalid) && !errors.Is(e, ErrUnavailable) {
			message := "Could not save your response. Please try again later."
			status := http.StatusInternalServerError
			if errors.Is(e, userdb.ErrFull) {
				message = "This form has reached its response limit."
				status = http.StatusTooManyRequests
			}
			app.Error(w, r, status, message)
			return
		}
		if app.WantsJSON(r) {
			app.BadRequest(w, r, e.Error())
			return
		}
		_, f, readErr := public(id)
		if readErr != nil {
			app.Error(w, r, 404, ErrUnavailable.Error())
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		app.Respond(w, r, app.Response{Title: f.Title, HTML: app.Problem(html.EscapeString(e.Error())) + formBody(f, answers)})
		return
	}
	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]string{"status": "received"})
		return
	}
	// POST/redirect/GET prevents a refresh of the confirmation resubmitting.
	http.Redirect(w, r, "/forms/received", http.StatusSeeOther)
}
func ReceivedHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	app.Respond(w, r, app.Response{Title: "Response received", HTML: `<p>Thank you. Your response has been saved for the form owner.</p>`})
}
