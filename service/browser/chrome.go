package browser

// Driving the browser.
//
// One allocator, shared, and a fresh tab per request. Starting a browser costs
// a second and a few hundred megabytes; starting one per call would make the
// price of this service the price of process creation. A tab is cheap and is
// the isolation that matters — its own cookies, its own storage, nothing
// carried from whoever asked last.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"mu/internal/app"
	"mu/internal/blob"
	"mu/internal/origin"
)

var (
	allocOnce   sync.Once
	allocCtx    context.Context
	allocCancel context.CancelFunc
)

// allocator is the browser, started or connected to on first use.
//
// Lazily, because most instances have no browser configured and every one of
// them would otherwise pay for the attempt at boot. It is never torn down: the
// process holding it is the server, and a browser that exits between requests
// is one that has to start again on the next.
func allocator() context.Context {
	allocOnce.Do(func() {
		if url := endpoint(); url != "" {
			// Somebody else's browser, over the wire. This is what keeps Mu a
			// single binary: the dependency is a network address rather than a
			// program that has to be on this disk.
			allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), url)
			app.Log("browser", "using the DevTools endpoint at %s", url)
			return
		}
		// binary() is either what an operator named or what was found on the
		// PATH, and it is never empty here — Configured() gates every path in.
		// Passing an empty ExecPath would defeat chromedp's own search, which
		// is what this was doing before it looked for one itself.
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(binary()),
			// Headless, and told not to give itself away as automation any more
			// than it has to. Not evasion — a site that refuses robots should be
			// able to refuse this one — but the default flags trip protections on
			// sites that are perfectly happy to be read.
			chromedp.Flag("headless", "new"),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", false),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.WindowSize(viewWide, viewTall),
		)
		allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
		app.Log("browser", "using the Chromium at %s", binary())
	})
	return allocCtx
}

// Close shuts a locally started browser down.
//
// Not called by anything yet, and that is worth saying rather than hiding: this
// server has no shutdown hook to hang it on, so a Chromium started by
// CHROME_PATH is cleaned up by the operating system when the process goes
// rather than by us. It is here because the cancel function has to be kept
// somewhere for that to ever be possible, and because the alternative — a
// package-level cancel nobody can reach — is worse. A BROWSER_URL instance has
// nothing to close: the browser is somebody else's process.
func Close() {
	if allocCancel != nil {
		allocCancel()
	}
}

// viewWide and viewTall are the window every page is opened in.
//
// A desktop shape, because a page rendered at phone width is a different page —
// half the sites on the web serve a cut-down layout below 768 and an agent
// reading one gets the cut-down answer without being told.
const (
	viewWide = 1280
	viewTall = 900
)

// tab is one page's context, and the function that gives it back.
func tab(parent context.Context) (context.Context, context.CancelFunc) {
	// The caller's own deadline still applies — an HTTP request that went away
	// should not leave a browser working on it — and the shorter of the two
	// wins, which is what taking the caller's first and then layering the page
	// timeout on top gives.
	//
	// The allocator is the parent of the browser context on purpose: chromedp
	// hangs its browser off whatever it is given, and rooting a tab in a
	// request context would take the browser down with the request.
	ctx, cancelTab := chromedp.NewContext(allocator())
	cancels := []context.CancelFunc{cancelTab}
	if parent != nil {
		if deadline, ok := parent.Deadline(); ok {
			var cancelCaller context.CancelFunc
			ctx, cancelCaller = context.WithDeadline(ctx, deadline)
			cancels = append(cancels, cancelCaller)
		}
	}
	ctx, cancelWait := context.WithTimeout(ctx, wait)
	cancels = append(cancels, cancelWait)

	return ctx, func() {
		for i := len(cancels) - 1; i >= 0; i-- {
			cancels[i]()
		}
	}
}

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
	ctx, done := tab(parent)
	defer done()

	var out page
	err := chromedp.Run(ctx,
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
	ctx, done := tab(parent)
	defer done()

	var buf []byte
	var title string
	grab := chromedp.CaptureScreenshot(&buf)
	if full {
		grab = chromedp.FullScreenshot(&buf, shotQuality)
	}
	err := chromedp.Run(ctx,
		emulation.SetUserAgentOverride(userAgent),
		chromedp.Navigate(target),
		settle(selector),
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
		return fmt.Errorf("this instance cannot reach its browser — an admin should check BROWSER_URL")
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
