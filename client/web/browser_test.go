package web_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
		w.Header().Set("Access-Control-Expose-Headers", "Micro-Protocol, Micro-Service, Micro-Endpoint, Micro-Id, Micro-Thread")
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
			if q.Get("scope") != "read write api:agent api:home api:inbox api:services" || q.Get("code_challenge_method") != "S256" {
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
		w.Header().Set("Micro-Service", r.Header.Get("Micro-Service"))
		w.Header().Set("Micro-Endpoint", r.Header.Get("Micro-Endpoint"))
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
		case "Assistant.Submit":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			body = body["request"].(map[string]any)
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
			json.NewEncoder(w).Encode(map[string]any{"id": "op-one", "thread": "own-thread", "state": "OPERATION_STATE_SUCCEEDED", "result": map[string]any{"thread": "own-thread", "text": "**Forecast**\n\n- Clear skies\n\n<script>window.pwned=true</script>\n\n[Unsafe](javascript:alert(1))", "content": []any{map[string]any{"fallback": "Future widget", "future": map[string]string{"html": "<script>bad()</script>"}}, map[string]any{"source": map[string]string{"title": "Weather source", "url": "https://example.com/weather"}}}}})
		default:
			t.Error("unknown protocol call")
			http.Error(w, "unknown", 400)
		}
	}))
	defer runtime.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true), chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1, EXCLUDE localhost"))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := chromedp.Run(ctx, chromedp.Navigate(client.URL+"/?view=settings"), chromedp.WaitVisible("#runtime"), chromedp.Evaluate(`document.querySelector("#runtime").select()`, nil), chromedp.SendKeys("#runtime", runtime.URL), chromedp.Click("form button"), chromedp.WaitVisible("#sign-in"), chromedp.Click("#sign-in"), chromedp.WaitReady("#new-chat"))
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

func TestHostedClientBrowser(t *testing.T) {
	if testing.Short() || os.Getenv("CHROME_PATH") == "" {
		t.Skip("browser gate runs separately")
	}
	var empty atomic.Bool
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app", web.Handler()))
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := r.Cookie("session"); err != nil {
			w.Write([]byte(`{"type":"guest"}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "csrf_token", Value: "csrf-proof", Path: "/"})
		w.Write([]byte(`{"account":"owner","type":"session"}`))
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("redirect") != "/app/" {
			t.Error("login lost app destination")
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "browser-session", Path: "/", HttpOnly: true})
		http.Redirect(w, r, "/app/", 303)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie("session")
		if r.URL.Path != "/" || r.Method != "POST" || err != nil || cookie.Value != "browser-session" || r.Header.Get("Authorization") != "" || r.Header.Get("X-CSRF-Token") != "csrf-proof" {
			t.Errorf("hosted client did not reuse its session safely: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Micro-Protocol", "mucp/1")
		w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
		w.Header().Set("Micro-Service", r.Header.Get("Micro-Service"))
		w.Header().Set("Micro-Endpoint", r.Header.Get("Micro-Endpoint"))
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("Micro-Endpoint") {
		case "Inbox.Read":
			w.Write([]byte(`{"item":{"id":"inbox-one","subject":"A useful update","channel":"mail","unread":true},"messages":[{"id":"mail-one","role":"ROLE_PERSON","text":"Your appointment is confirmed."}],"nextPageToken":""}`))
		case "Inbox.MarkRead":
			w.Write([]byte(`{"id":"inbox-one","subject":"A useful update","channel":"mail","unread":false}`))
		case "Home.Overview", "Inbox.List", "Services.List":
			op := r.Header.Get("Micro-Endpoint")
			if empty.Load() {
				switch op {
				case "Home.Overview":
					w.Write([]byte(`{"agents":[],"inbox":[],"pinnedServices":[],"events":[],"brief":"","calendarPending":true}`))
				case "Inbox.List":
					w.Write([]byte(`{"items":[],"nextPageToken":""}`))
				case "Services.List":
					w.Write([]byte(`{"services":[],"nextPageToken":""}`))
				}
				return
			}
			name := map[string]string{"Home.Overview": "home-response.json", "Inbox.List": "inbox-response.json", "Services.List": "services-response.json"}[op]
			data, err := os.ReadFile(filepath.Join("../../proto/testdata", name))
			if err != nil {
				t.Error(err)
			}
			w.Write(data)
		case "Assistant.ListAgents":
			w.Write([]byte(`{"agents":[{"id":"micro","slug":"micro","name":"Micro"}]}`))
		case "Assistant.Submit":
			w.Write([]byte(`{"id":"op-one","thread":"browser-thread","state":"OPERATION_STATE_SUCCEEDED","result":{"thread":"browser-thread","text":"Session reply","content":[]}}`))
		default:
			t.Error("unexpected operation")
			http.Error(w, "unknown", 400)
		}
	})
	runtime := httptest.NewServer(mux)
	defer runtime.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(os.Getenv("CHROME_PATH")), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var valid bool
	err := chromedp.Run(ctx, chromedp.Navigate(runtime.URL+"/app/"), chromedp.WaitVisible("#sign-in"), chromedp.Evaluate(`!document.querySelector('#runtime') && !!document.querySelector('a[href*="/signup?"]')`, &valid))
	if err != nil || !valid {
		t.Fatalf("default sign-in: %v valid=%v", err, valid)
	}
	err = chromedp.Run(ctx, chromedp.Click("#sign-in"), chromedp.WaitEnabled("#send"), chromedp.SendKeys("#prompt", "Hello\r"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`document.querySelector('#responses').textContent.includes('Session reply')`, &valid))
	if err != nil || !valid {
		t.Fatalf("session reuse: %v valid=%v", err, valid)
	}
	for _, width := range []int64{390, 1280} {
		for _, isEmpty := range []bool{false, true} {
			empty.Store(isEmpty)
			for _, view := range []string{"home", "inbox", "services"} {
				var screenshot []byte
				var overflow bool
				err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(width, 900, 1, width < 700), chromedp.Navigate(runtime.URL+"/app/?view="+view), chromedp.WaitVisible("h1"), chromedp.Poll(`!document.querySelector('[role="status"]')`, nil), chromedp.Evaluate(`document.documentElement.scrollWidth>innerWidth`, &overflow), chromedp.FullScreenshot(&screenshot, 90))
				if err != nil || overflow {
					t.Fatalf("%s width=%d empty=%t: %v overflow=%t", view, width, isEmpty, err, overflow)
				}
				if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
					os.MkdirAll(dir, 0755)
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("composition-%s-%d-%t.png", view, width, isEmpty)), screenshot, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}

	empty.Store(false)
	err = chromedp.Run(ctx, chromedp.Navigate(runtime.URL+"/app/?view=inbox"), chromedp.WaitVisible(".collection-item"), chromedp.Click(".collection-item"), chromedp.Poll(`document.body.textContent.includes('Your appointment is confirmed.')`, nil), chromedp.Evaluate(`Array.from(document.querySelectorAll('button')).find(b=>b.textContent==='Mark read').click()`, nil), chromedp.Poll(`Array.from(document.querySelectorAll('button')).some(b=>b.textContent==='Mark unread')`, nil))
	if err != nil {
		t.Fatalf("inbox read state: %v", err)
	}

}
