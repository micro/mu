package account

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAccountErrorsPreserveOnlyNonSecretFields(t *testing.T) {
	form := url.Values{"id": {`bad"><script>alert(1)</script>`}, "name": {"A name"}, "secret": {"never-reflect-this-password"}}
	r := httptest.NewRequest("POST", "/signup?redirect=%2Fassistant", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	page := accountFormValues(renderSignup("Please try again"), r)
	if strings.Contains(page, "never-reflect-this-password") || strings.Contains(page, `<script>alert(1)</script>`) {
		t.Fatal("unescaped field or password reflected")
	}
	if !strings.Contains(page, `value="A name"`) || !strings.Contains(page, `href="/login?redirect=%2Fassistant"`) {
		t.Fatal("form values or destination lost")
	}
	for _, want := range []string{"/composition.css?", `autocomplete="new-password"`, `class="field-label"`, ">Micro</a>"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestGoogleSignInKeepsOnlyASafeReturnDestination(t *testing.T) {
	withGoogle(t)
	for _, tc := range []struct{ to, want string }{{"/tasks", "/tasks"}, {"https://example.com", "/home"}, {"//example.com", "/home"}} {
		r := httptest.NewRequest("GET", "https://micro.mu/oauth2/google?redirect="+url.QueryEscape(tc.to), nil)
		w := httptest.NewRecorder()
		startGoogle(w, r, false)
		found := false
		for _, c := range w.Result().Cookies() {
			if c.Name == "g_return" {
				found = true
				got, _ := url.QueryUnescape(c.Value)
				if got != tc.want || !c.HttpOnly || !c.Secure {
					t.Fatalf("unsafe or lost return cookie: %#v", c)
				}
			}
		}
		if !found {
			t.Fatal("missing return destination")
		}
	}
}
