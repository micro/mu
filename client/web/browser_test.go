package web_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	web "mu/client/web"
)

// Two real origins: client files never come from the runtime. Exercise browser
// OAuth, CORS, explicit tokens, navigation, rendering and recovery end-to-end.
func TestFocusedClientBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser gate runs separately")
	}
	chrome := os.Getenv("CHROME_PATH")
	if chrome == "" {
		t.Skip("Chrome unavailable")
	}
	var calls atomic.Int32
	var catalogStarted atomic.Bool
	catalogRelease := make(chan struct{})
	defer func() {
		select {
		case <-catalogRelease:
		default:
			close(catalogRelease)
		}
	}()
	var mu sync.Mutex
	var callback, challenge string
	client := httptest.NewServer(web.Handler())
	defer client.Close()
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Micro-Protocol, Micro-Service, Micro-Endpoint, Micro-Id")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Expose-Headers", "Micro-Protocol, Micro-Id, Micro-Thread")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/register":
			var body struct {
				Redirects []string `json:"redirect_uris"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Redirects) != 1 || body.Redirects[0] != client.URL+"/" {
				t.Error("wrong callback")
			}
			mu.Lock()
			callback = body.Redirects[0]
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"client_id": "web-client"})
			return
		case "/oauth/authorize":
			q := r.URL.Query()
			if q.Get("scope") != "read write api:agent" || q.Get("code_challenge_method") != "S256" {
				t.Error("wrong grant")
			}
			mu.Lock()
			challenge = q.Get("code_challenge")
			target := callback + "?code=test-code&state=" + url.QueryEscape(q.Get("state"))
			mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "ambient-cookie", Path: "/"})
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		case "/oauth/token":
			r.ParseForm()
			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			mu.Lock()
			valid := base64.RawURLEncoding.EncodeToString(digest[:]) == challenge && r.Form.Get("redirect_uri") == callback
			mu.Unlock()
			if !valid || r.Form.Get("code") != "test-code" || r.Header.Get("Cookie") != "" {
				t.Error("invalid PKCE exchange or ambient credentials")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "scoped-token", "token_type": "Bearer", "expires_in": 86400})
			return
		}
		if r.URL.Path != "/" || r.Method != "POST" || r.Header.Get("Micro-Protocol") != "mucp/1" || r.Header.Get("Authorization") != "Bearer scoped-token" || r.Header.Get("Cookie") != "" {
			http.Error(w, "invalid client request", 401)
			return
		}
		w.Header().Set("Micro-Protocol", "mucp/1")
		w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
		switch r.Header.Get("Micro-Endpoint") {
		case "Assistant.ListAgents":
			if !catalogStarted.Swap(true) {
				select {
				case <-catalogRelease:
				case <-r.Context().Done():
					return
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"agents": []any{map[string]string{"id": "micro", "slug": "micro", "name": "Micro", "description": "A personal assistant"}}})
		case "Assistant.ListThreads":
			json.NewEncoder(w).Encode(map[string]any{"threads": []any{map[string]string{"id": "own-thread", "agentId": "micro", "subject": "Browser conversation"}}})
		case "Assistant.ReadThread":
			json.NewEncoder(w).Encode(map[string]any{"thread": map[string]string{"id": "own-thread", "agentId": "micro", "subject": "Browser conversation"}, "messages": []any{map[string]string{"id": "one", "role": "ROLE_AGENT", "text": "**Recovered** reply"}}})
		case "Assistant.Ask":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			calls.Add(1)
			if body["mode"] != "ACCESS_MODE_PERSONAL" || body["agentId"] != "micro" {
				t.Error("wrong request scope")
			}
			if body["prompt"] == "Pending" {
				w.Header().Set("Micro-Thread", "own-thread")
				w.WriteHeader(504)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "Working"}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"thread": "own-thread", "text": "**Forecast**\n\n- Clear skies\n\n<script>window.pwned=true</script>\n\n[Unsafe](javascript:alert(1))", "content": []any{map[string]any{"fallback": "Future widget", "future": map[string]string{"html": "<script>bad()</script>"}}, map[string]any{"source": map[string]string{"title": "Weather source", "url": "https://example.com/weather"}}}})
		default:
			t.Error("unknown protocol call")
			http.Error(w, "unknown", 400)
		}
	}))
	defer runtime.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := chromedp.Run(ctx, chromedp.Navigate(client.URL), chromedp.WaitVisible("#runtime"), chromedp.SetValue("#runtime", runtime.URL), chromedp.Click("form button"), chromedp.WaitReady("#new-chat"))
	if err != nil {
		t.Fatalf("independent sign-in: %v", err)
	}
	var disabled bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#new-chat').disabled`, &disabled)); err != nil || !disabled {
		t.Fatalf("new chat interrupted agent discovery: %v", err)
	}
	close(catalogRelease)
	if err := chromedp.Run(ctx, chromedp.WaitEnabled("#send")); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int64{320, 390, 1440} {
		var valid bool
		err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(width, 900, 1, width < 500), chromedp.Navigate(client.URL), chromedp.WaitEnabled("#send"), chromedp.SendKeys("#prompt", "Weather please\r"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`document.querySelector('.message-body strong').textContent==='Forecast' && !window.pwned && !document.querySelector('a[href^="javascript:"]') && document.querySelector('#responses').textContent.includes('Future widget') && document.documentElement.scrollWidth<=innerWidth && document.querySelector('#send').getBoundingClientRect().bottom<=innerHeight && document.querySelector('#send').getBoundingClientRect().top>document.querySelector('.answer').getBoundingClientRect().top`, &valid))
		if err != nil || !valid {
			t.Fatalf("width %d: %v valid=%v", width, err, valid)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.FullScreenshot(&image, 90)); err != nil {
				t.Fatal(err)
			}
			os.MkdirAll(dir, 0755)
			name := "web-mobile.png"
			if width > 500 {
				name = "web-desktop.png"
			}
			if err := os.WriteFile(filepath.Join(dir, name), image, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("unexpected resubmissions: %d", calls.Load())
	}
	var valid bool
	err = chromedp.Run(ctx, chromedp.Navigate(client.URL+"/?view=chats"), chromedp.WaitVisible(".collection-item"), chromedp.Click(".collection-item"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`document.querySelector('#responses').textContent.includes('Recovered')`, &valid))
	if err != nil || !valid {
		t.Fatalf("thread history: %v", err)
	}
	err = chromedp.Run(ctx, chromedp.Click("#new-chat"), chromedp.WaitEnabled("#send"), chromedp.SendKeys("#prompt", "Pending\r"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`document.querySelector('#responses').textContent.includes('Recovered')`, &valid))
	if err != nil || !valid {
		t.Fatalf("pending recovery: %v", err)
	}
}
