package files

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
)

// session returns a signed-in cookie for an account, creating it if this is the
// first test to ask for it.
func session(t *testing.T, account string) *http.Cookie {
	t.Helper()
	auth.Create(&auth.Account{ID: account, Name: account, Secret: "test-secret"})
	sess, err := auth.CreateSession(account)
	if err != nil {
		t.Fatalf("could not sign in as %s: %v", account, err)
	}
	return &http.Cookie{Name: "session", Value: sess.Token}
}

// csrfFor returns the token that belongs to a session cookie. It is derived
// from the session, so it must be minted from the same cookie the action will
// carry.
func csrfFor(t *testing.T, cookie *http.Cookie) string {
	t.Helper()
	probe := httptest.NewRequest("GET", "/files", nil)
	probe.AddCookie(cookie)
	token := auth.CSRFToken(probe)
	if token == "" {
		t.Fatal("no CSRF token for a signed-in request")
	}
	return token
}

// uploadRequest builds the multipart post a browser's file picker would send.
func uploadRequest(t *testing.T, cookie *http.Cookie, csrf, name, contents string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("_csrf", csrf)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte(contents))
	w.Close()

	r := httptest.NewRequest("POST", "/files", &buf)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.AddCookie(cookie)
	return r
}

// postAction submits one of the per-file forms.
func postAction(t *testing.T, cookie *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
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

// uploadAs stores a file through the page and returns the resulting record.
func uploadAs(t *testing.T, cookie *http.Cookie, account, name, contents string) *File {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(rec, uploadRequest(t, cookie, csrfFor(t, cookie), name, contents))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload returned %d, want a redirect", rec.Code)
	}
	if loc := rec.Header().Get("Location"); strings.Contains(loc, "error=") {
		t.Fatalf("upload failed: %s", loc)
	}
	for _, f := range List(account) {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s is not in the listing after upload", name)
	return nil
}

// The point of the change: a person can put a file in from the browser, not
// only an agent over MCP.
func TestUploadFromTheBrowserStoresTheFile(t *testing.T) {
	cookie := session(t, "uploader")
	f := uploadAs(t, cookie, "uploader", "notes.txt", "written from the browser")
	defer Delete("uploader", f.ID)

	_, raw, err := Get("uploader", f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "written from the browser" {
		t.Errorf("stored contents are %q", raw)
	}
	if f.Size != len("written from the browser") {
		t.Errorf("size is %d", f.Size)
	}
}

// Delete and share go through the same functions the tools call, so the page
// and the tools cannot drift apart.
func TestDeleteAndShareFromThePage(t *testing.T) {
	cookie := session(t, "sharer")
	f := uploadAs(t, cookie, "sharer", "public.txt", "hello")

	if rec := postAction(t, cookie, "/files/"+f.ID+"/share", url.Values{"public": {"1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("share returned %d", rec.Code)
	}
	if _, _, err := Get("bob", f.ID); err != nil {
		t.Errorf("sharing from the page did not make the file public: %v", err)
	}

	postAction(t, cookie, "/files/"+f.ID+"/share", url.Values{"public": {"0"}})
	if _, _, err := Get("bob", f.ID); err == nil {
		t.Error("making the file private again from the page had no effect")
	}

	postAction(t, cookie, "/files/"+f.ID+"/delete", nil)
	if _, _, err := Get("sharer", f.ID); err == nil {
		t.Error("deleting from the page did not remove the file")
	}
}

// A form post that changes stored data has to be the account's own doing, not
// another site's.
func TestActionsRejectAForgedRequest(t *testing.T) {
	cookie := session(t, "victim")
	f := uploadAs(t, cookie, "victim", "keep.txt", "must survive")
	defer Delete("victim", f.ID)

	r := httptest.NewRequest("POST", "/files/"+f.ID+"/delete",
		strings.NewReader("_csrf=not-the-right-token"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)

	rec := httptest.NewRecorder()
	Handler(rec, r)

	if rec.Code == http.StatusSeeOther {
		t.Error("a request with a bad CSRF token was accepted")
	}
	if _, _, err := Get("victim", f.ID); err != nil {
		t.Errorf("the file was deleted by a forged request: %v", err)
	}
}

// Storage is per account. A signed-out visitor has none.
func TestActionsRequireASession(t *testing.T) {
	cookie := session(t, "owner")
	f := uploadAs(t, cookie, "owner", "private.txt", "mine")
	defer Delete("owner", f.ID)

	rec := httptest.NewRecorder()
	Handler(rec, httptest.NewRequest("POST", "/files/"+f.ID+"/delete", nil))

	if loc := rec.Header().Get("Location"); rec.Code == http.StatusSeeOther && !strings.Contains(loc, "login") {
		t.Errorf("a signed-out request was handled: %d %s", rec.Code, loc)
	}
	if _, _, err := Get("owner", f.ID); err != nil {
		t.Errorf("a signed-out request deleted a file: %v", err)
	}
}

// One account's actions must not reach another's files, even with a valid
// session and token of its own.
func TestActionsCannotTouchAnotherAccountsFiles(t *testing.T) {
	owner := session(t, "alice2")
	f := uploadAs(t, owner, "alice2", "alices.txt", "hers")
	defer Delete("alice2", f.ID)

	postAction(t, session(t, "mallory"), "/files/"+f.ID+"/delete", nil)
	if _, _, err := Get("alice2", f.ID); err != nil {
		t.Errorf("another account deleted alice's file: %v", err)
	}

	postAction(t, session(t, "mallory"), "/files/"+f.ID+"/share", url.Values{"public": {"1"}})
	if _, _, err := Get("bob", f.ID); err == nil {
		t.Error("another account made alice's file public")
	}
}

// The page has to offer the actions, or they might as well not exist.
func TestListPageOffersTheActions(t *testing.T) {
	cookie := session(t, "browser")
	f := uploadAs(t, cookie, "browser", "listed.txt", "visible")
	defer Delete("browser", f.ID)

	r := httptest.NewRequest("GET", "/files", nil)
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	body := rec.Body.String()
	for _, want := range []string{
		`enctype="multipart/form-data"`,
		`name="file"`,
		`/files/` + f.ID + `/delete`,
		`/files/` + f.ID + `/share`,
		`name="_csrf"`,
		"listed.txt",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the files page is missing %q", want)
		}
	}
}

// The phone layout is CSS, but it hangs off the markup: without a thead to
// hide and classed cells to restack, a narrow screen gets a five-column table
// and scrolls sideways. Nothing else would catch that.
func TestListPageMarkupSupportsTheMobileLayout(t *testing.T) {
	cookie := session(t, "phone")
	f := uploadAs(t, cookie, "phone", "onphone.txt", "small")
	defer Delete("phone", f.ID)

	r := httptest.NewRequest("GET", "/files", nil)
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	body := rec.Body.String()
	for _, want := range []string{
		`class="data-table files-table"`,
		"<thead>", // hidden below 600px
		"<tbody>",
		`class="file-name"`,
		`class="file-meta"`,
		`class="file-actions"`,
		".files-table thead{display:none}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the files page is missing %q, which the phone layout needs", want)
		}
	}

	// The card must not repeat the page title — on a phone that is a whole line
	// of screen spent saying Files twice.
	if strings.Contains(body, "<h3>Files</h3>") {
		t.Error("the card repeats the page heading")
	}
}

func TestFilesPageOwnsSFTPOnboarding(t *testing.T) {
	t.Setenv("SHELL_SSH_PORT", "2222")
	cookie := session(t, "sftppage")
	r := httptest.NewRequest("GET", "/files", nil)
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	body := rec.Body.String()
	for _, want := range []string{"<h3>SFTP</h3>", "sftp -P 2222", `name="sshkey"`, "Add key"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Files page is missing %q", want)
		}
	}
}

// An upload past the limit is a mistake a person can fix, so it comes back as a
// message on the page rather than an error screen.
func TestOversizeUploadReturnsToThePageWithAMessage(t *testing.T) {
	cookie := session(t, "heavy")

	rec := httptest.NewRecorder()
	Handler(rec, uploadRequest(t, cookie, csrfFor(t, cookie), "huge.txt", strings.Repeat("x", MaxBytes+1)))

	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
		t.Fatalf("oversize upload returned %d %q, want a redirect carrying an error", rec.Code, loc)
	}
	if got := List("heavy"); len(got) != 0 {
		t.Errorf("an oversize file was stored anyway: %+v", got)
	}
}
