package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/pay"
)

type testPay struct{}

func (testPay) Checkout(_ context.Context, r pay.Record) (string, string, int64, error) {
	return "cs_" + r.Session.Id, "https://checkout.stripe.com/c/pay/test", time.Now().Add(24 * time.Hour).Unix(), nil
}
func (testPay) Credit(context.Context, pay.Record) error { return nil }

func TestMuPayAgentAPIAndNormalChat(t *testing.T) {
	const owner = "pay_api_owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Admin: true})
	t.Cleanup(func() { auth.RemoveAccountForTest(owner) })
	a, _, e := CreateAgent(owner, "Pay API agent", Hosted, "Be concise.", "", nil, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = SetPay(owner, a.ID, &pay.Policy{Enabled: true, Price: 300, Currency: "usd", Minutes: 60, ReturnUrl: "https://malten.me/session"}); e != nil {
		t.Fatal(e)
	}
	a = For(owner, a.ID)
	if a.Pay == nil || a.PaySecret == "" {
		t.Fatal("policy not persisted")
	}
	if e = SetPay("other", a.ID, a.Pay); e == nil {
		t.Fatal("foreign pricing updated")
	}
	old := pay.Backend
	pay.Backend = testPay{}
	t.Cleanup(func() { pay.Backend = old })
	_, token, e := auth.CreateToken(owner, "pay", []string{"read", "write", "api:agent"}, time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	_, readonly, e := auth.CreateToken(owner, "pay-read", []string{"read", "api:agent"}, time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	_, noscope, e := auth.CreateToken(owner, "pay-no-scope", []string{"read", "write", "api:inbox"}, time.Time{})
	if e != nil {
		t.Fatal(e)
	}
	base := "/agent/" + Slug(a)
	send := func(method, path, body, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
	for _, key := range []string{"", readonly, noscope} {
		if w := send("POST", base+"/pay", `{"reference":"denied","participants":1}`, key); w.Code == 200 {
			t.Fatal("unauthorized checkout", key)
		}
	}
	for _, body := range []string{`{"reference":"bad","participants":3,"price":1}`, `{"reference":"bad","participants":3,"owner":"other"}`, `{"reference":"bad","participants":0}`} {
		if w := send("POST", base+"/pay", body, token); w.Code != 400 {
			t.Fatal("unsafe checkout", w.Code, w.Body.String())
		}
	}
	w := send("POST", base+"/pay", `{"reference":"api-booking","participants":3}`, token)
	var s pay.Session
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &s) != nil || s.Id == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = send("GET", base+"/pay/"+s.Id, "", readonly); w.Code != 200 {
		t.Fatal("read denied", w.Code, w.Body.String())
	}
	if w = send("POST", base+"/pay/"+s.Id+"/start", "", token); w.Code != 402 {
		t.Fatal("unpaid start", w.Code)
	}
	if e = pay.Settle(context.Background(), pay.Settlement{ID: s.Id, StripeID: "cs_" + s.Id, Owner: owner, Agent: a.ID, Amount: s.Amount, Currency: s.Currency, Status: "paid"}); e != nil {
		t.Fatal(e)
	}
	if w = send("POST", base+"/pay/"+s.Id+"/start", "", token); w.Code != 200 {
		t.Fatal("start denied", w.Code, w.Body.String())
	}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"A helpful answer."},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	t.Setenv("AI_PROVIDER", "local")
	t.Setenv("OPENAI_BASE_URL", upstream.URL)
	t.Setenv("OPENAI_MODEL", "pay-test")
	t.Setenv("AGENT_MODEL", "")
	ask := func(id, thread string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(apiAsk{Prompt: "Hello", Session: id, Thread: thread})
		return send("POST", base, string(b), token)
	}
	w = ask(s.Id, "")
	var answer apiAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.Thread == "" {
		t.Fatal("paid prompt", w.Code, w.Body.String())
	}
	w = ask(s.Id, "")
	var resumed apiAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &resumed) != nil || resumed.Thread != answer.Thread {
		t.Fatal("session did not recover thread", w.Code, w.Body.String())
	}
	if w = ask("", answer.Thread); w.Code != 402 {
		t.Fatal("omission bypass", w.Code)
	}
	var records map[string]pay.Record
	if e = data.LoadJSON("pay/sessions.json", &records); e != nil {
		t.Fatal(e)
	}
	records[s.Id].Session.EndsAt = time.Now().Unix() - 1
	if e = data.SaveJSON("pay/sessions.json", records); e != nil {
		t.Fatal(e)
	}
	if w = ask(s.Id, answer.Thread); w.Code != 402 {
		t.Fatal("expiry bypass", w.Code)
	}
	if calls != 2 {
		t.Fatal("invalid calls reached model", calls)
	}
	if w = ask("", ""); w.Code != 200 {
		t.Fatal("ordinary chat blocked", w.Code, w.Body.String())
	}
	if calls != 3 {
		t.Fatal("ordinary chat did not run")
	}
	var ordinary apiAnswer
	if e := json.Unmarshal(w.Body.Bytes(), &ordinary); e != nil || ordinary.Thread == "" {
		t.Fatal("normal stream missing thread", e)
	}
	// Malten mentions use ordinary threaded calls, not public_only.
	if w = ask("", ordinary.Thread); w.Code != 200 || calls != 4 {
		t.Fatal("normal stream follow-up blocked", w.Code, w.Body.String())
	}
	// Integration tokens cannot edit the owner's billing policy through the form.
	r := httptest.NewRequest("POST", "/agents/data", strings.NewReader("action=save&id="+a.ID+"&pay_present=true&pay_enabled=true&pay_price=1"))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	AgentsHandler(w, r)
	if w.Code != 403 {
		t.Fatal("token could change policy", w.Code, w.Body.String())
	}
	if For(owner, a.ID).Pay.Price != 300 {
		t.Fatal("price changed")
	}
}
