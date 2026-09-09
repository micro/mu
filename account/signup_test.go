package account

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"mu/internal/auth"
)

func signupFields(t *testing.T, page string) url.Values {
	t.Helper()
	fields := url.Values{}
	for _, m := range regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)">`).FindAllStringSubmatch(page, -1) {
		fields.Set(m[1], m[2])
	}
	question := regexp.MustCompile(`What is (\d+) \+ (\d+)\?`).FindStringSubmatch(page)
	if len(question) != 3 {
		t.Fatal("signup has no solvable captcha")
	}
	a, _ := strconv.Atoi(question[1])
	b, _ := strconv.Atoi(question[2])
	fields.Set("captcha", strconv.Itoa(a+b))
	return fields
}

func TestSignupCreatesAccountAndSession(t *testing.T) {
	t.Setenv("INVITE_ONLY", "false")
	for i, target := range []string{"/signup", "/signup?redirect=%2Fbookmarks"} {
		page := httptest.NewRecorder()
		Signup(page, httptest.NewRequest("GET", target, nil))
		if page.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("signup may cache an expired captcha")
		}
		fields := signupFields(t, page.Body.String())
		id := fmt.Sprintf("signup_reader_%d", i)
		fields.Set("id", id)
		fields.Set("secret", "test-password-only")
		req := httptest.NewRequest("POST", target, strings.NewReader(fields.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		Signup(rec, req)
		want := "/home"
		if i == 1 {
			want = "/bookmarks"
		}
		if rec.Code != 302 || rec.Header().Get("Location") != want {
			t.Fatalf("signup failed: %d %s", rec.Code, rec.Body.String())
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != "session" || !cookies[0].HttpOnly || !cookies[0].Secure {
			t.Fatalf("missing secure login cookie: %v", cookies)
		}
		check := httptest.NewRequest("GET", "/home", nil)
		check.AddCookie(cookies[0])
		_, acc := auth.TrySession(check)
		if acc == nil || acc.ID != id {
			t.Fatal("signup did not create an authenticated account")
		}
	}
}

func TestSignupInviteIsEscapedAndRequestScoped(t *testing.T) {
	t.Setenv("INVITE_ONLY", "false")
	rec := httptest.NewRecorder()
	Signup(rec, httptest.NewRequest("GET", "/signup?invite="+url.QueryEscape(`"><script>alert(1)</script>`), nil))
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("invite injected markup")
	}
	rec = httptest.NewRecorder()
	Signup(rec, httptest.NewRequest("GET", "/signup", nil))
	if strings.Contains(rec.Body.String(), `name="invite"`) {
		t.Fatal("another visitor's invite leaked into signup")
	}
}
