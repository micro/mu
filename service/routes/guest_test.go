package routes

import (
	"mu/internal/quota"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestUnmeteredGuestCanRequestDirections(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	old := quota.Enabled
	quota.Enabled = func() bool { return false }
	defer func() { quota.Enabled = old }()
	form := url.Values{"from": {"51.5,-0.1"}, "to": {"51.51,-0.11"}, "mode": {"walk"}}
	r := httptest.NewRequest("POST", "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 200 {
		t.Fatalf("guest directions: %d %s", w.Code, w.Body.String())
	}
}
