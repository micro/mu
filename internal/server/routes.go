package server

// Every HTTP path this instance answers, and whether it needs a caller.
//
// The map is the authority on what is public: a path missing from it is
// treated as public, so a new page is open until somebody says otherwise, and
// the entry is the place to say it.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"mu/account"

	"mu/admin"
	"mu/agent"
	"mu/agent/a2a"
	"mu/agent/digest"
	"mu/agent/micro"
	"mu/client/whatsapp"
	help "mu/docs"
	"mu/home"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/imageproxy"
	"mu/internal/settings"
	"mu/internal/setup"
	"mu/internal/user"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/hazards"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/routes"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/transit"
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
	whatsappsvc "mu/service/whatsapp"
)

// authRequired reports, per path, whether a caller must be signed in.
func authRequired() map[string]bool {
	authenticated := map[string]bool{
		"/tools":                  false, // Public — the catalogue, agent lens
		"/services":               false, // Public — the catalogue, person lens
		"/card/":                  false, // Public — a service rendered at a glance
		"/usage":                  true,  // Your own calls and spend
		"/video":                  false, // Public viewing, auth for interactive features
		"/video/thumb":            false, // Public — thumbnails for the public feed
		"/news":                   false, // Public viewing, auth for search
		"/chat":                   false, // Public viewing, auth for chatting
		"/home":                   false, // Public viewing
		"/blog":                   false, // Public viewing, auth for posting
		"/markets":                false, // Public viewing
		"/text":                   false, // Public: the tools are callable without an account
		"/food":                   false, // Public food data is public
		"/transit":                false, // Public transport data is public
		"/hazards":                false, // Public hazard data, published to be redistributed
		"/prayer":                 false, // Public prayer times, daily verse and hadith
		"/about":                  false, // Public "what is Mu" pitch
		"/oauth2/google":          false, // Google sign-in start (no session yet)
		"/oauth2/google/connect":  true,  // Link Google to the current account
		"/agents":                 true,  // Your agents and their tokens — sign-in required
		"/agents/data":            true,  // JSON behind the chat's agent picker
		"/oauth2/google/calendar": true,  // Grant calendar access to the current account
		"/oauth2/google/contacts": true,  // Grant contacts access to the current account
		"/oauth2/callback":        false, // Google sign-in callback (no session yet)
		"/images":                 false, // Public daily image; generation needs login
		"/img":                    false, // Public — cached article images (a prefix of /images, same answer)
		"/events":                 true,  // Personal scheduled reminders — sign-in required
		"/contacts":               true,  // Your address book — sign-in required
		"/notes":                  true,  // What you and your agents wrote down — sign-in required
		// Your own documents. Sign-in required, but checked in the handler
		// rather than here: the map is matched by prefix, and /docs/<slug> is
		// still a public redirect to the documentation that used to live there.
		"/docs": false,
		// Your texts. Sign-in is required and the handler requires it — not
		// stated here, because this map is matched by prefix and /sms/webhook
		// is the provider posting an inbound message with no session at all.
		"/sms": false,
		// Your WhatsApp threads. Sign-in checked in the handler, for the same
		// reason as /sms: this map is matched by prefix and /whatsapp/twilio is
		// the provider posting an inbound message with no session at all.
		"/whatsapp":          false,
		"/runs":              true,  // What your agents did (redirects to /agent/runs)
		"/agent/runs":        true,  // What your agents did
		"/agent/session/":    true,  // Deleting one of your conversations
		"/agent/connect":     true,  // How to reach one agent
		"/tasks":             true,  // Your task list — sign-in required
		"/social":            false, // Public viewing, auth for search
		"/social/thread":     false, // Public thread view, auth for messaging
		"/places":            false, // Public map, auth for search
		"/weather":           false, // Public page, auth for forecast lookup
		"/flights":           false, // Public — aircraft broadcast their positions in clear
		"/mail":              true,  // Require auth for inbox
		"/logout":            true,
		"/account":           true,
		"/user":              true, // Your own saved, hidden and blocked
		"/user/":             true,
		"/verify":            false, // Public — token in URL is the credential
		"/token":             true,  // PAT token management
		"/passkey":           false, // Passkey login/register (auth checked in handler)
		"/session":           false, // Public - used to check auth status
		"/api":               false, // Public - API documentation
		"/admin/flag":        true,
		"/admin":             true,
		"/admin/users":       true,
		"/admin/moderate":    true,
		"/admin/blocklist":   true,
		"/admin/spam":        true,
		"/admin/email":       true,
		"/admin/api":         true,
		"/admin/log":         true,
		"/admin/env":         true,
		"/admin/server":      true,
		"/admin/usage":       true,
		"/admin/delete":      true,
		"/admin/console":     true,
		"/admin/diagnostics": true,
		"/admin/retention":   true,
		"/admin/backup":      true,
		"/admin/invite":      true,
		"/account/":          true, // Money: top-up, transfer, the ledger

		"/apps":      false, // Public - apps directory; auth checked in handler for create/edit
		"/work":      false, // Public - task bounties; auth checked in handler for post/claim
		"/search":    false, // Public - web search
		"/web":       false, // Redirect to /search
		"/web/fetch": false, // Public page, auth checked in handler (paid web fetch)
		"/web/read":  false, // Public page, auth checked in handler (proxied reader)

		"/status":                        false, // Public - server health status
		"/pricing":                       false, // Public - pricing page
		"/plans":                         false, // Public - the same page
		"/privacy":                       false, // Public - privacy policy
		"/support":                       false, // Public - how to reach the operator
		"/help":                          false, // Public - how to connect an agent
		"/install":                       false, // Public - run your own instance
		"/whitepaper":                    false, // Public - whitepaper
		"/mcp":                           false, // Public - MCP tools page
		"/whatsapp/webhook":              false, // Public - WhatsApp webhook
		"/sms/webhook":                   false, // Public - inbound SMS; the provider's signature is the credential
		"/.well-known/agent.json":        false, // Public - A2A agent card
		"/.well-known/mcp-registry-auth": false, // Public - registry domain proof
		"/a2a":                           false, // Public - A2A protocol
		// Public at the door, decided per tool inside. The same answer /mcp
		// gives, for the same reason: news and weather must not need an account.
		"/api/v1":     false,
		"/api/v1/":    false,
		"/agent":      false, // Public page, auth checked in handler
		"/setup":      false, // First-run setup (open only until an admin exists)
		"/developers": false, // Legacy alias → /tools (public)
	}
	return authenticated
}

