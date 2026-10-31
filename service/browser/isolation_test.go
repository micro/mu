package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

func TestBrowserProcessIsolationAndPrivateNetwork(t *testing.T) {
	if binary() == "" {
		t.Skip("local Chromium unavailable")
	}
	var hits atomic.Int32
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer private.Close()
	first, closeFirst, err := tab(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFirst()
	if err := chromedp.Run(first, network.SetCookie("audit", "secret").WithDomain("example.com").WithPath("/")); err != nil {
		t.Fatal(err)
	}
	second, closeSecond, err := tab(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer closeSecond()
	var result string
	err = chromedp.Run(second, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, e := storage.GetCookies().Do(ctx)
		if e != nil {
			return e
		}
		for _, c := range cookies {
			if c.Name == "audit" {
				t.Error("cookie crossed browser instances")
			}
		}
		return nil
	}), chromedp.Navigate("data:text/html,<html><body>network test</body></html>"), chromedp.Evaluate(`fetch(`+strconv.Quote(private.URL)+`,{mode:'no-cors'}).then(()=>"loaded",()=>"blocked")`, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }))
	if err != nil {
		t.Fatal(err)
	}
	// A no-cors fetch can resolve with an opaque proxy-denial response.
	// The security invariant is that the private destination is never reached.
	if hits.Load() != 0 {
		t.Fatalf("private request escaped proxy: hits=%d result=%s", hits.Load(), result)
	}
}
