package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		accept string
		want   bool
	}{
		{"application/json", true},
		{"text/html, application/json", true},
		{"text/html", false},
		{"", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Accept", tt.accept)
		got := WantsJSON(r)
		if got != tt.want {
			t.Errorf("WantsJSON(%q) = %v, want %v", tt.accept, got, tt.want)
		}
	}
}

func TestSendsJSON(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/html", false},
		{"", false},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Content-Type", tt.ct)
		got := SendsJSON(r)
		if got != tt.want {
			t.Errorf("SendsJSON(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	RespondError(w, 400, "bad request")
	if w.Code != 400 {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad request") {
		t.Error("expected error message in body")
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
}

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	RespondJSON(w, map[string]string{"status": "ok"})
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Error("expected JSON body")
	}
}

func TestError_JSONClient(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	Error(w, r, 404, "not found")
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not found") {
		t.Error("expected error message")
	}
}

func TestError_HTMLClient(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	Error(w, r, 404, "not found")
	if w.Code != 404 {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	Unauthorized(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	BadRequest(w, r, "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Bad request") {
		t.Error("expected default message")
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	NotFound(w, r, "custom not found")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "custom not found") {
		t.Error("expected custom message")
	}
}

func TestForbidden(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	Forbidden(w, r, "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Forbidden") {
		t.Error("expected default message")
	}
}

func TestServerError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	ServerError(w, r, "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	MethodNotAllowed(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestStripLatexDollars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Price in inline math", "$100$", "$100"},
		{"Price in display math", "$$94.63$$", "$94.63"},
		{"Escaped dollar", `\$50`, "$50"},
		{"Backslash parens", `\(x + y\)`, "x + y"},
		{"Backslash brackets", `\[x + y\]`, "x + y"},
		{"No latex", "plain text", "plain text"},
		{"HTML escaped backslash", "&#92;(x&#92;)", "x"},
		{"Price with suffix", "$100 billion$", "$100 billion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripLatexDollars(tt.input)
			if result != tt.expected {
				t.Errorf("StripLatexDollars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRender_Markdown(t *testing.T) {
	md := []byte("# Hello\n\nThis is **bold**.")
	result := string(Render(md))
	if !strings.Contains(result, "<h1") {
		t.Error("expected h1 tag")
	}
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Error("expected strong tag")
	}
}

func TestRenderString(t *testing.T) {
	result := RenderString("**bold**")
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Error("expected bold rendering")
	}
}

func TestRenderHTML(t *testing.T) {
	result := RenderHTML("Test", "A test page", "<p>content</p>")
	if !strings.Contains(result, "<title>Test | Mu</title>") {
		t.Error("expected title")
	}
	if !strings.Contains(result, `lang="en"`) {
		t.Error("expected English language")
	}
	if !strings.Contains(result, "<p>content</p>") {
		t.Error("expected body content")
	}
}

func TestRenderHTMLWithLang(t *testing.T) {
	result := RenderHTMLWithLang("Test", "desc", "<p>hello</p>", "ar")
	if !strings.Contains(result, `lang="ar"`) {
		t.Error("expected Arabic language")
	}
}

func TestRenderHTMLWithLang_DefaultsToEn(t *testing.T) {
	result := RenderHTMLWithLang("Test", "desc", "<p>hello</p>", "")
	if !strings.Contains(result, `lang="en"`) {
		t.Error("expected English when empty lang")
	}
}

func TestRenderTemplate(t *testing.T) {
	result := RenderTemplate("Test", "desc", "**bold**")
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Error("expected rendered markdown")
	}
	if !strings.Contains(result, "<title>Test | Mu</title>") {
		t.Error("expected page title")
	}
}

func TestLink(t *testing.T) {
	result := Link("Blog", "/blog")
	if !strings.Contains(result, `href="/blog"`) {
		t.Error("expected href")
	}
	if !strings.Contains(result, "Blog →") {
		t.Error("expected link text with arrow")
	}
}

func TestHead(t *testing.T) {
	result := Head("blog", []string{"tech", "news"})
	if !strings.Contains(result, `href="/blog"`) {
		t.Error("expected main link")
	}
	if !strings.Contains(result, "All") {
		t.Error("expected 'All' link")
	}
	if !strings.Contains(result, "#news") {
		t.Error("expected news anchor")
	}
	if !strings.Contains(result, "#tech") {
		t.Error("expected tech anchor")
	}
}

func TestHead_SkipsAll(t *testing.T) {
	result := Head("blog", []string{"All", "tech"})
	// Should have exactly one "All" (the main link), not a duplicate
	count := strings.Count(result, ">All<")
	if count != 1 {
		t.Errorf("expected 1 'All' link, got %d", count)
	}
}

func TestCard(t *testing.T) {
	result := Card("news", "News", "<p>Latest</p>")
	if !strings.Contains(result, `id="news"`) {
		t.Error("expected card id")
	}
	if !strings.Contains(result, "<h4>News</h4>") {
		t.Error("expected card title")
	}
	if !strings.Contains(result, "<p>Latest</p>") {
		t.Error("expected card content")
	}
}

func TestCardWithIcon(t *testing.T) {
	result := CardWithIcon("news", "News", "/news.png", "<p>Latest</p>")
	if !strings.Contains(result, `src="/news.png"`) {
		t.Error("expected icon image")
	}
}

func TestCardWithIcon_NoIcon(t *testing.T) {
	result := CardWithIcon("news", "News", "", "<p>Latest</p>")
	if strings.Contains(result, "<img") {
		t.Error("should not contain img when no icon")
	}
}

func TestServeHTML(t *testing.T) {
	handler := ServeHTML("<html>test</html>")
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(w, r)
	if w.Body.String() != "<html>test</html>" {
		t.Errorf("expected HTML content, got %q", w.Body.String())
	}
}

func TestSupportedLanguages(t *testing.T) {
	if _, ok := SupportedLanguages["en"]; !ok {
		t.Error("expected English in supported languages")
	}
	if _, ok := SupportedLanguages["ar"]; !ok {
		t.Error("expected Arabic in supported languages")
	}
	if _, ok := SupportedLanguages["zh"]; !ok {
		t.Error("expected Chinese in supported languages")
	}
}

func TestDecodeJSON_WrongContentType(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"foo":"bar"}`))
	r.Header.Set("Content-Type", "text/plain")
	var v map[string]string
	err := DecodeJSON(r, &v)
	if err == nil {
		t.Error("expected error for wrong content type")
	}
}

func TestDecodeJSON_ValidJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"foo":"bar"}`))
	r.Header.Set("Content-Type", "application/json")
	var v map[string]string
	err := DecodeJSON(r, &v)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if v["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", v)
	}
}

func TestRedirectToLogin(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/account?tab=settings", nil)
	RedirectToLogin(w, r)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/login?redirect=") {
		t.Errorf("expected login redirect, got %q", loc)
	}
	if !strings.Contains(loc, "account") {
		t.Error("expected original path in redirect")
	}
}
