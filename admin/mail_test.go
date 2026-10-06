package admin

import (
	"encoding/base64"
	"mu/internal/auth"
	"mu/service/mail"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperatorInboxRequiresAdminAndRetainsReports(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	actor := "operator-ui-admin"
	other := "operator-ui-person"
	for _, a := range []*auth.Account{{ID: actor, Admin: true}, {ID: other}} {
		auth.SetAccountForTest(a)
		defer auth.RemoveAccountForTest(a.ID)
	}
	adminSession, _ := auth.CreateSession(actor)
	userSession, _ := auth.CreateSession(other)
	xml := `<feedback><report_metadata><org_name>Reporter</org_name><report_id>example-report</report_id></report_metadata><policy_published><domain>example.test</domain></policy_published><record><row><count>2</count><policy_evaluated><dkim>pass</dkim><spf>fail</spf></policy_evaluated></row></record></feedback>`
	for _, id := range []string{"<operator-report-1>", "<operator-report-2>"} {
		if err := mail.SendMessageTo(mail.Delivery{ToID: auth.OperatorID, Tag: "dmarc", FromID: "dmarc@google.com", Subject: "Private operator report", MessageID: id, Attachment: &mail.Attachment{Name: "report.xml", Content: []byte(xml)}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := mail.SendMessageTo(mail.Delivery{ToID: other, Subject: "Personal secret", Body: "Should not appear here"}); err != nil {
		t.Fatal(err)
	}
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: token})
		}
		w := httptest.NewRecorder()
		MailHandler(w, r)
		return w
	}
	for _, token := range []string{"", userSession.Token} {
		w := request("/admin/mail", token)
		if w.Code != 403 || strings.Contains(w.Body.String(), "Private operator report") {
			t.Fatal("shared inbox exposed")
		}
	}
	w := request("/admin/mail?view=dmarc", adminSession.Token)
	if w.Code != 200 || strings.Count(w.Body.String(), "example.test · Reporter") != 1 || !strings.Contains(w.Body.String(), "2 messages · 0 failed DMARC alignment") {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "Personal secret") || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("wrong scope or cache")
	}
	msgs, _ := mail.OperatorMessages(actor)
	id := msgs[0].ID
	w = request("/admin/mail?id="+id+"&download=1", adminSession.Token)
	if base64.StdEncoding.EncodeToString(w.Body.Bytes()) != msgs[0].Attachment {
		t.Fatal("original attachment changed")
	}
	r := httptest.NewRequest("POST", "/admin/mail?id="+id, strings.NewReader("action=read"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: adminSession.Token})
	w = httptest.NewRecorder()
	MailHandler(w, r)
	if w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	r = httptest.NewRequest("POST", "/admin/mail?id="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: adminSession.Token})
	token := auth.CSRFToken(r)
	r = httptest.NewRequest("POST", "/admin/mail?id="+id, strings.NewReader("action=read&_csrf="+token))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: adminSession.Token})
	w = httptest.NewRecorder()
	MailHandler(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatal("admin read action failed", w.Code, w.Body.String())
	}
	updated, _ := mail.OperatorMessages(actor)
	for _, m := range updated {
		if m.ID == id && !m.Read {
			t.Fatal("shared read state not updated")
		}
	}

}
