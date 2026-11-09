package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"mu/internal/auth"
)

func TestResponsiveRuntimeNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("browser UI test runs separately from the short backend suite")
	}
	browser := os.Getenv("CHROME_PATH")
	if browser != "" {
		if _, err := os.Stat(browser); err != nil {
			t.Fatalf("configured Chrome: %v", err)
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if browser != "" {
			break
		}
		if path, err := exec.LookPath(name); err == nil {
			browser = path
			break
		}
	}
	if browser == "" {
		t.Skip("Chrome unavailable")
	}
	css, err := os.ReadFile("html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile("html/mu.js")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mu.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write(css)
		case "/mu.js":
			w.Header().Set("Content-Type", "text/javascript")
			w.Write(js)
		default:
			w.Header().Set("Content-Type", "text/html")
			body := `<h1>Home</h1><div class="section-stack"><section class="record-card"><h2>Agents</h2><p>Micro</p><p>A personal assistant</p></section><section class="record-card"><h2>Inbox</h2><p>No unread messages.</p></section><section class="record-card"><h2>Scheduled</h2><p>Morning Brief</p></section></div>`
			title := "Home"
			if r.URL.Path == "/agents" {
				title = "Agents"
				body = `<h1>Agents</h1><p>Choose an agent to start a conversation.</p>`
			}
			if r.URL.Path == "/agent/micro" {
				title = "Micro"
				body = `<div class="assistant-workspace"><div class="conversation is-active"><h1>Weather</h1><div id="responses" role="log"><section class="turn"><div class="answer"><p>Clear skies this afternoon.</p></div></section></div><div class="prompt-panel"><div class="prompt-welcome"><h1>Micro</h1></div><form id="command-form" data-account="navigation-test" data-path="/agent/micro"><label class="sr-only" for="command-input">Message</label><div class="composer"><textarea id="command-input" rows="1" placeholder="What do you need?"></textarea><button id="send">Send</button></div><p id="status"></p></form></div></div></div>`
			}
			w.Write([]byte(ConsoleHTML(title, body, &auth.Account{ID: "navigation-test"}, r.URL.Path)))
		}
	}))
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser),
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.WSURLReadTimeout(60*time.Second),
		chromedp.CombinedOutput(os.Stderr))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	capture := func(name string) {
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name+".png"), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, width := range []int64{320, 390, 900, 1440} {
		var valid bool
		err := chromedp.Run(ctx,
			emulation.SetDeviceMetricsOverride(width, 844, 1, width <= 700),
			chromedp.Navigate(server.URL+"/home"),
			chromedp.WaitVisible(".runtime-navigation"),
			chromedp.Evaluate(`(()=>{const nav=document.querySelector('.runtime-navigation'),r=nav.getBoundingClientRect();return document.querySelectorAll('.runtime-navigation a').length===4 && !document.querySelector('.mobile-menu-toggle,.mobile-drawer') && document.querySelector('.runtime-navigation [aria-current="page"]').textContent==='Home' && document.documentElement.scrollWidth<=innerWidth && (innerWidth<=700 ? Math.abs(r.bottom-innerHeight)<1 && r.width===innerWidth && parseFloat(getComputedStyle(document.body).paddingBottom)>=r.height : r.width===88 && document.querySelector('.page').getBoundingClientRect().left===88)})()`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("navigation layout at %d", width)
		}
		capture(fmt.Sprintf("runtime-home-%d", width))
		err = chromedp.Run(ctx,
			chromedp.Click(`.runtime-navigation a[href="/agents"]`),
			chromedp.Poll(`document.querySelector('.runtime-navigation [aria-current="page"]').textContent==='Agents'`, nil),
			chromedp.Navigate(server.URL+"/agent/micro"),
			chromedp.WaitVisible("#command-input"),
			chromedp.Evaluate(`(()=>{const box=document.querySelector('#command-form').getBoundingClientRect(),nav=document.querySelector('.runtime-navigation').getBoundingClientRect();return box.bottom<=(innerWidth<=700?nav.top:innerHeight) && box.left>=(innerWidth<=700?0:nav.right) && box.right<=innerWidth})()`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("navigation covers composer at %d", width)
		}
		capture(fmt.Sprintf("runtime-chat-%d", width))
	}
}
