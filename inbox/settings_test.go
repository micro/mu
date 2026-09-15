package inbox

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsRequireOwnerAndCSRF(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "inbox_settings_owner"
	if err := auth.Create(&auth.Account{ID: owner, Zone: "Europe/London"}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(owner) })
	send := func(method, body string, signed, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/inbox/settings", strings.NewReader(body))
		r.Header.Set("Accept", "application/json")
		r.Header.Set("Content-Type", "application/json")
		if signed {
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		SettingsHandler(w, r)
		return w
	}
	for _, tc := range []struct {
		method, body string
		signed, csrf bool
		code         int
	}{
		{"GET", "", false, false, 401},
		{"POST", `{"enabled":false,"include_world_news":false}`, true, false, 403},
		{"POST", `{"enabled":false}`, true, true, 400},
		{"POST", `{"enabled":"yes","include_world_news":true}`, true, true, 400},
		{"PUT", ``, true, true, 405},
		{"POST", `{"enabled":true,"include_world_news":false,"owner":"someone-else"}`, true, true, 200},
	} {
		w := send(tc.method, tc.body, tc.signed, tc.csrf)
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d %s", tc.method, tc.body, w.Code, w.Body.String())
		}
	}
	if events.Brief("someone-else") != nil {
		t.Fatal("forged owner was accepted")
	}
	w := send("GET", "", true, false)
	var prefs struct {
		Enabled bool   `json:"enabled"`
		News    bool   `json:"include_world_news"`
		Time    string `json:"time"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &prefs); err != nil {
		t.Fatal(err)
	}
	if !prefs.Enabled || prefs.News || prefs.Time != "06:00" {
		t.Fatalf("settings not saved: %+v", prefs)
	}
	w = send("POST", `{"enabled":false,"include_world_news":false}`, true, true)
	if w.Code != 200 || !events.Brief(owner).Paused {
		t.Fatal("disable did not pause existing event")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("preferences can be shared-cached")
	}
}
