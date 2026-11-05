package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"mu/internal/auth"
)

func TestMobileNavigationDrawer(t *testing.T) {
	browser := ""
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
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
			w.Write([]byte(ConsoleHTML("Home", "<h1>Home</h1>", &auth.Account{ID: "drawer-test"}, "/home")))
		}
	}))
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var valid bool
	err = chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(390, 844, 1, true),
		chromedp.Navigate(server.URL+"/home"),
		chromedp.WaitVisible(".mobile-menu-toggle"),
		chromedp.Click(".mobile-menu-toggle"),
		chromedp.Poll(`document.querySelector('.mobile-drawer').open`, nil),
		chromedp.Evaluate(`document.querySelector('.mobile-drawer').contains(document.activeElement) && document.querySelectorAll('.runtime-navigation a').length === 4 && document.querySelector('.runtime-navigation [aria-current="page"]').textContent === 'Home'`, &valid),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("drawer focus/navigation")
	}
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`history.back()`, nil),
		chromedp.Poll(`!document.querySelector('.mobile-drawer').open`, nil),
		chromedp.Evaluate(`document.activeElement === document.querySelector('.mobile-menu-toggle')`, &valid),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("focus not restored")
	}
	err = chromedp.Run(ctx,
		chromedp.Click(".mobile-menu-toggle"),
		chromedp.MouseClickXY(380, 200),
		chromedp.Poll(`!document.querySelector('.mobile-drawer').open`, nil),
		emulation.SetDeviceMetricsOverride(1280, 900, 1, false),
		chromedp.Poll(`document.querySelector('.runtime-navigation').parentElement === document.body`, nil),
		chromedp.Evaluate(`getComputedStyle(document.querySelector('.mobile-menu-toggle')).display === 'none' && getComputedStyle(document.querySelector('.runtime-navigation')).display !== 'none'`, &valid),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("desktop navigation not restored")
	}
}
