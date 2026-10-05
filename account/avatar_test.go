package account

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"mime/multipart"
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAvatarUploadAndRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := &auth.Account{ID: "avatar-owner", Name: "Amira", Approved: true}
	other := &auth.Account{ID: "avatar-other", Approved: true}
	auth.SetAccountForTest(owner)
	auth.SetAccountForTest(other)
	defer auth.RemoveAccountForTest(owner.ID)
	defer auth.RemoveAccountForTest(other.ID)
	sess, _ := auth.CreateSession(owner.ID)
	var original bytes.Buffer
	png.Encode(&original, image.NewRGBA(image.Rect(0, 0, 300, 200)))
	upload := func(raw []byte, csrf bool) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("photo", "picture.png")
		part.Write(raw)
		form.Close()
		r := httptest.NewRequest("POST", "/account/avatar", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		Account(w, r)
		return w
	}
	if w := upload(original.Bytes(), false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := upload([]byte(`<svg onload="alert(1)"></svg>`), true); w.Code != 400 {
		t.Fatal("non-raster accepted", w.Code)
	}
	if w := upload(original.Bytes(), true); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	updated, _ := auth.GetAccount(owner.ID)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(updated.Avatar, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "jpeg" || cfg.Width > 128 || cfg.Height > 128 {
		t.Fatal("photo not reduced to JPEG", cfg, format, err)
	}
	other, _ = auth.GetAccount(other.ID)
	if other.Avatar != "" {
		t.Fatal("changed another account")
	}
	r := httptest.NewRequest("POST", "/account/avatar", strings.NewReader(url.Values{"remove": {"1"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w := httptest.NewRecorder()
	Account(w, r)
	updated, _ = auth.GetAccount(owner.ID)
	if w.Code != 303 || updated.Avatar != "" {
		t.Fatal("photo not removed")
	}
}
