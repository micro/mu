package browser

// Driving the browser.
//
// Browser state and network access are isolated for each request.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"mu/internal/blob"
	"mu/internal/origin"
)

// Each request owns a fresh local browser and a checked egress proxy. Remote
// browsers cannot inherit this host's network boundary and are refused.
const (
	viewWide = 1280
	viewTall = 900
)

func tab(parent context.Context) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(parent, wait)
	proxy, stop, err := startProxy(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(binary()),
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("proxy-server", proxy),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("disable-quic", true),
		chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"),
		chromedp.WindowSize(viewWide, viewTall),
	)
	alloc, closeAlloc := chromedp.NewExecAllocator(ctx, opts...)
	page, closePage := chromedp.NewContext(alloc)
	return page, func() { closePage(); closeAlloc(); stop(); cancel() }, nil
}

// Close is retained for lifecycle callers; each request owns its browser.
func Close() {}

// page is what came back.
type page struct {
	Title string
	Text  string
	URL   string
}

// settle is what to run after navigating: wait for a selector when one was
// asked for, and otherwise give the page a moment past the load event.
//
// The moment is not a guess about the network — chromedp already waited for
// load. It is for the frame after, where a script that ran on load puts its
// content in. Without it a page reads as its own skeleton, which is the exact
// failure this service exists to fix.
func settle(selector string) chromedp.Action {
	if selector != "" {
		return chromedp.WaitVisible(selector, chromedp.ByQuery)
	}
	return chromedp.Sleep(settleFor)
}

const settleFor = 750 * time.Millisecond

// read opens a page and returns its text.
func read(parent context.Context, target, selector string) (page, error) {
	ctx, done, err := tab(parent)
	if err != nil {
		return page{}, err
	}
	defer done()

	var out page
	err = chromedp.Run(ctx,
		emulation.SetUserAgentOverride(userAgent),
		chromedp.Navigate(target),
		settle(selector),
		chromedp.Title(&out.Title),
		chromedp.Location(&out.URL),
		// innerText, not the HTML. It is what a person sees: script and style
		// contents are gone, and so is every attribute, which is most of the
		// bytes and none of the meaning.
		chromedp.Evaluate(`document.body ? document.body.innerText : ""`, &out.Text),
	)
	if err != nil {
		return page{}, browserErr(err)
	}

	out.Text = tidy(out.Text)
	if len(out.Text) > maxText {
		out.Text = out.Text[:maxText] + "\n\n[…truncated]"
	}
	return out, nil
}

// shot is a picture that was taken and where it went.
//
// Both addresses, because they are for different readers. A tool answers an
// agent that may be on another machine and needs the whole URL; the page is
// already on this origin and a relative path is one fewer thing that can be
// wrong behind a proxy.
type shot struct {
	URL   string
	Path  string
	Title string
}

// capture photographs a page and stores the picture.
func capture(parent context.Context, target, selector string, full bool) (shot, error) {
	ctx, done, err := tab(parent)
	if err != nil {
		return shot{}, err
	}
	defer done()

	var buf []byte
	var title string
	grab := chromedp.CaptureScreenshot(&buf)
	if full {
		grab = chromedp.FullScreenshot(&buf, shotQuality)
	}
	err = chromedp.Run(ctx,
		emulation.SetUserAgentOverride(userAgent),
		emulation.SetDeviceMetricsOverride(viewWide, viewTall, 1, false),
		chromedp.Navigate(target),
		settle(selector),
		chromedp.Evaluate(`(async () => {
          await Promise.race([Promise.all([
            document.fonts ? document.fonts.ready : Promise.resolve(),
            ...Array.from(document.images).filter(i => i.getBoundingClientRect().top < innerHeight).map(i => i.decode ? i.decode().catch(() => {}) : Promise.resolve())
          ]), new Promise(resolve => setTimeout(resolve, 3000))]);
          await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
          return true;
        })()`, nil, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }),
		chromedp.Title(&title),
		grab,
	)
	if err != nil {
		return shot{}, browserErr(err)
	}
	if len(buf) == 0 {
		return shot{}, fmt.Errorf("the browser took an empty picture of that page")
	}

	// Keyed by what was asked for, so the same request twice does not store the
	// same bytes twice — and so the URL is guessable only by somebody who
	// already knows the page and the shape. See ShotHandler.
	key := shotKey(target, full)
	if err := blob.Put(key, buf, "image/png"); err != nil {
		return shot{}, fmt.Errorf("the picture could not be stored: %w", err)
	}
	path := "/browser/shot/" + strings.TrimPrefix(key, shotPrefix)
	return shot{URL: origin.Self() + path, Path: path, Title: title}, nil
}

// shotQuality is the JPEG quality chromedp asks for on a full-page capture. It
// is a PNG either way; the argument only matters for a format we do not use.
const shotQuality = 90

// shotPrefix is where pictures live in the blob store.
const shotPrefix = "browser/shots/"

// shotKey names a picture by the page it is of.
func shotKey(target string, full bool) string {
	sum := sha256.Sum256([]byte(target + "|" + fmt.Sprint(full)))
	return shotPrefix + hex.EncodeToString(sum[:16]) + ".png"
}

// userAgent is what this browser says it is.
//
// A current Chrome string, and honestly so — it *is* Chromium. The default
// headless string names itself HeadlessChrome, which a good number of sites
// refuse outright, including ones with no objection to being read.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// browserErr turns a driver failure into something worth reading.
//
// The raw errors are about websockets and contexts, which describe our
// plumbing rather than the caller's problem. An agent that gets "context
// deadline exceeded" learns nothing it can act on.
func browserErr(err error) error {
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "context deadline exceeded"),
		strings.Contains(err.Error(), "context canceled"):
		return fmt.Errorf("that page did not finish loading within %s", wait)
	case strings.Contains(err.Error(), "connection refused"),
		strings.Contains(err.Error(), "websocket"):
		return fmt.Errorf("this instance cannot start its local browser — an admin should check CHROME_PATH")
	case strings.Contains(err.Error(), "exec:"), strings.Contains(err.Error(), "no such file"):
		return fmt.Errorf("this instance cannot start its browser — an admin should check CHROME_PATH")
	}
	return fmt.Errorf("the browser could not open that page: %w", err)
}

// tidy squeezes the blank lines out of innerText.
//
// A rendered page has runs of empty lines wherever a layout element had no
// text in it, and there are a lot of those. Left in, they are a third of what
// a model is charged to read.
func tidy(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
