package account

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProSignupDestination(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "test")
	t.Setenv("SUBSCRIPTION_CENTS", "")
	t.Setenv("SUBSCRIPTION_CREDITS", "")
	t.Setenv("INVITE_ONLY", "false")
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	want := "/account?plan=pro#subscription"
	var r = httptest.NewRequest("GET", "/signup?redirect="+url.QueryEscape(want), nil)
	w := httptest.NewRecorder()
	Signup(w, r)
	for _, entry := range []string{"Pro · $45/month", `action="/signup?redirect=`, `href="/login?redirect=`, `href="/oauth2/google?redirect=`} {
		if !strings.Contains(w.Body.String(), entry) {
			t.Fatalf("signup lost %s", entry)
		}
	}
}
