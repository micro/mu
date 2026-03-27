package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"mu/admin"
	"mu/agent"
	"mu/apps"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/blog"
	"mu/chat"
	"mu/internal/data"
	"mu/docs"
	"mu/home"
	"mu/mail"
	"mu/news"
	"mu/news/digest"
	"mu/markets"
	"mu/reminder"
	"mu/places"
	"mu/search"
	"mu/social"
	"mu/user"
	"mu/video"
	"mu/wallet"
	"mu/weather"
	"mu/work"
)

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8080", "Address for server")

func main() {
	flag.Parse()

	if !*ServeFlag {
		fmt.Println("--serve not set")
		return
	}

	// render the api markdwon
	md := api.Markdown()
	apiDoc := app.Render([]byte(md))
	apiHTML := app.RenderHTML("API", "API documentation", string(apiDoc))

	// load the data index
	data.Load()

	// load admin/flags
	admin.Load()

	// load the chat
	chat.Load()

	// load the news
	news.Load()

	// load the videos
	video.Load()

	// load the blog
	blog.Load()

	// load the mail (also configures SMTP and DKIM)
	mail.Load()

	// load places
	places.Load()

	// load weather
	weather.Load()

	// load markets, reminder, wallet
	markets.Load()
	reminder.Load()
	wallet.Load()

	// load apps
	apps.Load()

	// load work (task bounties)
	work.Load()

	// Wire work → apps builder (avoids direct import between building blocks)
	work.BuildApp = func(prompt, authorID, authorName string) (string, string, error) {
		a, err := apps.BuildAndSave(prompt, authorID, authorName)
		if err != nil {
			return "", "", err
		}
		return a.Slug, a.Name, nil
	}
	work.ConsumeCredits = func(userID string, amount int) error {
		w := wallet.GetWallet(userID)
		if w.Balance < amount {
			return fmt.Errorf("insufficient credits (%d available, %d needed)", w.Balance, amount)
		}
		wallet.ConsumeQuota(userID, wallet.OpChatQuery)
		return nil
	}
	work.Notify = func(toUserID, subject, body string) {
		acc, err := auth.GetAccount(toUserID)
		if err != nil {
			return
		}
		mail.SendMessage("Mu", "micro", acc.Name, toUserID, subject, body, "", "")
	}

	// load social
	social.Load()

	// Wire social context into news article views
	news.FetchSocialContext = func(articleURL, articleContent string) string {
		ctx := social.FetchContext(articleURL, articleContent)
		return social.RenderContextHTML(ctx)
	}

	// load the home cards
	home.Load()

	// load agent
	agent.Load()

	// Wire digest → blog callbacks (digest publishes as blog post)
	digest.PublishBlogPost = func(title, content, author, authorID, tags string) (string, error) {
		err := blog.CreatePost(title, content, author, authorID, tags, false)
		if err != nil {
			return "", err
		}
		// Return the ID of the just-created post
		post := blog.FindTodayDigest()
		if post != nil {
			return post.ID, nil
		}
		return "", nil
	}
	digest.UpdateBlogPost = func(id, title, content, tags string) error {
		return blog.UpdatePost(id, title, content, tags, false)
	}
	digest.FindTodayBlogDigest = func() *digest.DigestPost {
		post := blog.FindTodayDigest()
		if post == nil {
			return nil
		}
		return &digest.DigestPost{
			ID:      post.ID,
			Title:   post.Title,
			Content: post.Content,
		}
	}

	// load daily digest scheduler
	digest.Load()

	// load search
	search.Load()

	// load docs
	docs.Load()

	// load user presence tracking
	user.Load()

	// Wire user → blog callback (avoids direct import between building blocks)
	user.GetUserPosts = func(authorName string) []user.UserPost {
		posts := blog.GetPostsByAuthor(authorName)
		result := make([]user.UserPost, len(posts))
		for i, p := range posts {
			result[i] = user.UserPost{
				ID:        p.ID,
				Title:     p.Title,
				Content:   p.Content,
				CreatedAt: p.CreatedAt,
				Private:   p.Private,
			}
		}
		return result
	}
	user.LinkifyContent = blog.Linkify
	user.GetUserApps = func(authorID string) []user.UserApp {
		appList := apps.GetAppsByAuthor(authorID)
		result := make([]user.UserApp, len(appList))
		for i, a := range appList {
			result[i] = user.UserApp{
				Slug:        a.Slug,
				Name:        a.Name,
				Description: a.Description,
				Icon:        a.Icon,
			}
		}
		return result
	}

	// Wire admin → blog callbacks (avoids blog importing admin)
	admin.GetNewAccountBlog = blog.GetNewAccountBlogPosts
	admin.RefreshBlogCache = blog.RefreshCache

	// Enable indexing after all content is loaded
	// This allows the priority queue to process new items first
	data.StartIndexing()

	// Start web search topics (loads cache from disk, generates in background)
	search.StartTopics()

	// Start daily opinion generation (publishes as blog post)
	blog.StartOpinion()

	// Wire MCP quota checking using wallet credit system
	api.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		// Check for x402 payment (bypasses auth + credits)
		if r.Context().Value(wallet.X402ContextKey) != nil {
			_, err := wallet.VerifyAndSettle(r, op, r.URL.Path)
			if err != nil {
				return false, 0, fmt.Errorf("x402 payment failed: %w", err)
			}
			return true, 0, nil
		}
		sess, err := auth.GetSession(r)
		if err != nil {
			return false, 0, fmt.Errorf("authentication required")
		}
		canProceed, _, cost, err := wallet.CheckQuota(sess.Account, op)
		return canProceed, cost, err
	}

	// Wire agent quota checking (same wallet credit system)
	agent.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		// Check for x402 payment (bypasses auth + credits)
		if r.Context().Value(wallet.X402ContextKey) != nil {
			_, err := wallet.VerifyAndSettle(r, op, r.URL.Path)
			if err != nil {
				return false, 0, fmt.Errorf("x402 payment failed: %w", err)
			}
			return true, 0, nil
		}
		sess, err := auth.GetSession(r)
		if err != nil {
			return false, 0, fmt.Errorf("authentication required")
		}
		canProceed, _, cost, err := wallet.CheckQuota(sess.Account, op)
		return canProceed, cost, err
	}

	// Wire x402 payment required response for MCP
	if wallet.X402Enabled() {
		api.PaymentRequiredResponse = wallet.WritePaymentRequired
	}

	// Register MCP auth tools
	api.RegisterTool(api.Tool{
		Name:        "signup",
		Description: "Create a new account and return a session token",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "Username (4-24 chars, lowercase, starts with letter)", Required: true},
			{Name: "secret", Type: "string", Description: "Password (minimum 6 characters)", Required: true},
			{Name: "name", Type: "string", Description: "Display name (optional, defaults to username)", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			id, _ := args["id"].(string)
			secret, _ := args["secret"].(string)
			name, _ := args["name"].(string)
			if id == "" || secret == "" {
				return "username and password are required", fmt.Errorf("missing fields")
			}
			if len(secret) < 6 {
				return "password must be at least 6 characters", fmt.Errorf("short password")
			}
			if name == "" {
				name = id
			}
			if err := auth.Create(&auth.Account{
				ID: id, Secret: secret, Name: name, Created: time.Now(),
			}); err != nil {
				return err.Error(), err
			}
			sess, err := auth.Login(id, secret)
			if err != nil {
				return "account created but login failed", err
			}
			return fmt.Sprintf(`{"token":"%s"}`, sess.Token), nil
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "login",
		Description: "Log in and return a session token for use in Authorization header",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "Username", Required: true},
			{Name: "secret", Type: "string", Description: "Password", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			id, _ := args["id"].(string)
			secret, _ := args["secret"].(string)
			if id == "" || secret == "" {
				return "username and password are required", fmt.Errorf("missing fields")
			}
			sess, err := auth.Login(id, secret)
			if err != nil {
				return "invalid username or password", err
			}
			return fmt.Sprintf(`{"token":"%s"}`, sess.Token), nil
		},
	})

	// web_search tool registered via MCP
	api.RegisterTool(api.Tool{
		Name:        "web_search",
		Description: "Search the web for current information and news",
		Method:      "GET",
		Path:        "/web",
		WalletOp:    "web_search",
		Params: []api.ToolParam{
			{Name: "q", Type: "string", Description: "Search query", Required: true},
		},
	})

	// web_fetch tool — fetch a URL and return cleaned readable content
	api.RegisterTool(api.Tool{
		Name:        "web_fetch",
		Description: "Fetch a web page and return its cleaned readable content (strips ads, popups, navigation)",
		Method:      "GET",
		Path:        "/web/fetch",
		WalletOp:    "web_fetch",
		Params: []api.ToolParam{
			{Name: "url", Type: "string", Description: "The URL to fetch", Required: true},
		},
	})

	// Register apps MCP tools
	api.RegisterTool(api.Tool{
		Name:        "apps_search",
		Description: "Search the apps directory for small, useful tools",
		Method:      "GET",
		Path:        "/apps",
		Params: []api.ToolParam{
			{Name: "q", Type: "string", Description: "Search query (name, description, or tag)", Required: false},
			{Name: "tag", Type: "string", Description: "Filter by tag", Required: false},
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_read",
		Description: "Read details of a specific app by its slug",
		Method:      "GET",
		Path:        "/apps",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "The app's URL slug (e.g. pomodoro-timer)", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			slug, _ := args["slug"].(string)
			if slug == "" {
				return `{"error":"slug is required"}`, fmt.Errorf("missing slug")
			}
			a := apps.GetApp(slug)
			if a == nil {
				return `{"error":"app not found"}`, fmt.Errorf("not found")
			}
			b, _ := json.Marshal(a)
			return string(b), nil
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_create",
		Description: "Create a new app — a small, self-contained HTML tool hosted on Mu",
		Method:      "POST",
		Path:        "/apps/new",
		Params: []api.ToolParam{
			{Name: "name", Type: "string", Description: "App name (e.g. Pomodoro Timer)", Required: true},
			{Name: "slug", Type: "string", Description: "URL-friendly ID (e.g. pomodoro-timer)", Required: true},
			{Name: "description", Type: "string", Description: "Short description of what the app does", Required: true},
			{Name: "tags", Type: "string", Description: "Comma-separated tags (optional)", Required: false},
			{Name: "html", Type: "string", Description: "The app's HTML content (can include inline CSS and JavaScript, max 256KB)", Required: true},
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_edit",
		Description: "Edit an existing app — update its name, description, tags, icon, or HTML code",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "The app's URL slug (e.g. pomodoro-timer)", Required: true},
			{Name: "name", Type: "string", Description: "New app name", Required: false},
			{Name: "description", Type: "string", Description: "New description", Required: false},
			{Name: "tags", Type: "string", Description: "New comma-separated tags", Required: false},
			{Name: "html", Type: "string", Description: "New HTML content (max 256KB)", Required: false},
			{Name: "icon", Type: "string", Description: "New SVG icon", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			slug, _ := args["slug"].(string)
			if slug == "" {
				return `{"error":"slug is required"}`, fmt.Errorf("missing slug")
			}
			name, _ := args["name"].(string)
			description, _ := args["description"].(string)
			tags, _ := args["tags"].(string)
			html, _ := args["html"].(string)
			icon, _ := args["icon"].(string)
			a, err := apps.UpdateApp(slug, name, description, tags, html, icon)
			if err != nil {
				return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
			}
			b, _ := json.Marshal(a)
			return string(b), nil
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_build",
		Description: "AI-generate an app from a natural language description, save it, and return the app details with URL",
		WalletOp:    "chat_query",
		Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Description of the app to build (e.g. 'a pomodoro timer with lap counter')", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			prompt, _ := args["prompt"].(string)
			if prompt == "" {
				return `{"error":"prompt is required"}`, fmt.Errorf("missing prompt")
			}
			a, err := apps.BuildAndSave(prompt, "agent", "Agent")
			if err != nil {
				return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
			}
			b, _ := json.Marshal(map[string]string{
				"name": a.Name,
				"slug": a.Slug,
				"url":  "/apps/" + a.Slug,
				"run":  "/apps/" + a.Slug + "/run",
			})
			return string(b), nil
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_run",
		Description: "Run JavaScript code in a sandboxed environment and return the result. Use for calculations, data processing, or any computation the user needs.",
		WalletOp:    "agent_query",
		Params: []api.ToolParam{
			{Name: "code", Type: "string", Description: "JavaScript code to execute. The code runs as a function body — use 'return' to output a value. Has access to mu.ai(), mu.fetch(), mu.store for platform features.", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			code, _ := args["code"].(string)
			if code == "" {
				return `{"error":"code is required"}`, fmt.Errorf("missing code")
			}
			id := apps.CreateRun(code, "agent")
			b, _ := json.Marshal(map[string]string{
				"id":  id,
				"url": "/apps/run?id=" + id,
				"run": "/apps/run?id=" + id + "&raw=1",
			})
			return string(b), nil
		},
	})

	authenticated := map[string]bool{
		"/video":           false, // Public viewing, auth for interactive features
		"/news":            false, // Public viewing, auth for search
		"/chat":            false, // Public viewing, auth for chatting
		"/home":            false, // Public viewing
		"/blog":            false, // Public viewing, auth for posting
		"/markets":         false, // Public viewing
		"/social":          false, // Public viewing, auth for search
		"/social/thread":   false, // Public thread view, auth for messaging
		"/places":          false, // Public map, auth for search
		"/weather":         false, // Public page, auth for forecast lookup
		"/mail":            true,  // Require auth for inbox
		"/logout":          true,
		"/account":         true,
		"/token":           true,  // PAT token management
		"/passkey":         false, // Passkey login/register (auth checked in handler)
		"/session":         false, // Public - used to check auth status
		"/api":             false, // Public - API documentation
		"/admin/flag":      true,
		"/admin":           true,
		"/admin/users":     true,
		"/admin/moderate":  true,
		"/admin/blocklist": true,
		"/admin/spam":      true,
		"/admin/email":     true,
		"/admin/api":       true,
		"/admin/log":       true,
		"/admin/env":       true,
		"/admin/server":    true,
		"/admin/usage":  true,
		"/admin/delete": true,
		"/wallet":          false, // Public - shows wallet info; auth checked in handler

		"/apps":      false, // Public - apps directory; auth checked in handler for create/edit
		"/work":      false, // Public - task bounties; auth checked in handler for post/claim
		"/search":    false, // Public - local data index search
		"/web":       false, // Public page, auth checked in handler (paid Brave web search)
		"/web/fetch": false, // Public page, auth checked in handler (paid web fetch)
		"/web/read":  false, // Public page, auth checked in handler (proxied reader)

		"/status":     false, // Public - server health status
		"/docs":       false, // Public - documentation
		"/whitepaper": false, // Public - whitepaper
		"/mcp":        false, // Public - MCP tools page
		"/agent":  false, // Public page, auth checked in handler
	}

	// Static assets should not require authentication
	staticPaths := []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg",
		".ico", ".webmanifest", ".json",
	}
	// serve video
	http.HandleFunc("/video", video.Handler)

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
	http.HandleFunc("/admin", admin.AdminHandler)

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

	// admin delete (any content type)
	http.HandleFunc("/admin/delete", admin.DeleteHandler)

	// wallet - credits and payments
	http.HandleFunc("/wallet", wallet.Handler)
	http.HandleFunc("/wallet/", wallet.Handler) // Handle sub-routes like /wallet/topup

	// serve search page (local + Brave web search)
	http.HandleFunc("/search", search.Handler)

	// serve web search page (Brave-powered, paid)
	http.HandleFunc("/web", search.WebHandler)
	http.HandleFunc("/web/preview", search.PreviewHandler)

	// serve web fetch page (fetch and clean a URL)
	http.HandleFunc("/web/fetch", search.FetchHandler)

	// serve clean reader page for web results
	http.HandleFunc("/web/read", search.ReadHandler)

	// serve fact-check page and API

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

	// serve the agent
	http.HandleFunc("/agent", agent.Handler)
	http.HandleFunc("/agent/", agent.Handler) // Handle sub-routes like /agent/flow/...

	// serve mail inbox
	http.HandleFunc("/mail", mail.Handler)

	// serve markets page
	http.HandleFunc("/markets", markets.Handler)

	// serve social page
	http.HandleFunc("/social", social.Handler)
	http.HandleFunc("/social/thread", social.ThreadHandler)
	http.HandleFunc("/user/status", user.StatusHandler)

	// redirect /reminder to reminder.dev
	http.HandleFunc("/reminder", reminder.Handler)

	// serve places page
	http.HandleFunc("/places", places.Handler)
	http.HandleFunc("/places/", places.Handler)

	// serve weather page
	http.HandleFunc("/weather", weather.Handler)

	// serve apps
	http.HandleFunc("/apps", apps.Handler)
	http.HandleFunc("/apps/", apps.Handler)

	// serve work (task bounties)
	http.HandleFunc("/work", work.Handler)
	http.HandleFunc("/work/", work.Handler)

	// content controls (flag, save, dismiss, block, share)
	http.HandleFunc("/app/", app.ControlsHandler)

	// auth
	http.HandleFunc("/login", app.Login)
	http.HandleFunc("/logout", app.Logout)
	http.HandleFunc("/signup", app.Signup)
	http.HandleFunc("/account", app.Account)
	http.HandleFunc("/session", app.Session)
	http.HandleFunc("/token", app.TokenHandler)
	http.HandleFunc("/passkey/", app.PasskeyHandler)

	// internal status (injected into admin server page)
	app.DKIMStatusFunc = mail.DKIMStatus
	app.DigestStatusFunc = digest.Status
	admin.GenerateDigestFunc = digest.Generate

	// public status page - service health checks
	app.HealthCheckFunc = runHealthChecks
	http.HandleFunc("/status", app.StatusHandler)

	// whitepaper
	http.HandleFunc("/whitepaper", docs.WhitepaperHandler)
	http.HandleFunc("/whitepaper.pdf", docs.WhitepaperHandler)

	// documentation
	http.HandleFunc("/docs", docs.Handler)
	http.HandleFunc("/docs/", docs.Handler)


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
		onlineCount := auth.GetOnlineCount()
		w.Write([]byte(fmt.Sprintf(`{"status":"ok","online":%d}`, onlineCount)))
	})

	// serve the api doc
	http.Handle("/api", app.ServeHTML(apiHTML))

	// serve the MCP page and server (GET = HTML page, POST = JSON-RPC)
	http.HandleFunc("/mcp", api.MCPHandler)

	// serve the app
	http.Handle("/", app.Serve())

	// Create server with handler
	server := &http.Server{
		Addr: *AddressFlag,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			if *EnvFlag == "dev" {
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

			// Fast path for static assets - skip all middleware
			for _, ext := range staticPaths {
				if strings.HasSuffix(r.URL.Path, ext) {
					http.DefaultServeMux.ServeHTTP(w, r)
					return
				}
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
						if wallet.X402Enabled() && wallet.HasPayment(r) && (app.SendsJSON(r) || app.WantsJSON(r)) {
							r = r.WithContext(context.WithValue(r.Context(), wallet.X402ContextKey, true))
						} else if app.SendsJSON(r) || app.WantsJSON(r) {
							// Return JSON 401 for API-style requests
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusUnauthorized)
							w.Write([]byte(`{"error":"Authentication required"}`))
							return
						} else {
							http.Redirect(w, r, "/", 302)
							return
						}
					}
				} else if r.URL.Path == "/" {
					http.Redirect(w, r, "/home", 302)
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

				// Otherwise serve the HTML profile page
				if !strings.Contains(rest, "/") {
					user.Handler(w, r)
					return
				}
			}

			// CSRF protection: set token cookie on every response,
			// validate on state-changing requests.
			auth.SetCSRFCookie(w, r)
			if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
				// Skip CSRF for API endpoints using Bearer/PAT auth (not cookie-based)
				isBearerAuth := r.Header.Get("Authorization") != "" || r.Header.Get("X-Micro-Token") != ""
				// Skip CSRF for MCP endpoint (uses its own auth)
				isMCP := r.URL.Path == "/mcp"
				// Skip CSRF for Stripe webhooks
				isWebhook := r.URL.Path == "/wallet/stripe/webhook"
				// Skip CSRF for login/signup (no session yet)
				isAuth := r.URL.Path == "/login" || r.URL.Path == "/signup" ||
					strings.HasPrefix(r.URL.Path, "/passkey/")
				// Skip CSRF for SMTP/ActivityPub inbound
				isInbound := strings.HasSuffix(r.URL.Path, "/inbox")

				if !isBearerAuth && !isMCP && !isWebhook && !isAuth && !isInbound && !auth.ValidCSRF(r) {
					http.Error(w, `{"error":"invalid CSRF token"}`, http.StatusForbidden)
					return
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

	// Start server in a goroutine
	go func() {
		app.Log("main", "Starting server on %s", *AddressFlag)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.Log("main", "Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-quit
	app.Log("main", "Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		app.Log("main", "Server forced to shutdown: %v", err)
	}

	app.Log("main", "Server stopped")
}

// runHealthChecks performs lightweight health checks on public-facing services
func runHealthChecks() []app.ServiceHealth {
	type result struct {
		index int
		check app.ServiceHealth
	}

	checks := []struct {
		name string
		path string
		fn   func() bool
	}{
		{"News", "/news", func() bool { return len(news.GetFeed()) > 0 }},
		{"Blog", "/blog", func() bool { return blog.GetTopics() != nil }},
		{"Video", "/video", func() bool { return video.GetLatestVideos(1) != nil }},
		{"Chat", "/chat", func() bool { return os.Getenv("ANTHROPIC_API_KEY") != "" }},
		{"Mail", "/mail", func() bool { return os.Getenv("MAIL_DOMAIN") != "" }},
		{"Markets", "/markets", func() bool { return len(markets.GetAllPrices()) > 0 }},
		{"Social", "/social", func() bool { return len(social.GetThreads()) > 0 }},
	}

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
