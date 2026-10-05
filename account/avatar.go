package account

import (
	"encoding/base64"
	"io"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thumbnail"
	"net/http"
	"strings"
)

func avatarHandler(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	if r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(5 << 20); err != nil {
			app.BadRequest(w, r, "Choose a JPG, PNG or GIF up to 5 MB.")
			return
		}
		defer r.MultipartForm.RemoveAll()
	} else if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Invalid photo request")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reload and try again")
		return
	}
	if err := auth.CheckPostRate(acc.ID); err != nil {
		app.TooManyRequests(w, r, err.Error())
		return
	}
	photo := ""
	if r.FormValue("remove") != "1" {
		file, _, err := r.FormFile("photo")
		if err != nil {
			app.BadRequest(w, r, "Choose a photo")
			return
		}
		defer file.Close()
		raw, err := io.ReadAll(file)
		if err != nil {
			app.BadRequest(w, r, "Could not read photo")
			return
		}
		small, err := thumbnail.Render(raw, 128)
		if err != nil {
			app.BadRequest(w, r, "Choose a JPG, PNG or GIF, up to 16 megapixels.")
			return
		}
		photo = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(small)
	}
	copy := *acc
	copy.Avatar = photo
	if err := auth.UpdateAccount(&copy); err != nil {
		app.Error(w, r, 500, "Could not save photo")
		return
	}
	http.Redirect(w, r, "/account#photo", http.StatusSeeOther)
}

func avatarCard(r *http.Request, acc *auth.Account) string {
	body := app.Avatar(acc) + `<form class="form" method="post" action="/account/avatar" enctype="multipart/form-data">` + app.CSRFField(auth.CSRFToken(r)) + `<label class="field-label">Photo<input type="file" name="photo" accept="image/jpeg,image/png,image/gif" required></label><small class="text-muted">JPG, PNG or GIF, up to 5 MB. Leave unset to use your initial.</small><div class="form-actions"><button type="submit">Save photo</button></div></form>`
	if acc.Avatar != "" {
		body += `<form class="form-action" method="post" action="/account/avatar">` + app.CSRFField(auth.CSRFToken(r)) + `<button name="remove" value="1">Remove photo</button></form>`
	}
	return app.SectionID("photo", "Account photo", body)
}
