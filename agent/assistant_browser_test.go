package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"mu/agent"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// Exercise the real client HTML, stylesheet and command script with a local
// queued-response fixture. No model or external service is called.
func TestFocusedAssistantBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser gate runs separately")
	}
	chrome := os.Getenv("CHROME_PATH")
	if chrome == "" {
		t.Skip("Chrome unavailable")
	}
	const owner = "focused-browser-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	chat := thread.Open(owner, thread.WebClient, "focused-browser-chat")
	thread.Name(owner, chat.ID, "Browser conversation")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mu.css", "/mu.js":
			http.ServeFile(w, r, filepath.Join("../internal/app/html", r.URL.Path[1:]))
		case "/session":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"account": owner, "type": "user"})
		case "/agent":
			if r.Method != "POST" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"thread": chat.ID, "title": "Browser conversation"})
		case "/agent/pending":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"waiting": false, "answer_html": "<p><strong>Forecast</strong></p><ul><li>Clear skies</li></ul>"})
		default:
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			agent.AssistantHandler(w, r)
		}
	}))
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
		err := chromedp.Run(ctx,
			emulation.SetDeviceMetricsOverride(width, 844, 1, width < 700),
			chromedp.Navigate(server.URL+"/assistant/micro?new=1"),
			chromedp.WaitVisible("#command-input"),
			chromedp.Evaluate(`document.documentElement.scrollWidth <= innerWidth && !document.querySelector('.runtime-navigation') && document.querySelector('#send').getBoundingClientRect().bottom <= innerHeight && [...document.querySelectorAll('header a')].every(a=>{const r=a.getBoundingClientRect();return r.left>=0 && r.right<=innerWidth})`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("empty client layout at %d", width)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("assistant-empty-%d.png", width)), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
		err = chromedp.Run(ctx,
			chromedp.SendKeys("#command-input", "Weather please"),
			chromedp.Evaluate(`document.querySelector('#command-form').requestSubmit()`, nil),
			chromedp.Poll(`!!document.querySelector('#responses .answer ul')`, nil),
			chromedp.Evaluate(`location.pathname==='/assistant/micro' && document.querySelector('#responses .answer strong').textContent==='Forecast' && document.documentElement.scrollWidth<=innerWidth && document.querySelector('#send').getBoundingClientRect().bottom<=innerHeight`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("populated client at %d", width)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("assistant-%d.png", width)), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
		err = chromedp.Run(ctx, chromedp.Click(`header a[href="/assistant?view=chats"]`), chromedp.WaitVisible(".collection-list"), chromedp.Evaluate(`document.querySelector('.collection-list').textContent.includes('Browser conversation') && !document.querySelector('.runtime-navigation')`, &valid))
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatal("chat navigation")
		}
	}
}
