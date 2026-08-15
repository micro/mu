package server

// Small things the server needs and nothing else does.

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/agent"
	"mu/internal/ai"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/version"
	"mu/service/blog"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/prayer"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/video"
)

func argFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		var f float64
		fmt.Sscanf(n, "%g", &f)
		return f
	}
	return 0
}

// setSecurityHeaders applies the site-wide response headers. Handlers that
// need something stricter (the app sandbox at /apps/run) overwrite these.
//
// The script-src allows 'unsafe-inline' because the UI is built from server
// generated markup with inline handlers and styles throughout; tightening that
// means moving to nonces across every handler, which is a separate piece of
// work. So this is not an XSS backstop — escaping at the point of output is.
// What it does buy, cheaply: no attacker-hosted script can be pulled in, no
// plugins, no injected <base> to repoint relative URLs, no framing by another

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		// The page template pulls Nunito Sans from Google Fonts.
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' data: https://fonts.gstatic.com",
		"img-src 'self' data: blob: https:",
		"media-src 'self' data: blob: https:",
		"connect-src 'self'",
		// YouTube is the one third-party frame: /video embeds the player.
		"frame-src 'self' https://www.youtube.com https://www.youtube-nocookie.com",
		"object-src 'none'",
		"base-uri 'self'",
		// Stripe Checkout is the one place a form leaves this origin, and it
		// leaves by redirect: the form posts to /account/stripe/checkout, which
		// answers 303 to a checkout URL Stripe has just minted. form-action is
		// enforced against the *redirect target*, not only against the action
		// attribute, so 'self' alone blocked the POST that had already been
		// accepted — the console said the form violated 'self' while pointing
		// at a URL on this host, which reads like a browser bug and is not one.
		//
		// Named rather than wildcarded. checkout.stripe.com is where a session
		// lands; a wildcard would also cover anything else Stripe ever hosts, and
		// the point of this directive is to enumerate where a form may take a
		// person.
		"form-action 'self' https://checkout.stripe.com",
		"frame-ancestors 'self'",
	}, "; "))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

// updatesHandler serves GET /updates?since=<unix> — a single lightweight
// endpoint the client polls for change counts. Returns JSON:
//
//	{"mail":3,"status":2,"social":1,"ts":1713254400}
//
// The client stores ts and sends it back on the next poll. If since is
// omitted, returns current totals (unread mail, stream size, etc.).
func updatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var viewerID string
	if sess, _ := auth.TrySession(r); sess != nil {
		viewerID = sess.Account
	}

	now := time.Now()

	// Parse the "since" parameter — unix timestamp.
	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		var n int64
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			since = time.Unix(n, 0)
		}
	}

	result := map[string]interface{}{
		"ts": now.Unix(),
	}

	// Mail — always unread count (personal, independent of since).
	if viewerID != "" {
		result["mail"] = mail.GetUnreadCount(viewerID)
	} else {
		result["mail"] = 0
	}

	if since.IsZero() {
		result["social"] = 0
		result["stream"] = 0
	} else {
		result["social"] = social.CountSince(since)
		result["stream"] = stream.CountSince(since)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(result)
}

// chargedWriteOp maps a request method + path to the wallet operation
// that should be charged. Returns "" for routes that don't cost credits
// (reads, auth, payments, MCP — MCP has its own QuotaCheck). This is

// the SINGLE source of truth for what costs money on the web/API side.
func chargedWriteOp(r *http.Request) string {
	if r.Method != "POST" {
		return ""
	}
	path := r.URL.Path
	switch {
	// Social threads and replies
	case path == "/social":
		return quota.OpSocialPost
	case path == "/social/thread":
		return quota.OpSocialReply
	// Blog — only CREATE is charged (no id param). Updates are free.
	case path == "/blog" && r.URL.Query().Get("id") == "":
		return quota.OpBlogCreate
	case strings.HasPrefix(path, "/blog/post/") && strings.HasSuffix(path, "/comment"):
		return quota.OpBlogComment
	// Apps
	case path == "/apps/new":
		return quota.OpAppCreate
	case path == "/apps/generate":
		return quota.OpAppBuild
	case strings.HasPrefix(path, "/apps/") && strings.HasSuffix(path, "/ai-edit"):
		return quota.OpAppEdit
	// Stream (console)
	case path == "/stream":
		return quota.OpStreamPost
	}
	return ""
}

// serveListener returns the TCP listener to serve on. When the process is
// started via systemd socket activation (LISTEN_PID / LISTEN_FDS point at us),
// it adopts the inherited listening socket instead of binding its own. That
// lets the socket outlive the process across a redeploy: while the binary is
// being restarted the kernel keeps accepting and queuing connections on the
// held socket, so nginx sees a moment of latency rather than a connection
// refusal (502). Without activation it just binds addr as before.
//
// systemd passes activated fds starting at 3 (SD_LISTEN_FDS_START); Mu uses a

