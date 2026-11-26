package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"mu/internal/app"
	"mu/internal/auth"
)

// Run with CHROME_PATH and optionally UI_SCREENSHOTS. Installation still needs
// an Android/device check; this verifies the actual shared script and layout.
func TestAgentAppBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser flow runs separately")
	}
	browser := os.Getenv("CHROME_PATH")
	if browser == "" {
		t.Skip("Chrome not configured")
	}
	a, tokens := appFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Hello from Teacup."},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	t.Setenv("AI_PROVIDER", "local")
	t.Setenv("OPENAI_BASE_URL", upstream.URL)
	t.Setenv("OPENAI_MODEL", "app-browser")
	t.Setenv("AGENT_MODEL", "")
	t.Setenv("GUEST_MODEL", "")
	static := app.Serve()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "session", Value: tokens["alice"]})
		auth.SetCSRFCookie(w, r)
		if r.URL.Path == "/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"account": tokens["aliceID"], "type": "user"})
			return
		}
		if r.URL.Path == "/mu.js" || r.URL.Path == "/mu.css" {
			static.ServeHTTP(w, r)
			return
		}
		Handler(w, r)
	}))
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, width := range []int64{390, 1440} {
		if err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(width, 900, 1, width < 700), chromedp.Navigate(server.URL+appPath(a)), chromedp.WaitVisible(`[data-app-chat]`)); err != nil {
			t.Fatal(err)
		}
		var fits bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.scrollWidth<=innerWidth`, &fits)); err != nil || !fits {
			t.Fatal("horizontal overflow", width, err)
		}
		if err := chromedp.Run(ctx, chromedp.SendKeys(`#app-prompt`, "Hello Teacup"), chromedp.Click(`[data-app-chat] button`), chromedp.WaitVisible(`[data-app-messages] article`)); err != nil {
			t.Fatal(err)
		}
		var text string
		if err := chromedp.Run(ctx, chromedp.Text(`[data-app-messages]`, &text)); err != nil || !strings.Contains(text, "Hello from Teacup") {
			t.Fatal("chat round trip", text, err)
		}
		var manifest string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('link[rel=manifest]').getAttribute('href')`, &manifest)); err != nil || manifest != appPath(a)+"manifest.webmanifest" {
			t.Fatal("manifest", manifest, err)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var png []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("agent-app-%d.png", width)), png, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
