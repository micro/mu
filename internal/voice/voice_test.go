package voice

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVoiceRequiresSession(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/voice", nil))
	if w.Code != 302 && w.Code != 303 {
		t.Fatalf("guest: %d", w.Code)
	}
	t.Setenv("HOME", t.TempDir())
	auth.SetAccountForTest(&auth.Account{ID: "voice-owner", Admin: true, Approved: true})
	defer auth.RemoveAccountForTest("voice-owner")
	sess, err := auth.CreateSession("voice-owner")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/voice", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w = httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="voice-enable"`) {
		t.Fatal("missing opt-in")
	}
	if strings.Contains(w.Body.String(), `id="voice-wake" type="checkbox" checked`) {
		t.Fatal("wake listening enabled by default")
	}
}
func TestWorkerPolicyIsScoped(t *testing.T) {
	w := httptest.NewRecorder()
	Worker(w, httptest.NewRequest("GET", "/voice/worker.js", nil))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "wasm-unsafe-eval") {
		t.Fatal("missing worker runtime policy")
	}
	if strings.Contains(w.Header().Get("Content-Security-Policy"), "'unsafe-eval'") {
		t.Fatal("general eval permitted")
	}
	if strings.Contains(w.Body.String(), "api.openai.com") || strings.Contains(w.Body.String(), "speech.googleapis.com") {
		t.Fatal("speech uploaded")
	}
}
