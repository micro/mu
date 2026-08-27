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
	"mu/agent/digest"
	"mu/agent/micro"
	help "mu/docs"
	"mu/home"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/imageproxy"
	"mu/internal/push"
	"mu/internal/settings"
	"mu/internal/setup"
	"mu/internal/user"
	"mu/service/apps"
	"mu/service/archive"
	"mu/service/blog"
	"mu/service/browser"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/maps"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/notify"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/recall"
	"mu/service/routes"
	"mu/service/shell"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/transit"
	"mu/service/users"
	"mu/service/video"
	"mu/service/wallet"
	"mu/service/weather"
	"mu/service/web"
)

// authRequired reports, per path, whether a caller must be signed in.
func authRequired() map[string]bool {
	authenticated := map[string]bool{
		"/tools":       false, // Public — the catalogue, agent lens
		"/tools/":      false, // Public — one tool, same as the catalogue
		"/services":    false, // Public — the catalogue, person lens
		"/services/":   false, // Public — one service, what it is and how to call it
		"/card/":       false, // Public — a service rendered at a glance
		"/usage":       true,  // Your own calls and spend
		"/video":       false, // Public viewing, auth for interactive features
		"/video/thumb": false, // Public — thumbnails for the public feed
		"/news":        false, // Public viewing, auth for search
		"/chat":        false, // Public viewing, auth for chatting
		// SASL inside the stream, with an access token — so no session is
		// required to open it and none would be honoured. See xmpp_ws.go.
		"/xmpp-websocket":             false,
		"/.well-known/host-meta.json": false, // How a browser finds the above
		"/home":                       false, // Public viewing
		"/blog":                       false, // Public viewing, auth for posting
		"/markets":                    false, // Public viewing
		"/text":                       false, // Public: the tools are callable without an account
		"/food":                       false, // Public food data is public
		"/transit":                    false, // Public transport data is public
		"/browser":                    false, // Public — the page; reading one costs and needs a session
		"/shell":                      true,  // A machine with your files on it, so it needs a session
		"/browser/shot/":              false, // A picture already taken, of a page anybody could open
		"/maps":                       false, // Public — the page, and any tile already held
		"/maps/":                      false, // A held tile is free to anybody; a cold one needs a session
		"/prayer":                     false, // Public prayer times, daily verse and hadith
		"/oauth2/google":              false, // Google sign-in start (no session yet)
		"/oauth2/google/connect":      true,  // Link Google to the current account
		"/agents":                     true,  // Your agents and their tokens — sign-in required
		"/agents/data":                true,  // JSON behind the chat's agent picker
		"/oauth2/google/calendar":     true,  // Grant calendar access to the current account
		"/oauth2/google/contacts":     true,  // Grant contacts access to the current account
		"/oauth2/callback":            false, // Google sign-in callback (no session yet)
		"/images":                     false, // Public daily image; generation needs login
		"/img":                        false, // Public — cached article images (a prefix of /images, same answer)
		"/events":                     true,  // Personal scheduled reminders — sign-in required
		"/users":                      true,  // Who is on this instance — sign-in required
		"/contacts":                   true,  // Your address book — sign-in required
		"/notes":                      true,  // What you and your agents wrote down — sign-in required
		"/notify":                     true,  // What you were told, and where you can be reached — sign-in required
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
		"/agent/session/":    true,  // Deleting one of your conversations
		"/recall":            true,  // Your own past — sign-in required
		"/agent/connect":     true,  // How to reach one agent
		"/tasks":             true,  // Your task list — sign-in required
		"/social":            false, // Public viewing, auth for search
		"/social/thread":     false, // Public thread view, auth for messaging
		"/places":            false, // Public map, auth for search
		"/weather":           false, // Public — the forecast as JSON
		"/flights":           false, // Public — aircraft broadcast their positions in clear
		"/mail":              true,  // Require auth for inbox
		"/logout":            true,
		"/account":           true,
		"/report":            true,  // Telling an operator about somebody else's item
		"/profile/status":    true,  // Setting what you are doing, on your own profile
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
		"/admin/log":         true,
		"/admin/config":      true,
		"/admin/server":      true,
		"/admin/usage":       true,
		"/admin/delete":      true,
		"/admin/diagnostics": true,
		"/admin/alerts":      true,
		"/admin/backup":      true,
		"/admin/invite":      true,
		"/account/":          true, // Money: an old prefix, which redirects
		"/wallet/":           true, // Money: top-up, transfer, Stripe, the price list

		"/apps":      false, // Public - apps directory; auth checked in handler for create/edit
		"/work":      false, // Public - task bounties; auth checked in handler for post/claim
		"/web":       false, // Public - the open web: search it, read a page from it
		"/web/fetch": false, // Public page, auth checked in handler (paid web fetch)
		"/web/read":  false, // Public page, auth checked in handler (proxied reader)

		"/status":                        false, // Public - server health status
		"/privacy":                       false, // Public - privacy policy
		"/install":                       false, // Public - run your own instance
		"/mcp":                           false, // Public - MCP tools page
		"/sms/webhook":                   false, // Public - inbound SMS; the provider's signature is the credential
		"/.well-known/mcp-registry-auth": false, // Public - registry domain proof
		// Public at the door, decided per tool inside. The same answer /mcp
		// gives, for the same reason: news and weather must not need an account.
		"/api/v1":  false,
		"/api/v1/": false,
		"/agent":   false, // Redirects to the named page; auth checked in handler
		"/agent/":  false, // /agent/<name> — one agent's page; auth checked in handler
		"/push/":   true,  // Subscribing this device to notifications
		"/inbox":   true,  // The mailbox — yours, so it needs a session
		"/inbox/":  true,  // One alias's mail
		"/setup":   false, // First-run setup (open only until an admin exists)
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
	// XMPP for the browser (RFC 7395), so the web page is a carrier of the chat
	// protocol rather than a second protocol beside it — and host-meta, which is
	// how a browser finds this endpoint from a domain, since it cannot look up
	// the SRV record a desktop client uses.
	http.HandleFunc("/xmpp-websocket", chat.XMPPWebSocketHandler)
	http.HandleFunc("/.well-known/host-meta.json", chat.WellKnownHostMeta)

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

	// flag content
	http.HandleFunc("/admin/flag", admin.FlagHandler)

	// admin dashboard
	http.HandleFunc("/admin", admin.Handler)

	// admin user management
	http.HandleFunc("/admin/users", admin.UsersHandler)

	// moderation queue
	http.HandleFunc("/admin/moderate", admin.ModerateHandler)

	// mail blocklist management
	http.HandleFunc("/admin/blocklist", admin.BlocklistMoved)

	// spam filter management
	http.HandleFunc("/admin/spam", admin.SpamHandler)

	// email log
	http.HandleFunc("/admin/email", admin.MailLogMoved)

	// external API call log

	// system log
	http.HandleFunc("/admin/log", admin.LogHandler)

	// environment variables status
	http.HandleFunc("/admin/config", admin.ConfigHandler)

	// server update and restart
	http.HandleFunc("/admin/server", admin.ServerHandler)

	// AI usage tracking
	http.HandleFunc("/admin/usage", admin.SpendMoved)
	http.HandleFunc("/admin/traffic", admin.TrafficHandler)

	// admin delete (any content type)
	http.HandleFunc("/admin/delete", admin.DeleteHandler)

	// admin console
	http.HandleFunc("/admin/diagnostics", admin.DiagnosticsHandler)
	// What this instance will wake you for. See admin/alert.go.
	http.HandleFunc("/admin/alerts", admin.AlertsHandler)
	http.HandleFunc("/admin/backup", admin.BackupHandler)
	http.HandleFunc("/admin/invite", admin.InviteHandler)

	// Money: top-up, transfer, Stripe and the price list, all under /wallet with
	// the balance they change. The handler lives in account/ because that is
	// where credits are kept; the destination is /wallet because that is the
	// word somebody already has for the place their money is.
	//
	// /account/ and /billing/ stay registered and only redirect. Both were the
	// money prefix at some point and somebody has each bookmarked.
	http.HandleFunc("/account/", account.BalanceHandler)

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

	// The service is web — searching the open web is one of the things it does,
	// and a service is named for a domain rather than an action. Its page was
	// at /search and everything under it at /web/fetch, /web/read,
	// /web/preview, so one service had its URL tree split in half and was the
	// only exception to service name == route in the catalogue.
	//
	// /search still answers, permanently redirected, because it is the address
	// people have and search engines hold.
	// A real browser, for the pages a fetch cannot read. /browser is the page;
	// the pictures it takes are at /browser/shot/<id>.png. See service/browser.
	// A machine of your own, in a container. See service/shell.
	http.HandleFunc("/shell", shell.Handler)
	// The address this had until it was renamed. Kept because links to it
	// exist — in mail this instance has already sent, and in anybody's
	// bookmarks — and breaking a URL to tidy a name is a bad trade.
	http.HandleFunc("/browser", browser.Handler)
	http.HandleFunc("/browser/shot/", browser.ShotHandler)

	http.HandleFunc("/web", web.Handler)
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
	// Every MCP directory submission asks for a privacy policy URL, and this
	// instance runs a mail server — so there is real correspondence to account
	// for, not just a formality.
	http.HandleFunc("/privacy", home.PrivacyHandler)

	// first-run setup wizard (open only until an admin exists)
	http.HandleFunc("/setup", setup.Handler)

	// serve the agent
	// The inbox is /inbox, and the agent family is /agent*.
	//
	// It was /agent for the conversation and /agents for the roster — a
	// singular and a plural of the same word, one page apart, which is the
	// confusion routes.go already records about /agent/agents. Naming the page
	// after what it holds fixes it: /inbox is where you read and reply, and
	// /agents, /agent/new and /agent/connect are where an agent is made,
	// scoped and reached. Two families with two names.
	//
	// /agent still answers. A GET is redirected, because links to it exist; a
	// POST is the chat asking a question and is handled here, because moving
	// that is a change to the component every page embeds and it can follow.
	// The inbox is the mailbox. It was agent.Handler — the chat — which is why
	// ordinary mail never appeared on the page called Inbox; the chat is
	// /agent/<name> now.
	http.HandleFunc("/inbox", inbox.Handler)
	// /inbox/<box> — one alias's mail. An alias is a mailbox: asim+research@
	// goes to the research agent, so what arrives there is that agent's mail.
	http.HandleFunc("/inbox/", inbox.Handler)
	// /agent — the chat with no agent named. A GET goes to the one it is about,
	// which is /agent/<name>: an agent is a place, and a place has an address
	// rather than a query parameter. See agent/slug.go.
	//
	// ?id= and ?agent= still resolve, because links to them exist and breaking a
	// URL to tidy a parameter is a bad trade — they redirect to the name, which
	// is one hop and leaves the address bar saying something true.
	http.HandleFunc("/agent", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			agent.Handler(w, r)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			id = r.URL.Query().Get("agent")
		}
		owner := ""
		if sess, _ := auth.TrySession(r); sess != nil {
			owner = sess.Account
		}
		to := agent.Path(owner, id)
		// Everything except the agent's name, which is in the path now.
		rest := r.URL.Query()
		rest.Del("id")
		rest.Del("agent")
		if q := rest.Encode(); q != "" {
			to += "?" + q
		}
		http.Redirect(w, r, to, http.StatusFound)
	})
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
	// The basemap under anything spatial. /maps is the page; the images are at
	// /tiles/<style>/<z>/<x>/<y>.png, which is the shape every map library
	// takes and the only shape any of them take. See service/maps.
	//
	// Everything the service serves lives under the service's route, which is
	// what "service name == route" means and what /apps/<slug>/icon.svg already
	// does. The tile path was left at /tiles when the service was renamed, on
	// the argument that map configs somewhere point at it — a guess about a
	// service days old, which bought a top-level route with no service behind
	// it.
	//
	// /tiles/ still answers, permanently redirected. Browsers and map libraries
	// cache a 301, so a config that has not been updated costs one extra round
	// trip and then stops costing anything.
	http.HandleFunc("/maps", maps.Handler)
	http.HandleFunc("/maps/", maps.TileHandler)
	// What you have, and the key that can spend it. One destination: x402 is a
	// substrate and a substrate does not get a page of its own, the same way
	// SMTP has none and its connect details are a section under /inbox.
	http.HandleFunc("/wallet", account.Wallet)
	// Taking your key with you. A page action and never a tool: an agent that
	// can read a private key is a prompt injection away from posting it
	// somewhere. It re-checks the password, so it is not in authRequired()
	// either — the handler wants the session *and* the password.
	//
	// Registered before the /wallet/ prefix below and matched ahead of it:
	// net/http picks the longest matching pattern, and this is an exact one.
	http.HandleFunc("/wallet/export", wallet.ExportHandler)
	// Turning USDC in the wallet into credits — the one thing the crypto card
	// could not do, so money sent to your own address here bought nothing here.
	// Its own route rather than a case in BalanceHandler because it is a POST
	// that moves money on a chain, and the CSRF and method checks want to be
	// obvious rather than nested three switches deep.
	http.HandleFunc("/wallet/convert", account.ConvertUSDC)
	// The money actions. account/ owns them because it owns the ledger.
	http.HandleFunc("/wallet/", account.BalanceHandler)
	http.HandleFunc(imageproxy.Path, imageproxy.Handler)
	// Who is here. See service/users: the directory that did not exist, so a
	// person could sign up alongside a hundred and eighty others and meet none
	// of them.
	http.HandleFunc("/users", users.Handler)
	http.HandleFunc("/contacts", contacts.Handler)
	http.HandleFunc("/docs", docs.Handler)
	http.HandleFunc("/notes", notes.Handler)
	http.HandleFunc("/notify", notify.Handler)
	http.HandleFunc("/sms", sms.Handler)

	// Twilio posts everything arriving on a Messaging Service to one webhook,
	// so both paths answer and whichever is configured works.
	http.HandleFunc("/whatsapp/twilio", sms.WebhookHandler)
	http.HandleFunc("/sms/webhook", sms.WebhookHandler)
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

	// JSON only. The page is /services/weather; this no longer bounces there.
	http.HandleFunc("/weather", weather.Handler)
	http.HandleFunc("/prayer", prayer.Handler)

	// serve places page
	http.HandleFunc("/places", places.Handler)
	http.HandleFunc("/places/", places.Handler)

	// serve weather page

	// serve flights page
	http.HandleFunc("/flights", flights.Handler)

	// serve routes page
	http.HandleFunc("/routes", routes.Handler)

	// serve apps
	http.HandleFunc("/apps", apps.Handler)
	http.HandleFunc("/apps/", apps.Handler)

	// serve work (task bounties)

	// content controls (flag, save, dismiss, block, share)

	// auth
	http.HandleFunc("/login", account.Login)
	http.HandleFunc("/logout", account.Logout)
	http.HandleFunc("/signup", account.Signup)
	http.HandleFunc("/request-invite", account.RequestInvite)
	http.HandleFunc("/invite", account.InviteHandler)
	http.HandleFunc("/report", app.ReportHandler)
	// What you are doing, set on your own profile. See internal/user/status.go.
	http.HandleFunc("/profile/status", user.StatusHandler)
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

	http.HandleFunc("/admin/oauth", admin.OAuthHandler)
	http.HandleFunc("/oauth/register", auth.OAuthRegisterHandler)
	http.HandleFunc("/oauth/authorize", auth.OAuthAuthorizePostHandler)
	http.HandleFunc("/oauth/token", auth.OAuthTokenHandler)

	// internal status (injected into admin server page)
	app.DKIMStatusFunc = mail.DKIMStatus
	app.DigestStatusFunc = digest.Status

	// public status page - service health checks
	app.HealthCheckFunc = runHealthChecks
	http.HandleFunc("/status", app.StatusHandler)

	// Documentation. One page: how to run your own. Every address the old nine
	// answered on redirects to whatever replaced it — an exact pattern outranks
	// the /docs the service owns now, and /about is in that map pointing at the
	// landing, which is the page that answers the question it used to.
	http.HandleFunc("/install", help.InstallHandler)

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
	// /tools/<name> — one tool. The smallest unit in the catalogue, and until
	// now the only one with no page: clicking a tool jumped to a fragment on
	// the playground. See internal/api/tool_page.go.
	http.HandleFunc("/tools/", api.ToolPageHandler)
	http.HandleFunc("/services", api.ToolsPageHandler)
	// /services/<name> — one service as the thing you call: what it knows right
	// now, every method with its arguments and its price, and a form that makes
	// the call for real. See internal/api/service_ref.go.
	http.HandleFunc("/services/", api.ServiceRefHandler)
	// What your agents did. Flows were recorded and never served.
	// Runs belong to the agent, so they live under it and the agent surface
	// tabs between them. /runs still works — links to it exist.
	http.HandleFunc("/agent/connect", agent.ConnectHandler)
	// Search everything you have ever said to an agent. The list of your
	// conversations is /agent; this is the search over all of them.
	http.HandleFunc("/recall", recall.Handler)
	// And the search over what the instance has collected, which is the other
	// archive and belongs to nobody.
	http.HandleFunc("/archive", archive.Handler)
	// Deleting a conversation. The list of them is the rail on /agent.
	http.HandleFunc("/agent/session/", inbox.SessionHandler)
	// Putting a conversation back to unread. Its own path, because /inbox's POST
	// is the instruction box and the two are not variations of each other.
	http.HandleFunc("/inbox/unread", inbox.UnreadHandler)
	http.HandleFunc("/inbox/delete", inbox.DeleteHandler)
	http.HandleFunc("/inbox/held", inbox.HeldHandler)
	// Writing one yourself. An exact route, because
	// /inbox/<box> is a mailbox name and a box could be called anything.
	http.HandleFunc("/inbox/new", inbox.NewHandler)
	// What to type into a mail client, for the same reason and by the same
	// rule: an exact route, because a box could be called imap.
	http.HandleFunc("/inbox/imap", inbox.ImapHandler)
	// Where you are, which every specialist needs and nothing server-side
	// held — see account/place.go.
	http.HandleFunc("/account/place", account.PlaceHandler)
	// Telling a device something happened while the page is closed. Two
	// endpoints, one handler — see internal/push.
	http.HandleFunc("/push/subscribe", push.SubscribeHandler)
	http.HandleFunc("/push/unsubscribe", push.SubscribeHandler)
	http.HandleFunc("/push/test", push.SubscribeHandler)
	// The device reporting back that it woke up holding one. See internal/push.
	http.HandleFunc("/push/received", push.SubscribeHandler)
	// Two pages that were tabs and are not any more: one listed the same
	// conversations the rail lists, the other listed the workflow records behind
	// them. Both are on /agent now — the conversation, and the tools each answer
	// came from beside the answer.
	// /runs, /agent/runs and /agent/threads are gone. They were three routes
	// redirecting to /inbox — the remains of a page listing every workflow
	// record, prompt by prompt, which was a fourth name on a surface that
	// already had three. A redirect kept for links that existed is worth
	// keeping; three of them for one deleted page, long after anything linked
	// to it, is a route table remembering something nobody does.
	// A service rendered at a glance — see internal/api/card.go.
	http.HandleFunc("/card", api.CardHandler)
	http.HandleFunc("/card/", api.CardHandler)
	// Your own usage — the caller-facing half of /admin/traffic.
	http.HandleFunc("/usage", home.UsageHandler)
	http.HandleFunc("/mcp", api.MCPHandler)

	// serve the app
	http.Handle("/", app.Serve())
}
