package images

import (
	"bytes"
	"fmt"
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
	caption := strings.TrimSpace(r.FormValue("caption"))
	if caption == "" {
		caption = h.Filename
	}
	if _, err := saveUpload(sess.Account, raw, caption); err != nil {
		app.BadRequest(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/images", http.StatusSeeOther)
}
func uploadForm(r *http.Request) string {
	return `<details class="card"><summary>Upload an image</summary><form class="form form-inline mt-3" method="POST" action="/images?upload=1" enctype="multipart/form-data">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="file" name="file" accept="image/png,image/jpeg,image/gif" required><input name="caption" placeholder="Caption" maxlength="1000"><button>Upload</button></form><p class="text-sm text-muted">Private. PNG, JPEG or GIF, up to 8 MB and 8 megapixels. ` + html.EscapeString("GIF uploads keep the first frame.") + `</p></details>`
}

// saveUpload normalizes accepted raster images and creates a private record.
func saveUpload(owner string, raw []byte, caption string) (*userdb.Record, error) {
	if owner == "" {
		return nil, userdb.ErrAuth
	}
	if len(raw) > 8<<20 {
		return nil, fmt.Errorf("image exceeds 8 MB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 8000000 {
		return nil, fmt.Errorf("Use a PNG, JPEG or GIF up to 8 megapixels.")
	}
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("Could not read this image.")
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, im) != nil {
		return nil, fmt.Errorf("Could not store this image.")
	}
	if len(caption) > 1000 {
		return nil, fmt.Errorf("Keep the caption under 1000 characters.")
	}
	rec, err := userdb.Create(ns, owner, collection, map[string]interface{}{"prompt": caption, "uploaded": true}, false)
	if err != nil {
		return nil, fmt.Errorf("could not save image")
	}
	key := genPrefix(rec.ID) + ".png"
	if err = blob.Put(key, encoded.Bytes(), "image/png"); err == nil {
		rec.Data["file"] = key
		_, err = userdb.Update(ns, owner, collection, rec.ID, rec.Data, false)
	}
	if err != nil {
		_ = blob.Delete(key)
		_ = userdb.Delete(ns, owner, collection, rec.ID)
		return nil, fmt.Errorf("could not save image")
	}
	return rec, nil
}
