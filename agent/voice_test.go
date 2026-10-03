package agent

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/phone"
	"mu/internal/voice"
)

func signCall(r *http.Request, url string, form url.Values) {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data := url
	for _, k := range keys {
		data += k + form.Get(k)
	}
	mac := hmac.New(sha1.New, []byte("test-call-secret"))
	mac.Write([]byte(data))
	r.Header.Set("X-Twilio-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}
func TestVoiceRequiresAccountAndStrictCSRF(t *testing.T) {
	const owner = "voice-access-test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Admin: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		session, csrf bool
		status        int
	}{{false, false, 401}, {true, false, 403}, {true, true, 503}} {
		r := httptest.NewRequest("POST", "/agent/voice/speak", strings.NewReader(`{"text":"hello"}`))
		r.Header.Set("Content-Type", "application/json")
		if tc.session {
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		if tc.csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		VoiceHandler(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body)
		}
	}
}
func TestCallRequiresProviderNumberAndOneUseCode(t *testing.T) {
	t.Setenv("TWILIO_VOICE_ENABLED", "true")
	t.Setenv("TWILIO_VOICE_FROM", "+447700901234")
	t.Setenv("TWILIO_ACCOUNT_SID", "ACtest")
	t.Setenv("TWILIO_AUTH_TOKEN", "test-call-secret")
	t.Setenv("MU_DOMAIN", "micro.test")
	const owner = "voice-call-owner"
	const number = "+447700901235"
	auth.SetAccountForTest(&auth.Account{ID: owner, Admin: true})
	defer auth.RemoveAccountForTest(owner)
	if err := phone.Verify(owner, number); err != nil {
		t.Fatal(err)
	}
	defer phone.Forget(owner, number)
	code, _ := voice.NewCode(owner)
	form := url.Values{"AccountSid": {"ACtest"}, "CallSid": {"CAtest"}, "From": {number}, "To": {"+447700901234"}}
	call := func(signed bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/voice/webhook", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if signed {
			signCall(r, "https://micro.test/voice/webhook", form)
		}
		w := httptest.NewRecorder()
		CallWebhookHandler(w, r)
		return w
	}
	if w := call(false); w.Code != 403 {
		t.Fatal("unsigned call accepted")
	}
	form.Set("Digits", code)
	if strings.Contains(call(true).Body.String(), "ConversationRelay") {
		t.Fatal("skipped challenge")
	}
	form.Del("Digits")
	if !strings.Contains(call(true).Body.String(), "Gather") {
		t.Fatal("no keypad challenge")
	}
	form.Set("Digits", "bad")
	if strings.Contains(call(true).Body.String(), "ConversationRelay") {
		t.Fatal("bad code accepted")
	}
	form.Set("Digits", code)
	if !strings.Contains(call(true).Body.String(), "ConversationRelay") {
		t.Fatal("valid call refused")
	}
	if strings.Contains(call(true).Body.String(), "ConversationRelay") {
		t.Fatal("signed retry replayed authorization")
	}
	form.Set("CallSid", "CAunknown")
	form.Set("From", "+447700901299")
	form.Del("Digits")
	if strings.Contains(call(true).Body.String(), "Gather") {
		t.Fatal("unknown caller admitted")
	}
	calls.Lock()
	delete(calls.grants, "CAtest")
	calls.Unlock()
}

func TestRelayBindsSetupAndRejectsReplay(t *testing.T) {
	t.Setenv("TWILIO_VOICE_ENABLED", "true")
	t.Setenv("TWILIO_VOICE_FROM", "+447700902234")
	t.Setenv("TWILIO_ACCOUNT_SID", "ACtest")
	t.Setenv("TWILIO_AUTH_TOKEN", "test-call-secret")
	t.Setenv("MU_DOMAIN", "micro.test")
	const owner = "voice-relay-owner"
	const number = "+447700902235"
	auth.SetAccountForTest(&auth.Account{ID: owner, Admin: true})
	defer auth.RemoveAccountForTest(owner)
	if err := phone.Verify(owner, number); err != nil {
		t.Fatal(err)
	}
	defer phone.Forget(owner, number)
	calls.Lock()
	calls.grants["CArelay"] = callGrant{owner: owner, from: number, to: "+447700902234", secret: "setup-secret", until: time.Now().Add(time.Minute)}
	calls.Unlock()
	defer func() { calls.Lock(); delete(calls.grants, "CArelay"); calls.Unlock() }()
	srv := httptest.NewServer(http.HandlerFunc(CallRelayHandler))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	r := httptest.NewRequest("GET", "/voice/relay", nil)
	signCall(r, "wss://micro.test/voice/relay", nil)
	if c, rsp, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
		c.Close()
		t.Fatal("unsigned websocket accepted")
	} else if rsp.StatusCode != 403 {
		t.Fatalf("status %d", rsp.StatusCode)
	}
	connect := func(secret string) {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, r.Header)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		c.WriteJSON(callMessage{Type: "setup", AccountSID: "ACtest", CallSID: "CArelay", From: number, To: "+447700902234", Parameters: map[string]string{"key": secret}})
		c.WriteJSON(callMessage{Type: "error"})
		if _, _, err := c.ReadMessage(); err == nil {
			t.Fatal("unexpected answer")
		}
	}
	connect("wrong-secret")
	calls.Lock()
	used := calls.grants["CArelay"].connected
	calls.Unlock()
	if used {
		t.Fatal("wrong setup consumed grant")
	}
	connect("setup-secret")
	calls.Lock()
	used = calls.grants["CArelay"].connected
	calls.Unlock()
	if !used {
		t.Fatal("valid setup was not accepted")
	}
	connect("setup-secret")
}
