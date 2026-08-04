package contacts

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
)

// session returns a signed-in cookie, creating the account on first use.
func session(t *testing.T, account string) *http.Cookie {
	t.Helper()
	auth.Create(&auth.Account{ID: account, Name: account, Secret: "test-secret"})
	sess, err := auth.CreateSession(account)
	if err != nil {
		t.Fatalf("could not sign in as %s: %v", account, err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

func csrfFor(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	probe := httptest.NewRequest("GET", "/contacts", nil)
	probe.AddCookie(cookie)
	token := auth.CSRFToken(probe)
	if token == "" {
		t.Fatal("no CSRF token for a signed-in request")
	}
	return token
}

func post(t *testing.T, cookie *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set("_csrf", csrfFor(t, cookie))

	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)

	rec := httptest.NewRecorder()
	Handler(rec, r)
	return rec
}

func page(t *testing.T, cookie *http.Cookie, path string) string {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	Handler(rec, r)
	return rec.Body.String()
}

// The point of the page: contacts stop being something only an agent can see.
func TestAddAndRemoveFromThePage(t *testing.T) {
	cookie := session(t, "owner")

	if rec := post(t, cookie, "/contacts", url.Values{
		"name": {"Sarah Chen"}, "email": {"sarah@example.com"}, "note": {"designer"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("add returned %d", rec.Code)
	}

	body := page(t, cookie, "/contacts")
	for _, want := range []string{"Sarah Chen", "sarah@example.com", "designer"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(body, want) {
				t.Errorf("the page does not show %q", want)
			}
		})
	}

	// The same functions the tools use, so the two cannot drift.
	people := List("owner")
	if len(people) != 1 || people[0].Name != "Sarah Chen" {
		t.Fatalf("the service does not hold what the page added: %+v", people)
	}

	if rec := post(t, cookie, "/contacts/"+people[0].ID+"/delete", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("remove returned %d", rec.Code)
	}
	if len(List("owner")) != 0 {
		t.Error("removing from the page did not remove the contact")
	}
}

// A name is the one thing a contact needs; without it there is nothing to look
// up, and the message belongs on the page rather than an error screen.
func TestAddWithoutANameComesBackWithAMessage(t *testing.T) {
	cookie := session(t, "owner2")

	rec := post(t, cookie, "/contacts", url.Values{"email": {"nobody@example.com"}})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
		t.Fatalf("got %d %q, want a redirect carrying an error", rec.Code, loc)
	}
	if len(List("owner2")) != 0 {
		t.Error("a nameless contact was stored")
	}
}

// An address book is per person. Nobody else's page, tools or session reaches
// it.
func TestOneAccountCannotSeeOrTouchAnothers(t *testing.T) {
	owner := session(t, "alice")
	post(t, owner, "/contacts", url.Values{"name": {"Private Person"}, "email": {"p@example.com"}})

	people := List("alice")
	if len(people) != 1 {
		t.Fatalf("setup failed: %+v", people)
	}

	if body := page(t, session(t, "mallory"), "/contacts"); strings.Contains(body, "Private Person") {
		t.Error("another account's page shows these contacts")
	}
	post(t, session(t, "mallory"), "/contacts/"+people[0].ID+"/delete", nil)
	if len(List("alice")) != 1 {
		t.Error("another account removed a contact")
	}
}

// A change to stored data has to be the account's own doing.
func TestActionsRejectAForgedRequest(t *testing.T) {
	cookie := session(t, "victim")
	post(t, cookie, "/contacts", url.Values{"name": {"Keep Me"}})

	r := httptest.NewRequest("POST", "/contacts",
		strings.NewReader("_csrf=wrong&name=Injected"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	if rec.Code == http.StatusSeeOther {
		t.Error("a request with a bad CSRF token was accepted")
	}
	if len(List("victim")) != 1 {
		t.Error("a forged request changed the address book")
	}
}

// Signed out, there is no address book to show.
func TestThePageRequiresASession(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("GET", "/contacts", nil))

	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Errorf("a signed-out visitor got %d, want a redirect to login", rec.Code)
	}
}

// Search is how a long book stays usable.
func TestSearchNarrowsTheList(t *testing.T) {
	cookie := session(t, "searcher")
	post(t, cookie, "/contacts", url.Values{"name": {"Sarah Chen"}, "email": {"sarah@example.com"}})
	post(t, cookie, "/contacts", url.Values{"name": {"Tom Baker"}, "email": {"tom@example.com"}})

	body := page(t, cookie, "/contacts?q=tom")
	if !strings.Contains(body, "Tom Baker") {
		t.Error("search did not find Tom")
	}
	if strings.Contains(body, "Sarah Chen") {
		t.Error("search returned everyone")
	}
}

// The phone layout is CSS, but it hangs off the markup: without a thead to hide
// and classed cells to restack, a narrow screen gets a five-column table.
func TestMarkupSupportsTheMobileLayout(t *testing.T) {
	cookie := session(t, "phoneuser")
	post(t, cookie, "/contacts", url.Values{"name": {"Someone"}})

	body := page(t, cookie, "/contacts")
	for _, want := range []string{
		`class="data-table contacts-table"`,
		"<thead>", "<tbody>",
		`class="contact-name"`, `class="contact-meta"`, `class="contact-actions"`,
		".contacts-table thead{display:none}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page is missing %q, which the phone layout needs", want)
		}
	}
}
