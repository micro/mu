package account

import (
	"mu/internal/auth"
	"mu/internal/codex"
	"mu/internal/dir"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexPreferenceIsAdminSelfService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	admin := &auth.Account{ID: "codex-admin", Admin: true, Created: time.Now()}
	other := &auth.Account{ID: "codex-other", Admin: false, Created: time.Now()}
	auth.SetAccountForTest(admin)
	auth.SetAccountForTest(other)
	defer auth.RemoveAccountForTest(admin.ID)
	defer auth.RemoveAccountForTest(other.ID)
	if strings.Contains(codexCard(other), "Codex") {
		t.Fatal("preview offered to non-admin")
	}
	request := func(id, state string, csrf bool) *http.Request {
		s, e := auth.CreateSession(id)
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest("POST", "/account/codex", strings.NewReader(url.Values{"state": {state}, "account_id": {other.ID}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: s.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	for _, tc := range []struct {
		id, state string
		csrf      bool
		want      int
	}{{other.ID, "on", true, 403}, {admin.ID, "on", false, 403}, {admin.ID, "on", true, 400}, {admin.ID, "invalid", true, 400}} {
		w := httptest.NewRecorder()
		CodexHandler(w, request(tc.id, tc.state, tc.csrf))
		if w.Code != tc.want {
			t.Fatalf("status %d want %d", w.Code, tc.want)
		}
	}
	// A fixture representing a successful host check; no model runs in this test.
	p := filepath.Join(dir.Root(), "codex-auth")
	os.MkdirAll(p, 0700)
	os.WriteFile(filepath.Join(p, "checked"), []byte(codex.Version), 0600)
	w := httptest.NewRecorder()
	CodexHandler(w, request(admin.ID, "on", true))
	if w.Code != 303 || !auth.CodexPreviewEnabled(admin.ID) || auth.CodexPreviewEnabled(other.ID) {
		t.Fatal("preference crossed account boundary")
	}
	w = httptest.NewRecorder()
	CodexHandler(w, request(admin.ID, "off", true))
	if w.Code != 303 || auth.CodexPreviewEnabled(admin.ID) {
		t.Fatal("could not revert provider")
	}
}
