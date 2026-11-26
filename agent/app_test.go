package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/pay"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
)

func appFixture(t *testing.T) (*Agent, map[string]string) {
	t.Helper()
	tokens := map[string]string{}
	for _, who := range []string{"owner", "alice", "bob"} {
		id := "pwa-" + t.Name() + "-" + who
		auth.SetAccountForTest(&auth.Account{ID: id, Name: who, Approved: true, Admin: who == "owner"})
		t.Cleanup(func() { auth.RemoveAccountForTest(id) })
		s, e := auth.CreateSession(id)
		if e != nil {
			t.Fatal(e)
		}
		tokens[who] = s.Token
		tokens[who+"ID"] = id
	}
	a, _, e := CreateAgent(tokens["ownerID"], "App "+t.Name(), Hosted, "You are Teacup. Be thoughtful.", "A private conversation.", nil, false)
	if e != nil {
		t.Fatal(e)
	}
	var icon bytes.Buffer
	_ = png.Encode(&icon, image.NewRGBA(image.Rect(0, 0, 64, 64)))
	id, e := saveAppIcon(&icon)
	if e != nil {
		t.Fatal(e)
	}
	a.App = &appSettings{Published: true, Name: "Teacup", Description: "Your companion", Icon: id}
	if e = a.save(); e != nil {
		t.Fatal(e)
	}
	return a, tokens
}
func appRequest(method, path, body, token string, csrf bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://micro.test"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "session", Value: token})
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	}
	w := httptest.NewRecorder()
	Handler(w, r)
	return w
}
func TestAgentAppIdentityAndPublishing(t *testing.T) {
	a, tokens := appFixture(t)
	base := appPath(a)
	w := appRequest("GET", base+"manifest.webmanifest", "", "", false)
	var manifest map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manifest) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, k := range []string{"id", "scope", "start_url"} {
		if manifest[k] != base {
			t.Fatal(k, manifest[k])
		}
	}
	if manifest["name"] != "Teacup" {
		t.Fatal(manifest)
	}
	for _, token := range []string{"", tokens["owner"], tokens["alice"]} {
		w = appRequest("GET", base, "", token, false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), base+"manifest.webmanifest") || strings.Contains(w.Body.String(), `href="/manifest.webmanifest"`) {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), a.Prompt) || strings.Contains(w.Body.String(), "b-pay") {
			t.Fatal("private config exposed")
		}
	}
	if w = appRequest("GET", base+"settings", "", tokens["alice"], false); w.Code != 404 {
		t.Fatal("customer settings", w.Code)
	}
	for _, size := range []string{"192", "512"} {
		w = appRequest("GET", base+"icon-"+size+".png", "", "", false)
		cfg, err := png.DecodeConfig(w.Body)
		if err != nil || fmt.Sprint(cfg.Width) != size || cfg.Height != cfg.Width {
			t.Fatal("invalid icon", err)
		}
	}
	a.App.Published = false
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"", "manifest.webmanifest", "icon-192.png", "chats", "sw.js"} {
		if w = appRequest("GET", base+route, "", tokens["alice"], false); w.Code != 404 {
			t.Fatal("unpublished app served", route, w.Code)
		}
	}
}
func TestAgentAppCustomerChatIsolation(t *testing.T) {
	a, tokens := appFixture(t)
	base := appPath(a)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Contains(raw, []byte("Be thoughtful")) {
			t.Error("agent instructions missing")
		}
		if bytes.Contains(raw, []byte("OWNER PRIVATE")) {
			t.Error("creator history leaked")
		}
		if calls == 1 && !bytes.Contains(raw, []byte("Hello there.")) {
			t.Error("customer history missing")
		}
		calls++
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Hello there."},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	t.Setenv("AI_PROVIDER", "local")
	t.Setenv("OPENAI_BASE_URL", upstream.URL)
	t.Setenv("OPENAI_MODEL", "app-test")
	t.Setenv("AGENT_MODEL", "")
	t.Setenv("GUEST_MODEL", "")
	private := thread.Open(a.Owner, "web", "private-app-owner")
	Said(a.Owner, private.ID, "OWNER PRIVATE", "", a.Owner)
	opts := appQuery(a, nil)
	if !opts.Public || !opts.anonymousTools || opts.Thread != "" || opts.Extra != "" || len(managementTools(a.Owner, opts)) != 0 {
		t.Fatal("private context enabled")
	}
	w := appRequest("POST", base+"chat", `{"prompt":"Hello"}`, tokens["alice"], false)
	if w.Code != 403 {
		t.Fatal("CSRF accepted", w.Code)
	}
	w = appRequest("POST", base+"chat", `{"prompt":"Hello"}`, "", true)
	if w.Code != 401 {
		t.Fatal("guest accepted", w.Code)
	}
	w = appRequest("POST", base+"chat", `{"prompt":"Hello"}`, tokens["alice"], true)
	var answer pb.AppAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.Thread == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	if thread.Get(a.Owner, answer.Thread) != nil || len(thread.Messages(tokens["aliceID"], answer.Thread, 0)) != 2 {
		t.Fatal("history ownership")
	}
	for _, who := range []string{"owner", "bob"} {
		w = appRequest("GET", base+"chats/"+answer.Thread, "", tokens[who], false)
		if w.Code != 404 {
			t.Fatal("foreign read", who, w.Code)
		}
		w = appRequest("POST", base+"chat", `{"prompt":"steal","thread":"`+answer.Thread+`"}`, tokens[who], true)
		if w.Code == 200 {
			t.Fatal("foreign continuation")
		}
	}
	w = appRequest("POST", base+"chat", `{"prompt":"continue","thread":"`+answer.Thread+`"}`, tokens["alice"], true)
	if w.Code != 200 || calls != 2 {
		t.Fatal(w.Code, w.Body.String(), calls)
	}
}
func TestAgentAppPaidCustomerAccess(t *testing.T) {
	a, tokens := appFixture(t)
	a.App.Paid = true
	a.Pay = &pay.Policy{Enabled: true, Price: 300, Currency: "gbp", Minutes: 60, ReturnUrl: "https://external.test"}
	if err := a.save(); err != nil {
		t.Fatal(err)
	}
	old := pay.Backend
	pay.Backend = testPay{}
	t.Cleanup(func() { pay.Backend = old })
	base := appPath(a)
	w := appRequest("POST", base+"checkout", `{"reference":"booking-reference-1234","participants":100}`, tokens["alice"], true)
	var s pay.Session
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if s.Participants != 1 || s.Credits != 300 {
		t.Fatal("customer changed price", s.String())
	}
	for _, who := range []string{"owner", "bob"} {
		for _, method := range []string{"GET", "POST"} {
			w = appRequest(method, base+"session/"+s.Id, `{}`, tokens[who], true)
			if w.Code != 404 {
				t.Fatal("foreign receipt", who, method, w.Code)
			}
		}
	}
	for _, id := range []string{"", s.Id} {
		w = appRequest("POST", base+"chat", `{"prompt":"hello","session":"`+id+`"}`, tokens["alice"], true)
		if w.Code == 200 {
			t.Fatal("unpaid chat allowed")
		}
	}
	if err := pay.Settle(context.Background(), pay.Settlement{ID: s.Id, StripeID: "cs_" + s.Id, Owner: a.Owner, Agent: a.ID, Currency: s.Currency, Status: "paid", Amount: s.Amount}); err != nil {
		t.Fatal(err)
	}
	w = appRequest("POST", base+"session/"+s.Id, `{}`, tokens["alice"], true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var started pay.Session
	_ = json.Unmarshal(w.Body.Bytes(), &started)
	w = appRequest("POST", base+"session/"+s.Id, `{}`, tokens["alice"], true)
	var repeated pay.Session
	_ = json.Unmarshal(w.Body.Bytes(), &repeated)
	if repeated.EndsAt != started.EndsAt {
		t.Fatal("clock extended")
	}
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"A paid answer."},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	t.Setenv("AI_PROVIDER", "local")
	t.Setenv("OPENAI_BASE_URL", upstream.URL)
	t.Setenv("OPENAI_MODEL", "paid-app")
	t.Setenv("AGENT_MODEL", "")
	t.Setenv("GUEST_MODEL", "")
	w = appRequest("POST", base+"chat", `{"prompt":"hello","session":"`+s.Id+`"}`, tokens["alice"], true)
	var answer pb.AppAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer.Thread == "" {
		t.Fatal("paid prompt", w.Code, w.Body.String())
	}
	w = appRequest("POST", base+"chat", `{"prompt":"continue","session":"`+s.Id+`"}`, tokens["alice"], true)
	var resumed pb.AppAnswer
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &resumed) != nil || resumed.Thread != answer.Thread || calls != 2 {
		t.Fatal("paid thread recovery", w.Code, w.Body.String())
	}
	w = appRequest("POST", base+"chat", `{"prompt":"bypass","thread":"`+answer.Thread+`"}`, tokens["alice"], true)
	if w.Code == 200 || calls != 2 {
		t.Fatal("omitted payment bypass")
	}
	var records map[string]pay.Record
	if err := data.LoadJSON("pay/sessions.json", &records); err != nil {
		t.Fatal(err)
	}
	records[s.Id].Session.EndsAt = time.Now().Unix() - 1
	if err := data.SaveJSON("pay/sessions.json", records); err != nil {
		t.Fatal(err)
	}
	w = appRequest("POST", base+"chat", `{"prompt":"hello","session":"`+s.Id+`"}`, tokens["alice"], true)
	if w.Code == 200 {
		t.Fatal("expired chat allowed")
	}
	if records[s.Id].Customer != tokens["aliceID"] || !strings.Contains(records[s.Id].ReturnURL, base) {
		t.Fatal("customer/return binding missing")
	}
}
func TestAgentAppIconAndSettings(t *testing.T) {
	a, tokens := appFixture(t)
	for _, raw := range []string{"<svg onload='alert(1)'/>", strings.Repeat("x", (2<<20)+1)} {
		if _, err := saveAppIcon(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid upload accepted")
		}
	}
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)
	_ = mp.WriteField("name", "New app")
	_ = mp.WriteField("description", "A companion")
	_ = mp.WriteField("published", "on")
	_ = mp.WriteField("access", "free")
	_ = mp.Close()
	r := httptest.NewRequest("POST", appPath(a)+"settings", &body)
	r.Header.Set("Content-Type", mp.FormDataContentType())
	r.AddCookie(&http.Cookie{Name: "session", Value: tokens["owner"]})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 303 || For(a.Owner, a.ID).App.Name != "New app" {
		t.Fatal(w.Code, w.Body.String())
	}
}