// staticSuffixes never require authentication.
func staticSuffixes() []string {
	return []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg",
		".ico", ".webmanifest", ".json",
	}
}

// registerRoutes attaches every handler to the default mux.
func registerRoutes() {
	// serve video
	http.HandleFunc("/video", video.Handler)
	http.HandleFunc("/video/thumb", video.ThumbHandler)

	// serve news
	http.HandleFunc("/news", news.Handler)
	// serve chat
	http.HandleFunc("/chat", chat.Handler)

	// serve blog (full list)
	http.HandleFunc("/blog", blog.Handler)

	// serve individual blog post (public, no auth)
	// Serves ActivityPub JSON-LD when requested via Accept header
	http.HandleFunc("/blog/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && blog.WantsActivityPub(r) {
			blog.PostObjectHandler(w, r)
			return
		}
		blog.PostHandler(w, r)
	})

	// handle comments on posts /blog/post/{id}/comment
	http.HandleFunc("/blog/post/", blog.CommentHandler)

	// Legacy redirects for old URL structure (301 so browsers/crawlers update)
	legacyRedirect := func(oldPrefix, newPrefix string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			target := newPrefix + r.URL.Path[len(oldPrefix):]
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}
	}
	http.HandleFunc("/post/", legacyRedirect("/post/", "/blog/post/"))
	http.HandleFunc("/post", legacyRedirect("/post", "/blog/post"))
	http.HandleFunc("/fetch", legacyRedirect("/fetch", "/web/fetch"))
	http.HandleFunc("/read", legacyRedirect("/read", "/web/read"))

	// flag content
	http.HandleFunc("/admin/flag", admin.FlagHandler)

	// admin dashboard
	http.HandleFunc("/admin", admin.Handler)

	// admin user management
	http.HandleFunc("/admin/users", admin.UsersHandler)

	// moderation queue
	http.HandleFunc("/admin/moderate", admin.ModerateHandler)

	// mail blocklist management
	http.HandleFunc("/admin/blocklist", admin.BlocklistHandler)

	// spam filter management
	http.HandleFunc("/admin/spam", admin.SpamFilterHandler)

	// email log
	http.HandleFunc("/admin/email", admin.EmailLogHandler)

	// external API call log
	http.HandleFunc("/admin/api", admin.APILogHandler)

	// system log
	http.HandleFunc("/admin/log", admin.SysLogHandler)

	// environment variables status
	http.HandleFunc("/admin/env", admin.EnvHandler)

	// server update and restart
	http.HandleFunc("/admin/server", admin.UpdateHandler)

	// AI usage tracking
	http.HandleFunc("/admin/usage", admin.AIUsageHandler)
	http.HandleFunc("/admin/traffic", admin.TrafficHandler)

	// admin delete (any content type)
	http.HandleFunc("/admin/delete", admin.DeleteHandler)

	// admin console
	http.HandleFunc("/admin/console", admin.ConsoleHandler)
	http.HandleFunc("/admin/diagnostics", admin.DiagnosticsHandler)
	http.HandleFunc("/admin/retention", admin.RetentionHandler)
	http.HandleFunc("/admin/backup", admin.BackupHandler)
	http.HandleFunc("/admin/invite", admin.InviteHandler)

	// Money: top-up, transfer, Stripe and the price list, all under the account
	// that holds them.
	http.HandleFunc("/account/", account.BalanceHandler)

	// Where the money used to be. /wallet is a service now, so these are not
	// merely renamed — the old prefix has come to mean something else, and a
	// bookmark for a balance must not land on a crypto address.
	for _, moved := range []string{
		"/wallet/topup", "/wallet/transfer", "/wallet/pricing",
		"/wallet/stripe/checkout", "/wallet/stripe/success",
	} {
		http.HandleFunc(moved, account.MovedToAccount)
	}

	// Stripe posts here. Named for the provider, at the top level, and that is
	// the whole point: a webhook URL is a contract with somebody outside this
	// process, configured once in their dashboard, and it has to survive every
	// rearrangement of ours. It was /wallet/stripe/webhook, and when the money
	// moved to the account the webhook could not follow — a redirect on a POST
	// is a dropped payment, and /account is authenticated by prefix while a
	// webhook must be public. Both of those are reasons the path should never
	// have described where credits happened to live.
	//
	// This instance has made the same mistake once before in the other
	// direction: /sms/webhook is named for our service rather than the provider,
	// and Twilio — which bundles SMS and WhatsApp onto one webhook — posted
	// WhatsApp messages to it that were refused for being the wrong kind.
	//
	// The old path is gone. It was kept live across the move so that no top-up
	// was lost between the deploy and the dashboard edit; the dashboard now
	// names this one.
	http.HandleFunc("/stripe/webhook", account.HandleStripeWebhook)

	// serve whatsapp webhook
	http.HandleFunc("/whatsapp/webhook", whatsapp.Handler)

	// A2A protocol endpoints
	domain := settings.Get("MU_DOMAIN")
	if domain == "" {
		domain = "localhost:8080"
	}
	if !strings.HasPrefix(domain, "http") {
		domain = "https://" + domain
	}
	a2a.BaseURL = domain
	http.HandleFunc("/.well-known/agent.json", a2a.AgentCardHandler)
	http.HandleFunc("/a2a", a2a.Handler)

	// serve search page (local + Brave web search)
	// serve search page
	http.HandleFunc("/search", web.Handler)
	http.HandleFunc("/web", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/search?"+r.URL.RawQuery, http.StatusMovedPermanently)
	})
	http.HandleFunc("/web/preview", web.PreviewHandler)

	// serve web fetch page (fetch and clean a URL)
	http.HandleFunc("/web/fetch", web.FetchHandler)

	// serve clean reader page for web results
	http.HandleFunc("/web/read", web.ReadHandler)

	// serve fact-check page and API

	// The dashboard lives at the named URL /home, consistent with every other
	// section (/news, /mail, /agent …). It renders for everyone: logged out, the
	// home screen is the public face — real cards plus the agent — so a visitor
	// sees the product rather than a separate marketing page.
	http.HandleFunc("/home", home.Handler)
	// Everything used to be a landing: the logged-out root said nothing, /about
	// was a pitch and /agents was a second pitch. There is one landing now — the
	// root — and these two go back to being what their names say.
	//
	// /about is the about page, which is the ABOUT doc rather than a second copy
	// of it — registered with the other documentation pages below. /agents
	// belongs to the user's agents, not to marketing; it points at the agent
	// surface until the page that shows what your agents are doing exists to
	// take it.
	http.HandleFunc("/pricing", home.PricingHandler)
	// The same page under the name a customer looks for. A visitor deciding
	// whether to start reads "pricing"; somebody already signed in who wants
	// more agents looks for "plans", and finding nothing there is how they end
	// up hunting through the menu. One page, because two would describe the same
	// prices and drift.
	http.HandleFunc("/plans", home.PricingHandler)
	// Every MCP directory submission asks for a privacy policy URL, and this
	// instance runs a mail server — so there is real correspondence to account
	// for, not just a formality.
	http.HandleFunc("/privacy", home.PrivacyHandler)
	http.HandleFunc("/support", home.SupportHandler)

	// first-run setup wizard (open only until an admin exists)
	http.HandleFunc("/setup", setup.Handler)

	// Redirect the old path so existing links keep working. It used to point at
	// /agents back when that was a public redirect to /agent; /agents is now the
	// signed-in page where you create and scope agents, so a developer following
	// this link would hit a login wall instead of the thing they came for. /tools
	// is that thing: the endpoint, the config and the token.
	http.HandleFunc("/developers", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tools", http.StatusMovedPermanently)
	})

	// serve the agent
	http.HandleFunc("/agent", agent.Handler)
	http.HandleFunc("/agent/", agent.Handler)
	http.HandleFunc("/agents/data", agent.AgentsHandler)
	// The old path, so a page cached with the previous script keeps working.
	http.HandleFunc("/agent/agents", agent.AgentsHandler)
	http.HandleFunc("/agent/new", agent.NewAgentHandler)
	http.HandleFunc("/agent/run", agent.RunHandler)
	http.HandleFunc("/agent/exec", agent.ExecResultHandler)

	// serve mail inbox
	http.HandleFunc("/mail", mail.Handler)

	// serve markets page
	http.HandleFunc("/markets", markets.Handler)
	http.HandleFunc("/text", text.Handler)
	http.HandleFunc("/food", food.Handler)
	http.HandleFunc("/transit", transit.Handler)
	http.HandleFunc("/hazards", hazards.Handler)
	http.HandleFunc("/wallet", wallet.Handler)
	// Taking your key with you. A page action and never a tool: an agent that
	// can read a private key is a prompt injection away from posting it
	// somewhere. It re-checks the password, so it is not in authRequired()
	// either — the handler wants the session *and* the password.
	http.HandleFunc("/wallet/export", wallet.ExportHandler)
	http.HandleFunc(imageproxy.Path, imageproxy.Handler)
	http.HandleFunc("/contacts", contacts.Handler)
	http.HandleFunc("/docs", docs.Handler)
	http.HandleFunc("/notes", notes.Handler)
	http.HandleFunc("/sms", sms.Handler)
	http.HandleFunc("/whatsapp", whatsappsvc.Handler)

	// One inbound endpoint for both, because Twilio uses one.
	//
	// A Messaging Service carries a phone number and a WhatsApp sender
	// together and posts everything that arrives to the one webhook on it, so
	// a WhatsApp message turned up at /sms/webhook and was refused there for
	// not being a phone number. Two paths were registered on the belief that
	// one endpoint would mean guessing which channel had called, and the
	// payload settles it without guessing: WhatsApp addresses carry a
	// whatsapp: prefix. Both paths still answer, so whichever is configured
	// works.
	inbound := func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm() //nolint:errcheck — the handlers parse again and report it
		if strings.HasPrefix(r.PostForm.Get("From"), "whatsapp:") ||
			strings.HasPrefix(r.PostForm.Get("To"), "whatsapp:") {
			whatsappsvc.WebhookHandler(w, r)
			return
		}
		sms.WebhookHandler(w, r)
	}
	http.HandleFunc("/whatsapp/twilio", inbound)
	http.HandleFunc("/sms/webhook", inbound)
	http.HandleFunc("/contacts/", contacts.Handler)
	http.HandleFunc("/tasks", tasks.Handler)
	http.HandleFunc("/tasks/", tasks.Handler)
	http.HandleFunc("/images", images.Handler)
	http.HandleFunc("/images/daily/", images.DailyImageHandler)
	http.HandleFunc("/images/file/", images.GeneratedImageHandler)
	http.HandleFunc("/events", events.Handler)
	// /files lists a person's files; /files/<id> serves one. A stored file's URL
	// has to be fetchable by an ordinary HTTP client, or handing someone a link
	// to it is worthless.
	http.HandleFunc("/files", files.Handler)
	http.HandleFunc("/files/", files.Handler)

	// serve social page
	http.HandleFunc("/social", social.Handler)
	http.HandleFunc("/social/thread", social.ThreadHandler)

	// Stream (console) routes
	http.HandleFunc("/stream", stream.Handler)
	http.HandleFunc("/stream/fragment", stream.FragmentHandler)

	http.HandleFunc("/prayer", prayer.Handler)
	// Back-compat: the page lived at /reminder, then at /islam. Both are still
	// in bookmarks and in other people's links, so both keep working.
	for _, old := range []string{"/reminder", "/islam"} {
		http.HandleFunc(old, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/prayer", http.StatusMovedPermanently)
		})
	}

	// serve places page
	http.HandleFunc("/places", places.Handler)
	http.HandleFunc("/places/", places.Handler)

	// serve weather page
	http.HandleFunc("/weather", weather.Handler)

	// serve flights page
	http.HandleFunc("/flights", flights.Handler)

	// serve routes page
	http.HandleFunc("/routes", routes.Handler)

	// serve apps
	http.HandleFunc("/apps", apps.Handler)
	http.HandleFunc("/apps/", apps.Handler)

	// serve work (task bounties)

	// content controls (flag, save, dismiss, block, share)
	http.HandleFunc("/app/", app.ControlsHandler)

	// auth
	http.HandleFunc("/login", account.Login)
	http.HandleFunc("/logout", account.Logout)
	http.HandleFunc("/signup", account.Signup)
	http.HandleFunc("/request-invite", account.RequestInvite)
	http.HandleFunc("/invite", account.InviteHandler)
	http.HandleFunc("/user", user.Handler)
	http.HandleFunc("/user/", user.UndoHandler)
	http.HandleFunc("/account", account.Account)
	http.HandleFunc("/verify", account.Verify)
	http.HandleFunc("/session", account.Session)
	http.HandleFunc("/updates", updatesHandler)
	http.HandleFunc("/token", account.TokenHandler)
	http.HandleFunc("/passkey/", account.PasskeyHandler)

	// OAuth 2.1 for MCP authentication
	http.HandleFunc("/.well-known/oauth-authorization-server", auth.OAuthMetadataHandler)
	http.HandleFunc("/.well-known/oauth-protected-resource", auth.OAuthResourceHandler)

	// Proof of domain ownership for the MCP registry, which accepts either a
	// DNS TXT record or this file. Serving it is the easier half: it ships with
	// the binary, so an operator publishing their own instance needs no DNS
	// access and nothing to remember to keep in sync.
	//
	// The value is a public key. It is safe to serve and useless without the
	// private half, which stays with whoever publishes. MCP_REGISTRY_PROOF holds
	// it; unset, this 404s like any other absent file.
	http.HandleFunc("/.well-known/mcp-registry-auth", func(w http.ResponseWriter, r *http.Request) {
		proof := strings.TrimSpace(settings.Get("MCP_REGISTRY_PROOF"))
		if proof == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(proof + "\n"))
	})
	// Google sign-in (Mu as an OAuth client of Google).
	// One list of agents, whichever page you found it on.
	//
	// The chat at /agent already had user-defined agents — a name, a prompt and
	// an allowed set of services — stored by agent/micro. /agents adds the two
	// things those lacked: a scope that is enforced against a credential, and a
	// token to hand out. Rather than a second concept wearing the same word,
	// the record on /agents resolves as one of those agents, so an agent marked
	// "runs here" can be talked to, with its own instructions and confined to
	// its own services. Without this, that option on the create form stored a
	// field nothing read.
	// One store. Agents made in the chat used to live in agent/micro's own
	// file while /agents wrote to the roster, so "my agents" depended on which
	// page you asked and an agent made in the chat had no scope and no token.
	// Existing ones are imported once, without minting credentials nobody asked
	// for; the old file is left alone so this is reversible.
	if n := agent.ImportUserAgents(micro.AllUserAgents()); n > 0 {
		app.Log("agents", "imported %d agent(s) into the roster", n)
	}
	micro.UserAgentResolver = func(accountID, id string) *micro.Agent {
		if a := agent.For(accountID, id); a != nil {
			return a.AsMicro()
		}
		return nil
	}

	// /agents is a page, not a service, and deliberately has no RPC surface.
	// A tool that created agents would let a scoped agent mint an unscoped one,
	// which is privilege escalation dressed as a feature. Agents are created by
	// a person in a browser or not at all.
	http.HandleFunc("/agents", agent.RosterHandler)

	http.HandleFunc("/oauth2/google", account.GoogleLogin)
	http.HandleFunc("/oauth2/google/connect", account.GoogleConnect)
	http.HandleFunc("/oauth2/callback", account.GoogleCallback)
	// Reading a calendar is a separate grant, asked for separately — see
	// internal/app/google_calendar.go.
	http.HandleFunc("/oauth2/google/calendar", account.GoogleGrantConnect)
	http.HandleFunc("/oauth2/google/contacts", account.GoogleGrantConnect)
	http.HandleFunc("/oauth2/google/disconnect", account.GoogleGrantDisconnect)

	http.HandleFunc("/oauth/register", auth.OAuthRegisterHandler)
	http.HandleFunc("/oauth/authorize", auth.OAuthAuthorizePostHandler)
	http.HandleFunc("/oauth/token", auth.OAuthTokenHandler)

	// internal status (injected into admin server page)
	app.DKIMStatusFunc = mail.DKIMStatus
	app.DigestStatusFunc = digest.Status
	admin.GenerateDigestFunc = digest.Generate

	// public status page - service health checks
	app.HealthCheckFunc = runHealthChecks
	http.HandleFunc("/status", app.StatusHandler)

	// whitepaper
	http.HandleFunc("/whitepaper", help.WhitepaperHandler)
	http.HandleFunc("/whitepaper.pdf", help.WhitepaperHandler)

	// Documentation. Two pages: how to point an agent at this instance, and
	// how to run your own. Every address the old nine answered on redirects to
	// whichever of the two replaced it — an exact pattern outranks the /docs
	// the service owns now.
	http.HandleFunc("/about", help.AboutHandler)
	http.HandleFunc("/help", help.Handler)
	http.HandleFunc("/install", help.InstallHandler)
	for from, to := range help.Redirects {
		if from == "/docs" {
			continue // the service's page, not a redirect
		}
		target := to
		http.HandleFunc(from, func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		})
	}

	// ActivityPub: WebFinger discovery
	http.HandleFunc("/.well-known/webfinger", blog.WebFingerHandler)

	// presence WebSocket endpoint
	http.HandleFunc("/presence", user.PresenceHandler)

	// presence ping endpoint
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, acc, err := auth.RequireSession(r)
		if err != nil {
			app.Unauthorized(w, r)
			return
		}

		auth.UpdatePresence(acc.ID)

		w.Header().Set("Content-Type", "application/json")
		onlineCount := auth.OnlineCount()
		w.Write([]byte(fmt.Sprintf(`{"status":"ok","online":%d}`, onlineCount)))
	})

	// /version — what's deployed and how it's wired, for verifying releases.
	http.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(versionInfo())
	})

	// /api is the HTTP API reference. It was a redirect to /mcp, on the
	// argument that two documented doors is a decision the reader has to make
	// before they can start — right when the second door was another way for an
	// agent to call tools, wrong now. Somebody building a desktop client is not
	// choosing between two things, and being sent to a tool-calling protocol
	// reads as "not for you". The two pages say different things and link to
	// each other.
	http.HandleFunc("/api", api.RESTPageHandler)

	// /api/v1/<service>/<method> — the door for a program that is not an agent.
	//
	// It has no auth story and no price table of its own; it turns a path into a
	// tool name and calls the same function /mcp calls, which is why adding it
	// is not adding a second way in. See internal/api/rest.go.
	// Both forms, because serve() strips a trailing slash before routing: with
	// only the subtree pattern the bare root redirects to itself forever. Every
	// other subtree route here does the same.
	http.HandleFunc(api.RESTRoot, api.RESTHandler)
	http.HandleFunc(api.RESTPrefix, api.RESTHandler)

	// serve the MCP page and server (GET = HTML page, POST = JSON-RPC)
	// One catalogue, two lenses — see internal/api/tools_page.go.
	http.HandleFunc("/tools", api.ToolsPageHandler)
	http.HandleFunc("/services", api.ToolsPageHandler)
	// What your agents did. Flows were recorded and never served.
	// Runs belong to the agent, so they live under it and the agent surface
	// tabs between them. /runs still works — links to it exist.
	http.HandleFunc("/agent/connect", agent.ConnectHandler)
	// Deleting a conversation. The list of them is the rail on /agent.
	http.HandleFunc("/agent/session/", agent.SessionHandler)
	// Two pages that were tabs and are not any more: one listed the same
	// conversations the rail lists, the other listed the workflow records behind
	// them. Both are on /agent now — the conversation, and the tools each answer
	// came from beside the answer.
	http.HandleFunc("/agent/threads", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agent", http.StatusMovedPermanently)
	})
	http.HandleFunc("/agent/runs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agent", http.StatusMovedPermanently)
	})
	http.HandleFunc("/runs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agent", http.StatusMovedPermanently)
	})
	// A service rendered at a glance — see internal/api/card.go.
	http.HandleFunc("/card", api.CardHandler)
	http.HandleFunc("/card/", api.CardHandler)
	// Your own usage — the caller-facing half of /admin/traffic.
	http.HandleFunc("/usage", home.UsageHandler)
	http.HandleFunc("/mcp", api.MCPHandler)

	// serve the app
	http.Handle("/", app.Serve())
}
