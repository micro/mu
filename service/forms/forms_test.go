package forms

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/userdb"
)

func setup(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	owner := "forms-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Admin: true, Approved: true})
	t.Cleanup(func() { auth.RemoveAccountForTest(owner) })
	limits.Lock()
	limits.entries = map[string]bucket{}
	limits.Unlock()
	return owner
}
func definition() Form {
	return Form{Title: "Contact", Fields: []Field{{Name: "email", Label: "Email", Type: "email"}, {Name: "message", Label: "Message", Type: "textarea", Required: true}}}
}
func TestPublicFormKeepsResponsesPrivate(t *testing.T) {
	owner := setup(t)
	f, e := Save(owner, definition())
	if e != nil {
		t.Fatal(e)
	}
	answers := map[string]string{"message": "Private visitor text"}
	if _, e = Submit(f.ID, answers); e == nil {
		t.Fatal("private form accepted response")
	}
	f.Public = true
	f, e = Save(owner, *f)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Read("other", f.ID); e == nil {
		t.Fatal("other account read management")
	}
	if _, e = Save("other", *f); e == nil {
		t.Fatal("other account edited")
	}
	id, e := Submit(f.ID, answers)
	if e != nil {
		t.Fatal(e)
	}
	for _, caller := range []string{"", "other"} {
		if _, e := userdb.Get(namespace, caller, "responses-"+f.ID, id); e == nil {
			t.Fatal("response exposed")
		}
		if _, _, e := Responses(caller, f.ID, 0); e == nil {
			t.Fatal("response API exposed")
		}
	}
	rs, _, e := Responses(owner, f.ID, 0)
	if e != nil || len(rs) != 1 || rs[0].Answers["message"] != answers["message"] {
		t.Fatal(rs, e)
	}
	f.Fields[1].Label = "Changed label"
	f.Closed = true
	if _, e = Save(owner, *f); e != nil {
		t.Fatal(e)
	}
	if _, e = Submit(f.ID, answers); e == nil {
		t.Fatal("closed form accepted")
	}
	rs, _, _ = Responses(owner, f.ID, 0)
	if rs[0].Fields[1].Label != "Message" {
		t.Fatal("edited schema changed old response")
	}
	if e = Remove(owner, f.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = userdb.Get(namespace, owner, "responses-"+f.ID, id); e == nil {
		t.Fatal("orphan response")
	}
}
func TestSubmissionValidationAndNoCookieAuthority(t *testing.T) {
	owner := setup(t)
	f := definition()
	f.Public = true
	saved, e := Save(owner, f)
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range []map[string]string{{}, {"message": "x", "email": "bad"}, {"message": strings.Repeat("x", 8001)}, {"message": "x", "owner": "other"}} {
		if _, e = Submit(saved.ID, a); e == nil {
			t.Fatal("invalid accepted", a)
		}
	}
	r := httptest.NewRequest("POST", "/forms/submit?id="+saved.ID, strings.NewReader(url.Values{"message": {"<script>alert(1)</script>"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://another-site.test")
	w := httptest.NewRecorder()
	SubmissionHandler(w, r)
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	rs, _, _ := Responses(owner, saved.ID, 0)
	if len(rs) != 1 {
		t.Fatal(rs)
	}
	session, _ := auth.CreateSession(owner)
	r = httptest.NewRequest("GET", "/forms?id="+saved.ID, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w = httptest.NewRecorder()
	Handler(w, r)
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("unescaped response")
	}
	r = httptest.NewRequest("POST", "/forms", strings.NewReader("action=delete&id="+saved.ID))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w = httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	DeleteAll(owner)
	if _, _, e := public(saved.ID); e == nil {
		t.Fatal("deleted account form retained")
	}
}
func TestSubmissionLimitsAndPublication(t *testing.T) {
	owner := setup(t)
	f := definition()
	f.Public = true
	saved, e := Save(owner, f)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 21; i++ {
		r := httptest.NewRequest("POST", "/forms/submit?id="+saved.ID, strings.NewReader("message=hello"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		SubmissionHandler(w, r)
		if i == 20 && w.Code != 429 {
			t.Fatal("no rate limit", w.Code)
		}
	}
	saved.Public = false
	if _, e = Save(owner, *saved); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "/forms/view?id="+saved.ID, nil)
	w := httptest.NewRecorder()
	PublicHandler(w, r)
	if w.Code != 404 {
		t.Fatal("unpublished form exposed")
	}
}

func TestExternalFormJSONReceipt(t *testing.T) {
	owner := setup(t)
	f := definition()
	f.Public = true
	saved, err := Save(owner, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		message string
		status  int
	}{{"Hello from an external form", 200}, {"", 400}} {
		req := httptest.NewRequest("POST", "/forms/submit?id="+saved.ID, strings.NewReader(url.Values{"message": {tc.message}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Origin", "https://example.org")
		w := httptest.NewRecorder()
		SubmissionHandler(w, req)
		if w.Code != tc.status {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal("receipt must be readable without credentials")
		}
		if w.Header().Get("Location") != "" {
			t.Fatal("JSON response redirected off site")
		}
		if tc.status == 200 && !strings.Contains(w.Body.String(), `"received"`) {
			t.Fatal("missing success receipt")
		}
	}
	responses, _, err := Responses(owner, saved.ID, 0)
	if err != nil || len(responses) != 1 {
		t.Fatalf("expected only valid submission saved: %v %d", err, len(responses))
	}
}
