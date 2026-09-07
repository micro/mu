package contacts

import (
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/google"
)

func importHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		app.BadRequest(w, r, "Choose a CSV file up to 1 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	if err := auth.CheckPostRate(sess.Account); err != nil {
		app.RespondError(w, 429, "Please wait before importing again.")
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		app.BadRequest(w, r, "Choose a CSV file.")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		app.BadRequest(w, r, "Choose a CSV file up to 1 MB.")
		return
	}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 || len(rows) > 501 {
		app.BadRequest(w, r, "Use a CSV file with a header and up to 500 contacts.")
		return
	}
	value := func(row []string, names ...string) string {
		for i, h := range rows[0] {
			for _, name := range names {
				if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")), name) && i < len(row) {
					return strings.TrimSpace(row[i])
				}
			}
		}
		return ""
	}
	added, skipped, failed := 0, 0, 0
	for _, row := range rows[1:] {
		name := value(row, "Name", "Full Name", "Display Name")
		if name == "" {
			name = strings.TrimSpace(value(row, "First Name", "Given Name") + " " + value(row, "Last Name", "Family Name"))
		}
		email := value(row, "Email", "E-mail Address", "E-mail 1 - Value", "Email 1 - Value")
		phone := value(row, "Phone", "Mobile Phone", "Phone 1 - Value")
		if name == "" {
			name = email
		}
		if email != "" && HasEmail(sess.Account, email) {
			skipped++
			continue
		}
		if _, err := Add(sess.Account, name, email, phone, value(row, "Notes", "Note")); err != nil {
			failed++
		} else {
			added++
		}
	}
	body := fmt.Sprintf(`<div class="card"><p>Imported %d contacts. Skipped %d existing email addresses. %d rows could not be imported.</p><a class="btn" href="/contacts">Back to contacts</a></div>`, added, skipped, failed)
	app.Respond(w, r, app.Response{Title: "Contacts", HTML: body})
}

func googleCard(r *http.Request, owner, query string) string {
	if !HasExternal(owner) {
		return ""
	}
	var people []google.Person
	var next string
	var err error
	if query != "" {
		people, err = google.SearchContacts(owner, query, 30)
	} else {
		people, next, err = google.ContactPage(owner, r.PostFormValue("google_page"))
	}
	b := `<section class="card"><h3>Google Contacts</h3>`
	if err != nil {
		return b + `<p role="status">Could not load Google Contacts. Try again, or reconnect from Account.</p></section>`
	}
	if len(people) == 0 {
		b += `<p>No matching Google contacts.</p>`
	}
	for _, p := range people {
		b += `<div class="thin-row"><strong>` + html.EscapeString(p.Name) + `</strong><div class="text-sm text-muted">` + html.EscapeString(strings.TrimSpace(p.Email+" "+p.Phone)) + `</div></div>`
	}
	if next != "" {
		b += `<form method="POST" action="/contacts" class="mt-3">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="q" value=""><input type="hidden" name="google_page" value="` + html.EscapeString(next) + `"><button>More Google contacts</button> <a href="/contacts">First page</a></form>`
	}
	return b + `</section>`
}
