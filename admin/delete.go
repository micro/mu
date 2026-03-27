package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
)

// DeleteHandler handles admin deletion of any registered content type.
// GET shows the delete form, POST performs the deletion.
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm()
		contentType := strings.TrimSpace(r.FormValue("type"))
		id := strings.TrimSpace(r.FormValue("id"))

		if contentType == "" || id == "" {
			http.Redirect(w, r, "/admin/delete?error=Type+and+ID+are+required", http.StatusSeeOther)
			return
		}

		if err := data.Delete(contentType, id); err != nil {
			http.Redirect(w, r, fmt.Sprintf("/admin/delete?error=%s", strings.ReplaceAll(err.Error(), " ", "+")), http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/admin/delete?success=Deleted+%s+%s", contentType, id), http.StatusSeeOther)
		return
	}

	types := data.DeleteTypes()

	var sb strings.Builder

	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-error">%s</p></div>`, errMsg))
	}
	if msg := r.URL.Query().Get("success"); msg != "" {
		sb.WriteString(fmt.Sprintf(`<div class="card"><p class="text-success">%s</p></div>`, msg))
	}

	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h3>Delete Content</h3>`)
	sb.WriteString(`<p class="text-sm text-muted">Delete any content by type and ID.</p>`)

	sb.WriteString(`<form method="POST" action="/admin/delete">`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label class="text-sm">Type</label>`)
	sb.WriteString(`<select name="type" class="form-input w-full mt-1">`)
	for _, t := range types {
		sb.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, t, t))
	}
	sb.WriteString(`</select>`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="mt-3">`)
	sb.WriteString(`<label class="text-sm">ID</label>`)
	sb.WriteString(`<input type="text" name="id" placeholder="Item ID" required class="form-input w-full mt-1">`)
	sb.WriteString(`</div>`)

	sb.WriteString(`<button type="submit" class="btn mt-3" onclick="return confirm('Delete this item?')">Delete</button>`)
	sb.WriteString(`</form>`)
	sb.WriteString(`</div>`)

	// Show registered types
	sb.WriteString(`<div class="card">`)
	sb.WriteString(`<h4>Registered Types</h4>`)
	sb.WriteString(`<p class="text-sm text-muted">`)
	for i, t := range types {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(t)
	}
	sb.WriteString(`</p>`)
	sb.WriteString(`</div>`)

	html := app.RenderHTMLForRequest("Delete Content", "Admin", sb.String(), r)
	w.Write([]byte(html))
}
