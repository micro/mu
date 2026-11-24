package agent

import (
	"mu/internal/pay"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublishedAgentWebsiteAndBookingOffer(t *testing.T) {
	const owner = "public_pay_owner"
	a, _, e := CreateAgent(owner, "Teacup public offer", Hosted, "private-role-secret", "A casual agent for everyday chitchat.", nil, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = SetLinks(owner, a.ID, "https://teacup.chat", ""); e != nil {
		t.Fatal(e)
	}
	if e = SetLinks("other", a.ID, "https://other.test", ""); e == nil {
		t.Fatal("foreign links updated")
	}
	for _, raw := range []string{"javascript:alert(1)", "https://user:pass@teacup.chat", "http://teacup.chat"} {
		if e = SetLinks(owner, a.ID, raw, ""); e == nil {
			t.Fatal("unsafe link accepted", raw)
		}
	}
	render := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		Handler(w, httptest.NewRequest("GET", "/agent/"+Slug(a), nil))
		return w
	}
	if w := render(); w.Code != 404 {
		t.Fatal("unpublished profile exposed", w.Code)
	}
	if e = SetProfile(owner, a.ID, true); e != nil {
		t.Fatal(e)
	}
	w := render()
	if w.Code != 200 || !strings.Contains(w.Body.String(), `href="https://teacup.chat"`) || strings.Contains(w.Body.String(), "Book a private session") || strings.Contains(w.Body.String(), ">Sign in</a></div>") {
		t.Fatal("website-only profile", w.Code, w.Body.String())
	}
	p := &pay.Policy{Enabled: true, Price: 100, Currency: "gbp", Minutes: 60, ReturnUrl: "https://teacup.chat/private-return", NotifyUrl: "https://teacup.chat/private-callback"}
	if e = SetPay(owner, a.ID, p); e != nil {
		t.Fatal(e)
	}
	a = For(owner, a.ID)
	w = render()
	body := w.Body.String()
	for _, want := range []string{"£1 per person for one hour", "Book a private session", `href="https://teacup.chat"`} {
		if !strings.Contains(body, want) {
			t.Fatal("missing profile offer", want)
		}
	}
	for _, secret := range []string{"private-role-secret", "private-return", "private-callback", a.PaySecret} {
		if strings.Contains(body, secret) {
			t.Fatal("private policy leaked", secret)
		}
	}
	if e = SetLinks(owner, a.ID, "https://teacup.chat", "https://teacup.chat/book"); e != nil {
		t.Fatal(e)
	}
	if body = render().Body.String(); !strings.Contains(body, `href="https://teacup.chat/book"`) {
		t.Fatal("booking override missing")
	}
	p.Enabled = false
	if e = SetPay(owner, a.ID, p); e != nil {
		t.Fatal(e)
	}
	body = render().Body.String()
	if strings.Contains(body, "Book a private session") || strings.Contains(body, "£1 per person") {
		t.Fatal("disabled price shown")
	}
	if !strings.Contains(body, `href="https://teacup.chat"`) {
		t.Fatal("website hidden when pay disabled")
	}
}
