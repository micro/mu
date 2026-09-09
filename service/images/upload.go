package images

import (
	"bytes"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/blob"
	"mu/internal/userdb"
	"net/http"
	"strings"
)

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 9<<20)
	if err := r.ParseMultipartForm(9 << 20); err != nil {
		app.BadRequest(w, r, "Choose an image up to 8 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	if err := auth.CheckPostRate(sess.Account); err != nil {
		app.RespondError(w, 429, "Please wait before uploading again.")
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		app.BadRequest(w, r, "Choose an image.")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil || len(raw) > 8<<20 {
		app.BadRequest(w, r, "Choose an image up to 8 MB.")
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 8000000 {
		app.BadRequest(w, r, "Use a PNG, JPEG or GIF up to 8 megapixels.")
		return
	}
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		app.BadRequest(w, r, "Could not read this image.")
		return
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, im) != nil {
		app.BadRequest(w, r, "Could not store this image.")
		return
	}
	caption := strings.TrimSpace(r.FormValue("caption"))
	if caption == "" {
		caption = h.Filename
	}
	if len(caption) > 1000 {
		app.BadRequest(w, r, "Keep the caption under 1000 characters.")
		return
	}
	rec, err := userdb.Create(ns, sess.Account, collection, map[string]interface{}{"prompt": caption, "uploaded": true}, false)
	if err != nil {
		app.RespondError(w, 500, "Could not save image")
		return
	}
	key := genPrefix(rec.ID) + ".png"
	if err = blob.Put(key, encoded.Bytes(), "image/png"); err == nil {
		rec.Data["file"] = key
		_, err = userdb.Update(ns, sess.Account, collection, rec.ID, rec.Data, false)
	}
	if err != nil {
		_ = blob.Delete(key)
		_ = userdb.Delete(ns, sess.Account, collection, rec.ID)
		app.RespondError(w, 500, "Could not save image")
		return
	}
	http.Redirect(w, r, "/images", http.StatusSeeOther)
}
func uploadForm(r *http.Request) string {
	return `<details class="card"><summary>Upload an image</summary><form class="form form-inline mt-3" method="POST" action="/images?upload=1" enctype="multipart/form-data">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="file" name="file" accept="image/png,image/jpeg,image/gif" required><input name="caption" placeholder="Caption" maxlength="1000"><button>Upload</button></form><p class="text-sm text-muted">Private. PNG, JPEG or GIF, up to 8 MB and 8 megapixels. ` + html.EscapeString("GIF uploads keep the first frame.") + `</p></details>`
}
