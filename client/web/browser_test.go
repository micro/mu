package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	web "mu/client/web"
	"mu/internal/app"
)

func TestFocusedClientBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser gate runs separately")
	}
	chrome := os.Getenv("CHROME_PATH")
	if chrome == "" {
		t.Skip("Chrome unavailable")
	}
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app", web.Handler()))
	mux.Handle("/app/mu.css", http.StripPrefix("/app", app.Serve()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/" || r.Header.Get("Micro-Protocol") != "mucp/1" || r.Header.Get("Micro-Service") != "assistant" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Micro-Protocol", "mucp/1")
		w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("Micro-Endpoint") {
		case "Assistant.ListAgents":
			json.NewEncoder(w).Encode(map[string]any{"agents": []any{map[string]string{"id": "micro", "slug": "micro", "name": "Micro", "description": "A personal assistant"}}})
		case "Assistant.ListThreads":
			json.NewEncoder(w).Encode(map[string]any{"threads": []any{map[string]string{"id": "own-thread", "agentId": "micro", "subject": "Browser conversation"}}})
		case "Assistant.ReadThread":
			json.NewEncoder(w).Encode(map[string]any{"thread": map[string]string{"id": "own-thread", "agentId": "micro", "subject": "Browser conversation"}, "messages": []any{map[string]string{"id": "one", "role": "ROLE_AGENT", "text": "**Recovered** reply"}}})
		case "Assistant.Ask":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["mode"] != "ACCESS_MODE_PERSONAL" || body["agentId"] != "micro" {
				t.Error("wrong request scope")
			}
			calls.Add(1)
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
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for _, width := range []int64{320, 390, 1440} {
		var valid bool
		err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(width, 900, 1, width < 500), chromedp.Navigate(server.URL+"/app/"), chromedp.WaitVisible("#chat"), chromedp.SendKeys("#prompt", "Weather please\r"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`document.querySelector('.answer strong').textContent==='Micro' && document.querySelector('.message-body strong').textContent==='Forecast' && !window.pwned && !document.querySelector('a[href^="javascript:"]') && document.querySelector('#responses').textContent.includes('Future widget') && document.documentElement.scrollWidth<=innerWidth && document.querySelector('#send').getBoundingClientRect().bottom<=innerHeight && document.querySelector('#send').getBoundingClientRect().top>document.querySelector('.answer').getBoundingClientRect().top`, &valid))
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
	err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/app/?view=chats"), chromedp.WaitVisible(".collection-item"), chromedp.Click(".collection-item"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`location.pathname==='/app/' && document.querySelector('#responses').textContent.includes('Recovered')`, &valid))
	if err != nil || !valid {
		t.Fatalf("thread history: %v", err)
	}
	err = chromedp.Run(ctx, chromedp.Click("#new-chat"), chromedp.SendKeys("#prompt", "Pending\r"), chromedp.WaitVisible(".answer"), chromedp.Evaluate(`location.pathname==='/app/' && document.querySelector('#responses').textContent.includes('Recovered')`, &valid))
	if err != nil || !valid {
		t.Fatalf("pending recovery: %v", err)
	}
	// No runtime-rendered HTML endpoint exists in the fixture: every page is static.
	if strings.Contains(server.URL, "/assistant") {
		t.Fatal("unexpected old route")
	}
}
