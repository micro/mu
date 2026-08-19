package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/service"
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
	result := RenderHTML("Test", "A test page", "<p>content</p>", nil)
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

func TestRenderHTMLGuestNavHidesSignedInActions(t *testing.T) {
	result := RenderHTML("Test", "A test page", "<p>content</p>", nil)
	if strings.Contains(result, `id="nav-account"`) {
		t.Fatalf("guest nav should not render account link: %s", result)
	}
	if strings.Contains(result, `id="nav-logout"`) {
		t.Fatalf("guest nav should not render logout link: %s", result)
	}
	if !strings.Contains(result, `id="nav-login"`) {
		t.Fatalf("guest nav should render login link")
	}
}

func TestRenderHTMLWithAuthNavShowsSignedInActions(t *testing.T) {
	result := renderWithLang("Test", "A test page", "<p>content</p>", "en", &auth.Account{ID: "alice"})
	for _, want := range []string{`id="nav-account"`, `id="nav-logout"`, `Signed in as @alice`} {
		if !strings.Contains(result, want) {
			t.Fatalf("signed-in nav missing %q", want)
		}
	}
}

// The bottom group is the account itself: who you are, the page about you, and
// the way out.
//
// There is no Wallet item any more. Money is the account's, so the balance is
// the first card on /account and the badge in the header links to it.
//
// And no Usage item, which this test used to require. It moved from the top
// group to here on the argument that a view of money belongs beside the money —
// and the money is a card on /account, not a rail entry beside it. It is a card
// there now, under the balance, and /usage is still the page it links to. A
// sidebar entry per view of a page is how a sidebar becomes a site map.
func TestTheBottomGroupIsTheAccount(t *testing.T) {
	result := renderWithLang("Test", "d", "<p>c</p>", "en", &auth.Account{ID: "alice"})

	for _, want := range []string{`id="nav-account"`, `id="nav-logout"`} {
		if !strings.Contains(result, want) {
			t.Errorf("the sidebar is missing %s", want)
		}
	}
	if strings.Contains(result, `href="/usage"`) {
		t.Error("Usage is back in the sidebar. It is a view of what the account " +
			"spent, so it is a card on /account under the balance — the rail is " +
			"for places, not for views of one")
	}
	// Signing out is reached for directly; a logout behind another page is one
	// people hunt for.
	if !strings.Contains(result, `href="/logout"`) {
		t.Error("logout is not a direct link")
	}
}

// The spine of the sidebar is the product, and it is short on purpose. Every
// service lives in the catalogue; a nav that named four of twenty implied the
// other sixteen did not exist.
//
// Apps is not in it. It was, on the argument that it is half the product — but
// it is a service with a Spec and a tile in the catalogue like the other
// nineteen, so a permanent second entry above the fold was the spine claiming
// something the rest of the product does not agree with. Anyone who lives in
// Apps pins it, which is what pinning is for.
func TestTheSidebarIsTheProductsNouns(t *testing.T) {
	result := renderWithLang("Test", "d", "<p>c</p>", "en", &auth.Account{ID: "alice"})

	// The order somebody meets them in, and it has moved: what is yours first —
	// Inbox and the mailboxes under it, then Agents and the roster under it —
	// and the catalogue after, Tools then Services.
	//
	// Tools sat above Agents when tools were the lead, on the reasoning that
	// the product was named for them and Agents was what you built on top. The
	// thesis moved (DIRECTION §1 and §8) and so does this: the inbox and the
	// agents are the product, and the catalogue is what they reach for. Putting
	// the two personal lists together also keeps the rail readable as two
	// levels rather than as six equal destinations.
	want := []string{`href="/home"`, `href="/inbox"`, `href="/agents"`, `href="/tools"`, `href="/services"`}
	at := -1
	for _, w := range want {
		i := strings.Index(result, w)
		if i < 0 {
			t.Errorf("the sidebar is missing %s", w)
			continue
		}
		if i < at {
			t.Errorf("%s is out of order in the sidebar", w)
		}
		at = i
	}
	// Context was a fifth row: a page holding what an agent remembers and what
	// it is watching. It became a second home screen with a card picker on it,
	// and it is gone. Memory, the half that was real, is a card on /account.
	if strings.Contains(result, `href="/context"`) {
		t.Error("Context is back in the sidebar")
	}
	// A service reaches the sidebar by being pinned, never by being a service —
	// and apps is a service.
	for _, gone := range []string{`href="/apps"`, `href="/tasks"`, `href="/events"`, `href="/news"`} {
		if strings.Contains(result, gone) {
			t.Errorf("%s is in the sidebar of an account that pinned nothing", gone)
		}
	}
}

// A pinned service comes back into the sidebar. Demoting anything out of the
// spine — apps, most recently — is only defensible if the way back is the
// ordinary one that every other service already uses.
//
// Asserted with a service registered here rather than by naming apps: Pinned
// resolves through the registry, so naming a real service would make this pass
// or fail on whether that service's package happens to be linked into this test
// binary, which is not what is being tested.
func TestAPinnedServiceReturnsToTheSidebar(t *testing.T) {
	const name = "pinprobe"
	if _, known := service.SpecFor(name); !known {
		if err := service.Register(service.Spec{
			Name: name, Handler: new(PinProbe), Page: "/" + name,
			Endpoints: map[string]service.Endpoint{"List": {Doc: "probe"}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	acc := &auth.Account{ID: "alice"}
	acc.SetPinned([]string{name})
	result := renderWithLang("Test", "d", "<p>c</p>", "en", acc)
	if !strings.Contains(result, `href="/`+name+`"`) {
		t.Error("a pinned service did not appear in the sidebar")
	}
}

// PinProbe is the service the test above registers.
type PinProbe struct{}

func (PinProbe) List(ctx context.Context, req *struct{}, rsp *struct {
	Text string `json:"text"`
}) error {
	return nil
}

// Nothing pinned draws no group at all. An empty heading over an empty list is
// a worse answer than no heading, and it is what every new account would see.
func TestPinningNothingDrawsNoGroup(t *testing.T) {
	result := renderWithLang("Test", "d", "<p>c</p>", "en", &auth.Account{ID: "alice"})
	if strings.Contains(result, "nav-group") || strings.Contains(result, "nav-heading") {
		t.Error("an account that pinned nothing was given a Services group")
	}
}

func TestTheLanguageIsSetOnTheDocument(t *testing.T) {
	result := renderWithLang("Test", "desc", "<p>hello</p>", "ar", nil)
	if !strings.Contains(result, `lang="ar"`) {
		t.Error("expected Arabic language")
	}
}

func TestTheLanguageDefaultsToEnglish(t *testing.T) {
	result := renderWithLang("Test", "desc", "<p>hello</p>", "", nil)
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