// single web socket, so fd 3 is the one.
func serveListener(addr string) (net.Listener, bool, error) {
	if pid, err := strconv.Atoi(os.Getenv("LISTEN_PID")); err == nil && pid == os.Getpid() {
		if n, err := strconv.Atoi(os.Getenv("LISTEN_FDS")); err == nil && n >= 1 {
			f := os.NewFile(uintptr(3), "systemd-mu-socket")
			if ln, err := net.FileListener(f); err == nil {
				return ln, true, nil
			}
			// Fall through to binding ourselves if the fd wasn't usable.
		}
	}
	ln, err := net.Listen("tcp", addr)
	return ln, false, err
}

// isServerMode returns true when the argument list contains the
// `--serve` flag. This is the single signal that switches between the
// server and CLI entry points — kept deliberately simple so it can't

// versionInfo reports the running build and how the system is wired, so a
// deploy can be verified with `curl micro.mu/version`.
func versionInfo() map[string]any {
	info := map[string]any{
		"version":  version.String(), // release version (tag), or dev+commit
		"build":    app.Version,      // per-process id (start time), for cache busting
		"go":       runtime.Version(),
		"agent":    agent.Mode(),       // "native" (go-micro agent) or "planner"
		"mcp":      "go-micro/gateway", // /mcp served by go-micro's gateway
		"services": service.Services(), // in-process go-micro services
		"go_micro": "unknown",
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info["commit"] = s.Value
			case "vcs.time":
				info["commit_time"] = s.Value
			case "vcs.modified":
				info["dirty"] = s.Value == "true"
			}
		}
		for _, dep := range bi.Deps {
			if dep.Path == "go-micro.dev/v6" {
				info["go_micro"] = dep.Version
			}
		}
	}
	return info
}

func runHealthChecks() []app.ServiceHealth {
	type result struct {
		index int
		check app.ServiceHealth
	}

	// Probes for services where there is a cheap signal that the thing actually
	// works. Anything without one is reported on being registered and reachable,
	// which is still a real check — the service failed to start otherwise.
	probes := map[string]func() bool{
		"news":    func() bool { return len(news.GetFeed()) > 0 },
		"blog":    func() bool { return blog.Topics() != nil },
		"video":   func() bool { return video.LatestVideos(1) != nil },
		"markets": func() bool { return len(markets.AllPrices()) > 0 },
		"social":  func() bool { return len(social.Threads()) > 0 },
		// No probe for mail. It had one — ConfiguredDomain() != "" — which
		// could never be false, because that function answers "localhost" when
		// nothing is set. Replacing it with a real domain check would be worse:
		// an instance with no mail server still has a working inbox and private
		// messaging, so it would report a healthy service as broken. Registered
		// and serving is the honest check here.
		"prayer": func() bool { return prayer.GetReminderData() != nil },
		"search": func() bool { return settings.Get("BRAVE_API_KEY") != "" },
	}
	// Services with no page of their own.
	noPage := map[string]bool{"index": true}

	type check struct {
		name string
		path string
		fn   func() bool
	}
	var checks []check

	// Derived from the registry rather than hardcoded: the previous list had
	// drifted to 7 of 15 services, so most of what mu runs was unreported.
	svcs := service.Services()
	sort.Strings(svcs)
	for _, name := range svcs {
		path := "/" + name
		if noPage[name] {
			path = ""
		}
		fn := probes[name]
		if fn == nil {
			fn = func() bool { return true } // registered and serving
		}
		// The service's own label, not a capitalised id — otherwise /status is
		// the one page in the product that calls the database "Db".
		checks = append(checks, check{name: service.Label(name), path: path, fn: fn})
	}

	// Cross-cutting checks that aren't domain services.
	checks = append(checks,
		// Whether it answers, not whether a key is set. See internal/ai/health.go.
		check{"Agent", "/agent", func() bool { ok, _ := ai.Healthy(); return ok }},
		// Named for what the reader is being told, not for the library behind
		// it — /status is linked from every page footer.
		check{"runtime", "/version", func() bool { return len(service.Services()) > 0 }},
	)

	results := make([]app.ServiceHealth, len(checks))
	ch := make(chan result, len(checks))

	for i, c := range checks {
		go func(idx int, name, path string, fn func() bool) {
			ok := fn()
			ch <- result{idx, app.ServiceHealth{Name: name, Status: ok, Path: path}}
		}(i, c.name, c.path, c.fn)
	}

	for range checks {
		r := <-ch
		results[r.index] = r.check
	}

	return results
}

// argString reads an optional string argument from a tool call.
func argString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}
