package server

// The request path every call goes through, and the lifecycle around it.
//
// Security headers, CSRF, the central write gate and the x402 payment gate all
// live here rather than in individual handlers, so none of them can be
// forgotten by a new page — the reason they were centralised in the first
// place.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"mu/home"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/setup"
	"mu/internal/thread"
	"mu/internal/usage"
	"mu/internal/x402"
	"mu/service/blog"
	"mu/service/chat"
	"mu/service/mail"
	"mu/service/wallet"
)

// serve builds the handler and runs the server until interrupted.
func serve(addr string) {
	// Resolved once rather than per request: the auth map and the static
	// suffixes are fixed for the life of the process.
	authenticated := authRequired()
	staticPaths := staticSuffixes()

	// Create server with handler
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Block known bot paths silently
			if strings.HasPrefix(r.URL.Path, "/audio/") {
				http.NotFound(w, r)
				return
			}

			setSecurityHeaders(w)

			// Set Onion-Location header for Tor Browser discovery
			if onion := os.Getenv("TOR_ONION"); onion != "" {
				w.Header().Set("Onion-Location", "http://"+onion+r.URL.RequestURI())
			}

			// Request logging (Apache-style)
			start := time.Now()
			defer func() {
				// Skip logging for static assets and frequent endpoints
				if !strings.HasSuffix(r.URL.Path, ".css") &&
					!strings.HasSuffix(r.URL.Path, ".js") &&
					!strings.HasSuffix(r.URL.Path, ".png") &&
					!strings.HasSuffix(r.URL.Path, ".ico") &&
					!strings.HasPrefix(r.URL.Path, "/chat/ws") {
					app.Log("http", "%s %s %s %v", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
				}
			}()

			if Env == "dev" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Access-Control-Allow-Credentials", "true")

				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
			}

			if v := len(r.URL.Path); v > 1 && strings.HasSuffix(r.URL.Path, "/") {
				r.URL.Path = r.URL.Path[:v-1]
			}

			// One mark per browser, before anything asks who this is.
			//
			// Here rather than at each gate, so nothing downstream needs a
			// ResponseWriter to find out who it is talking to, and so the very
			// first request of a visit is already marked — on a front page that
			// answers a question on arrival, that is most first questions.
			//
			// Above the static fast path on purpose: an image is not a call and
			// is not counted, but it may well be the first thing a browser
			// fetches, and a mark handed out there is one the page below it
			// already has. See app.MarkClient for what this is and is not.
			app.MarkClient(w, r)

			// Fast path for static assets - skip all middleware
			for _, ext := range staticPaths {
				if strings.HasSuffix(r.URL.Path, ext) {
					http.DefaultServeMux.ServeHTTP(w, r)
					return
				}
			}

			// Count the request. Both tool doors are counted per tool instead,
			// inside the dispatcher: for /mcp because every call is a POST to
			// the same path so the path says nothing, and for /api/v1/ because
			// counting here as well would file every call twice — once as a
			// path and once as the tool it ran. Assets and polling endpoints
			// are noise — see internal/usage.
			if !api.ToolDispatch(r.URL.Path) && !usage.Skipped(r.URL.Path) {
				account := ""
				if _, acc := auth.TrySession(r); acc != nil {
					account = acc.ID
				}
				usage.Record("web", usage.Endpoint(r.URL.Path), account)
			}

			var token string

			// set via session cookie
			if c, err := r.Cookie("session"); err == nil && c != nil {
				token = c.Value
			}

			// Try Authorization header (Bearer token or PAT)
			if token == "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "" {
					// Support both "Bearer <token>" and just "<token>"
					if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
						token = authHeader[7:]
					} else {
						token = authHeader
					}
				}
			}

			// Try X-Micro-Token header (legacy support)
			if token == "" {
				token = r.Header.Get("X-Micro-Token")
			}

			// Check if static asset - skip authentication entirely
			isStaticAsset := false
			for _, ext := range staticPaths {
				if strings.HasSuffix(r.URL.Path, ext) {
					isStaticAsset = true
					break
				}
			}

			// Skip auth check for static assets
			if !isStaticAsset {
				var isAuthed bool

				// Check if path requires authentication
				{
					for url, authed := range authenticated {
						if strings.HasPrefix(r.URL.Path, url) {
							isAuthed = authed
							break
						}
					}
				}

				// check token
				if isAuthed {
					// deny access if invalid
					if err := auth.ValidateToken(token); err != nil {
						// Allow x402 payment as alternative to auth for API requests
						if x402.Enabled() && x402.HasPayment(r) && (app.SendsJSON(r) || app.WantsJSON(r)) {
							r = r.WithContext(context.WithValue(r.Context(), x402.X402ContextKey, true))
						} else if app.SendsJSON(r) || app.WantsJSON(r) {
							// Return JSON 401 for API-style requests
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusUnauthorized)
							w.Write([]byte(`{"error":"Authentication required"}`))
							return
						} else {
							// To the sign-in page, carrying where they were
							// going — not to the landing page, which is the
							// pitch for a product they have already bought and
							// which loses the destination. Signing in should
							// finish the thing you were doing.
							//
							// RequestURI is path and query, always relative, so
							// this cannot be turned into an open redirect; the
							// login side re-checks with safeRedirect anyway.
							http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.RequestURI()),
								http.StatusSeeOther)
							return
						}
					}
				} else if r.URL.Path == "/" {
					// Fresh instance with no admin yet → guide the operator
					// through the one-time setup wizard.
					if setup.Needed() {
						http.Redirect(w, r, "/setup", http.StatusSeeOther)
						return
					}
					// A query in the URL is a question, and questions have their
					// own page with the history on it.
					q := r.URL.Query()
					if q.Get("q") != "" || q.Get("prompt") != "" {
						if _, acc := auth.TrySession(r); acc != nil {
							http.Redirect(w, r, "/agent?"+r.URL.RawQuery, http.StatusFound)
							return
						}
					}

					// The same front door either way.
					//
					// Signing in used to move you to /home, so the page a person
					// chose to visit was replaced by a different one the moment
					// they had an account — and the thing they were doing, which
					// is asking a question in the box, did not survive the move.
					// A product whose front page becomes a different product
					// once you sign up has two front pages and no front door.
					//
					// So this is one page in two states: the box, the day, the
					// way on, and signed in the day is also yours. /home is
					// still there and is still the dashboard — the rail of your
					// inbox and agents and balance, the grid of services — and
					// it is reached by the link in the corner, deliberately, by
					// somebody who came to look at things rather than to find
					// one thing out. See home.today.
					home.Index(w, r)
					return
				}
			}

			// Check if this is a user profile request (/@username)
			if strings.HasPrefix(r.URL.Path, "/@") {
				rest := r.URL.Path[2:]

				// Handle ActivityPub sub-endpoints: /@username/outbox, /@username/inbox
				if strings.HasSuffix(rest, "/outbox") {
					blog.OutboxHandler(w, r)
					return
				}
				if strings.HasSuffix(rest, "/inbox") {
					blog.InboxHandler(w, r)
					return
				}

				// Serve ActivityPub actor JSON if requested
				if !strings.Contains(rest, "/") && blog.WantsActivityPub(r) {
					blog.ActorHandler(w, r)
					return
				}

				// Otherwise it is the conversation with them.
				//
				// This served a profile — a name, a tick, a join date, a status
				// box and a list of their posts — and POST to it set the status,
				// through the content write gate, charged as a social post. All of
				// that is gone with the page. /@somebody is what the two of you
				// have said to each other now, which is a read of your own record
				// and writes nothing: the message itself is posted to /inbox/new,
				// where the gate for sending one already is.
				if !strings.Contains(rest, "/") {
					inbox.PersonHandler(w, r)
					return
				}
			}

			// CSRF protection: set token cookie on every response,
			// validate on state-changing requests.
			auth.SetCSRFCookie(w, r)
			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				if !csrfExempt(r) && !auth.ValidCSRF(r) {
					http.Error(w, `{"error":"invalid CSRF token"}`, http.StatusForbidden)
					return
				}
			}

			// ── Centralised write gate ──────────────────────────────
			// Every content-creating POST is charged, rate-limited,
			// and moderated from ONE place. Individual handlers do
			// NOT call CheckQuota/ConsumeQuota — the middleware does
			// it so nothing can be forgotten.
			if op := chargedWriteOp(r); op != "" {
				// Refusals go through app.Error, not http.Error: this is a
				// person who just pressed a button in a browser, and a bare
				// white page carrying one line of text is the worst possible
				// way to tell them what was refused and what to do about it.
				sess, err := auth.GetSession(r)
				if err != nil {
					app.Unauthorized(w, r)
					return
				}
				if !auth.CanPost(sess.Account) {
					app.Forbidden(w, r, auth.PostBlockReason(sess.Account))
					return
				}
				if err := auth.CheckPostRate(sess.Account); err != nil {
					app.TooManyRequests(w, r, err.Error())
					return
				}
				canProceed, _, cost, _ := quota.CheckQuota(sess.Account, op)
				if !canProceed {
					app.Error(w, r, http.StatusPaymentRequired,
						fmt.Sprintf("This costs %d credit(s). Top up at /wallet/topup", cost))
					return
				}
				// Charge up-front. The handler runs only if the
				// user can afford it. Failed handler calls (panics,
				// 5xx) are rare enough that the lost credit is
				// acceptable — and it's the only way to guarantee
				// we never forget to charge.
				if err := quota.Charge(sess.Account, op, nil); err != nil {
					app.Error(w, r, http.StatusPaymentRequired, err.Error())
					return
				}
				app.Log("wallet", "Charged %s %d credit(s) for %s %s", sess.Account, quota.OperationCost(op), r.Method, r.URL.Path)
			}

			// MCP authorization: an unauthenticated call to a tool that needs an
			// account gets a 401 naming the resource metadata, which is how a
			// client discovers it should start an OAuth flow. The discovery
			// documents existed without this and were never fetched, so the
			// standard way of connecting quietly did not work.
			//
			// Only auth-requiring tools challenge. A blanket 401 would make news
			// and weather unreachable without an account.
			// A wallet that signed instead of paying. Verified once, here,
			// because the nonce may only be spent once — checking it again
			// deeper in would refuse the caller's own second look.
			// Two doors dispatch tools — /mcp for something choosing one, and
			// /api/v1/ for something that already knows which it wants — and
			// everything below has to happen for both. It used to say /mcp four
			// times, which is how a second door starts out unauthenticated and
			// unpriced: the handler is the easy half, and this is the half
			// nobody remembers exists.
			//
			// Read the body once. It was read twice, restored twice, and parsed
			// twice for two questions about the same tool.
			if api.ToolDispatch(r.URL.Path) {
				host := strings.TrimPrefix(strings.TrimPrefix(app.BaseURL(r), "https://"), "http://")
				r, _ = wallet.AuthenticateRequest(r, strings.TrimRight(host, "/"))

				var body []byte
				if r.Method == http.MethodPost {
					body, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
					r.Body.Close()
					r.Body = io.NopCloser(bytes.NewReader(body))
				}
				tool := api.RequestTool(r.URL.Path, body)

				if api.ToolNeedsAuth(tool) {
					if _, err := auth.GetSession(r); err != nil && !x402.HasPayment(r) &&
						wallet.SignerFrom(r.Context()) == "" {
						origin := app.BaseURL(r)
						w.Header().Set("WWW-Authenticate",
							`Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource"`)
						app.RespondError(w, http.StatusUnauthorized, "authentication required")
						return
					}
				}

				// x402: gate metered tool calls. Both doors are public, so the
				// payment handshake lives here where auth + wallet are in
				// scope. A metered call with no session gets the standard 402
				// challenge; one bearing a payment header is routed to the
				// facilitator for verify+settle by the tool's QuotaCheck.
				//
				// Metered, not merely priced. A tool having a wallet operation
				// is not the same as it costing anything: news, web fetch,
				// quran and video search are zero on purpose. Gating on "has an
				// operation" charged an anonymous caller for all four, so the
				// free tier was unreachable and an agent that found this
				// endpoint mid-task met a demand for USDC on its first call.
				if op := api.ToolWalletOp(tool); x402.Enabled() && op != "" && quota.Metered(op) {
					// The public origin, not r.Host: behind the proxy r.Host is
					// the loopback port, and an x402 client checks this field
					// against what it is calling.
					resource := app.BaseURL(r) + r.URL.Path
					if x402.HasPayment(r) {
						holder := &x402.SettleHolder{}
						ctx := context.WithValue(r.Context(), x402.X402ContextKey, true)
						ctx = context.WithValue(ctx, x402.X402SettleKey, holder)
						r = r.WithContext(ctx)
						w = x402.NewSettleWriter(w, holder)
						// Nothing is written or charged until this runs: the
						// payment settles only if the response says the work
						// succeeded, and the response is held back until then
						// because the receipt is a header and the verdict is in
						// the body.
						defer x402.Finish(w)
					} else if who, blocked, reason := payer(r, token, op); blocked {
						// No listing. A discovery extension used to ride along
						// in this challenge, describing the refused tool so a
						// facilitator could index it, behind a setting that was
						// off by default and never turned on. See
						// internal/x402/bazaar.go for why the whole idea went.
						if x402.WritePaymentRequired(w, op, resource, nil, reason) {
							// Count the refusal. Calls are recorded inside the
							// dispatcher, which this returns before reaching,
							// so every call turned away at the door was absent
							// from the usage figures — including the free ones
							// this gate should never have been refusing. The
							// number that would have shown the mistake could
							// not see it.
							usage.Record("mcp-refused", op, who)
							return
						}
						// Nothing to charge: let it through rather than
						// inventing a price. Belt and braces with Metered
						// above, so neither check alone can paywall a free
						// tool again.
					}
				}
			}

			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start SMTP server if enabled (disabled by default)
	mail.StartSMTPServerIfEnabled()

	// And IMAP, so the mail this instance receives can be read in whatever
	// client somebody already has open. See service/mail/imap.go.
	mail.StartIMAPServerIfEnabled()

	// And submission, so that client can reply. IMAP on its own is a mailbox
	// you can read and not answer, which is half an address.
	mail.StartSubmissionServerIfEnabled()

	// And XMPP, which is the same address in real time. asim@here is a mailbox
	// and a chat address — one account, one local part, reachable two ways —
	// so Conversations, Dino or Gajim is a client for this instance the same
	// way Thunderbird already is. See service/chat/xmpp.go.
	chat.StartXMPPServerIfEnabled()
	// And the federated port. Separate because it is a separate decision: an
	// operator may want their own people on XMPP without accepting connections
	// from every other server on the internet.
	chat.StartS2SIfEnabled()

	// Log initial memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	app.Log("main", "Startup complete. Memory: Alloc=%dMB Sys=%dMB NumGC=%d", m.Alloc/1024/1024, m.Sys/1024/1024, m.NumGC)

	// Start memory monitoring goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			app.Log("main", "Memory: Alloc=%dMB Sys=%dMB NumGC=%d Goroutines=%d",
				m.Alloc/1024/1024, m.Sys/1024/1024, m.NumGC, runtime.NumGoroutine())
		}
	}()

	// Start server in a goroutine, preferring a systemd-activated socket so
	// redeploys don't drop the listener (see serveListener).
	go func() {
		ln, activated, err := serveListener(addr)
		if err != nil {
			app.Log("main", "Listen error on %s: %v", addr, err)
			quit <- syscall.SIGTERM
			return
		}
		if activated {
			app.Log("main", "Serving on systemd-activated socket (restarts queue, no 502)")
		} else {
			app.Log("main", "Starting server on %s", addr)
		}
		// And on the screen, which is a different audience with a different
		// question — see ready.go.
		ready(addr, activated)
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			app.Log("main", "Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	stopping := time.Now()
	app.Log("main", "Shutting down server...")

	// Flush the usage counters. They are saved on a slow cadence to keep the
	// request path cheap, so without this a deploy — which is a restart every
	// time — would drop the last minute of counts on every push.
	usage.Save()

	// How long to let in-flight requests finish.
	//
	// This is downtime unless systemd is holding the socket. Go's Shutdown
	// closes the listeners first and drains second, so from the moment it is
	// called nothing new can connect — and systemd will not start the
	// replacement until this process exits. Ten seconds of draining is
	// therefore ten seconds of refused connections, which is most of what "the
	// restart takes ages" is.
	//
	// With mu.socket in front of it none of that is true: the socket outlives
	// the process, connections queue, and a long drain costs latency rather
	// than errors. See docs/INSTALL.md — setting this low is the workaround,
	// holding the socket is the fix.
	//
	// The trade is real either way. An agent run is a model call, so a short
	// drain cuts somebody's answer off mid-sentence to save a few seconds of a
	// deploy.
	ctx, cancel := context.WithTimeout(context.Background(), drainFor)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		app.Log("main", "Server forced to shutdown: %v", err)
	}

	// What was said, on disk, before this process goes.
	//
	// internal/thread does not write on every message — it sets a flag and a
	// flusher does the work at most once a second, because writing the whole
	// file per message with the lock held made the UI stop answering while a
	// few hundred conversations were adopted. The comment on Flush says it is
	// exported "for the two callers that cannot wait for the tick: a test that
	// wants to know the file is on disk, and anything shutting down."
	//
	// The shutting-down caller was never written. So every restart dropped up
	// to a second of conversation, and this instance redeploys on a push — ask
	// a question, have a deploy land in that second, and the message is gone
	// from the page you reload. That is exactly how it was reported: not there
	// on refresh, there again later, because a later message flushed the file
	// with the earlier one still in memory.
	//
	// After Shutdown, not before: an agent run finishing during the drain
	// records its answer, and flushing first would write the file and then let
	// that answer land in memory only.
	thread.Flush()

	// How long it took, because this is the other half of a slow restart and
	// the half nobody measures. Shutdown waits for in-flight requests to
	// finish, and an agent run is a model call — so one chat open when a deploy
	// lands holds this for as long as the answer takes, up to the timeout
	// above. Seeing that number is what tells an operator whether the pause is
	// the old process leaving or the new one arriving.
	app.Log("main", "Server stopped in %s", time.Since(stopping).Round(time.Millisecond))
}

// drainFor is how long in-flight requests get when the server is stopping.
//
// Ten seconds, and not settable. This was SHUTDOWN_SECONDS, justified as an
// operator's decision because the right answer depends on whether something in
// front of the process is holding the listening socket — but the answer to that
// is the socket, not the drain: systemd socket activation keeps connections
// queued across a restart, which is what INSTALL.md tells an operator to set up.
// Ten seconds is longer than any request here except an agent run, and an agent
// run that has not finished in ten seconds is not going to finish in thirty.
const drainFor = 10 * time.Second
