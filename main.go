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
)

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8080", "Address for server")

func main() {
	flag.Parse()

	if !*ServeFlag {
		fmt.Errorf("--serve not set")
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

	// load social discussions
	social.Load()

	// load mini apps
	apps.Load()

	// load the home cards
	home.Load()

	// load agent
	agent.Load()

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

	// Wire admin → blog callbacks (avoids blog importing admin)
	admin.GetNewAccountBlog = blog.GetNewAccountBlogPosts
	admin.RefreshBlogCache = blog.RefreshCache

	// Enable indexing after all content is loaded
	// This allows the priority queue to process new items first
	data.StartIndexing()

	// Start web search topics (loads cache from disk, generates in background)
	search.StartTopics()

	// Wire news → digest callback (avoids circular import: digest imports news)
	news.HasDigest = func() bool {
		return digest.GetLatestDigest() != nil
	}

	// Wire social seed callbacks (avoids social importing blog/digest directly)
	social.GetOpinionSeed = func() *social.SeedData {
		post := blog.FindTodayOpinion()
		if post == nil {
			return nil
		}
		summary := post.Content
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		return &social.SeedData{
			Title:   post.Title,
			Summary: summary,
			Link:    "/post?id=" + post.ID,
		}
	}
	social.GetDigestSeed = func() *social.SeedData {
		d := digest.GetTodayDigest()
		if d == nil {
			return nil
		}
		summary := d.Content
		if len(summary) > 200 {
			summary = summary[:200] + "..."
		}
		return &social.SeedData{
			Title:   d.Title,
			Summary: summary,
			Link:    "/news/digest?date=" + d.ID,
		}
	}

	// Start social discussion seeding (3 daily threads: reminder, opinion, digest)
	social.SeedingEnabled = true
	social.StartSeeding()

	// Start daily opinion generation (publishes as blog post)
	blog.StartOpinion()

	// Wire MCP quota checking using wallet credit system
	api.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		sess, err := auth.GetSession(r)
		if err != nil {
			return false, 0, fmt.Errorf("authentication required")
		}
		canProceed, _, cost, err := wallet.CheckQuota(sess.Account, op)
		return canProceed, cost, err
	}

	// Wire agent quota checking (same wallet credit system)
	agent.QuotaCheck = func(r *http.Request, op string) (bool, int, error) {
		sess, err := auth.GetSession(r)
		if err != nil {
			return false, 0, fmt.Errorf("authentication required")
		}
		canProceed, _, cost, err := wallet.CheckQuota(sess.Account, op)
		return canProceed, cost, err
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
		Path:        "/fetch",
		WalletOp:    "web_fetch",
		Params: []api.ToolParam{
			{Name: "url", Type: "string", Description: "The URL to fetch", Required: true},
		},
	})

	// fact_check tool — verify claims against web sources
	api.RegisterTool(api.Tool{
		Name:        "fact_check",
		Description: "Fact-check a claim or statement by searching the web and assessing accuracy. Returns verdict (accurate, misleading, missing_context, none) with sources.",
		Method:      "POST",
		Path:        "/factcheck",
		WalletOp:    "fact_check",
		Params: []api.ToolParam{
			{Name: "claim", Type: "string", Description: "The claim or statement to fact-check (minimum 20 characters)", Required: true},
		},
	})

	// Register mini apps MCP tools
	api.RegisterTool(api.Tool{
		Name:        "apps_search",
		Description: "Search the mini apps directory for small, useful tools",
		Method:      "GET",
		Path:        "/apps",
		Params: []api.ToolParam{
			{Name: "q", Type: "string", Description: "Search query (name, description, or category)", Required: false},
			{Name: "category", Type: "string", Description: "Filter by category (Productivity, Tools, Finance, Writing, Health, Education, Fun, Developer)", Required: false},
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_read",
		Description: "Read details of a specific mini app by its slug",
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
		Description: "Create a new mini app — a small, self-contained HTML tool hosted on Mu",
		Method:      "POST",
		Path:        "/apps/new",
		Params: []api.ToolParam{
			{Name: "name", Type: "string", Description: "App name (e.g. Pomodoro Timer)", Required: true},
			{Name: "slug", Type: "string", Description: "URL-friendly ID (e.g. pomodoro-timer)", Required: true},
			{Name: "description", Type: "string", Description: "Short description of what the app does", Required: true},
			{Name: "category", Type: "string", Description: "Category: Productivity, Tools, Finance, Writing, Health, Education, Fun, Developer", Required: true},
			{Name: "html", Type: "string", Description: "The app's HTML content (can include inline CSS and JavaScript, max 256KB)", Required: true},
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_build",
		Description: "AI-generate a mini app from a natural language description. Returns the generated HTML.",
		Method:      "POST",
		Path:        "/apps/build/generate",
		WalletOp:    "chat_query",
		Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Description of the app to build (e.g. 'a pomodoro timer with lap counter')", Required: true},
		},
	})

	authenticated := map[string]bool{
		"/video":           false, // Public viewing, auth for interactive features
		"/news":            false, // Public viewing, auth for search
		"/news/digest":     false, // Public - daily digest
		"/chat":            false, // Public viewing, auth for chatting
		"/home":            false, // Public viewing
		"/blog":            false, // Public viewing, auth for posting
		"/social":          false, // Public viewing, auth for posting
		"/markets":         false, // Public viewing
		"/reminder":        false, // Public viewing
		"/places":          false, // Public map, auth for search
		"/weather":         false, // Public page, auth for forecast lookup
		"/mail":            true,  // Require auth for inbox
		"/logout":          true,
		"/account":         true,
		"/token":           true,  // PAT token management
		"/passkey":         false, // Passkey login/register (auth checked in handler)
		"/session":         false, // Public - used to check auth status
		"/api":             false, // Public - API documentation
		"/flag":            true,
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
		"/plans":           false, // Public - shows pricing options
		"/donate":          false,
		"/wallet":          false, // Public - shows wallet info; auth checked in handler

		"/apps":      false, // Public - mini apps directory; auth checked in handler for create/edit
		"/search":    false, // Public - local data index search
		"/web":       false, // Public page, auth checked in handler (paid Brave web search)
		"/fetch":     false, // Public page, auth checked in handler (paid web fetch)
		"/factcheck": false, // Public page, auth checked in handler (paid fact-check)

		"/status": false, // Public - server health status
		"/docs":   false, // Public - documentation
		"/about":  false, // Public - about page
		"/mcp":    false, // Public - MCP tools page
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
	http.HandleFunc("/news/digest", digest.Handler)

	// serve chat
	http.HandleFunc("/chat", chat.Handler)

	// serve social discussions
	http.HandleFunc("/social", social.Handler)
	http.HandleFunc("/social/guidelines", social.GuidelinesHandler)
	http.HandleFunc("/social/dismiss", social.DismissHandler)

	// serve blog (full list)
	http.HandleFunc("/blog", blog.Handler)

	// serve individual blog post (public, no auth)
	// Serves ActivityPub JSON-LD when requested via Accept header
	http.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && blog.WantsActivityPub(r) {
			blog.PostObjectHandler(w, r)
			return
		}
		blog.PostHandler(w, r)
	})

	// handle comments on posts /post/{id}/comment
	http.HandleFunc("/post/", blog.CommentHandler)

	// flag content
	http.HandleFunc("/flag", admin.FlagHandler)

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

	// plans page (public - overview of options)
	http.HandleFunc("/plans", app.Plans)

	// donate page (public - handles GoCardless redirects)
	http.HandleFunc("/donate", app.Donate)

	// wallet - credits and payments
	http.HandleFunc("/wallet", wallet.Handler)
	http.HandleFunc("/wallet/", wallet.Handler) // Handle sub-routes like /wallet/topup

	// serve search page (local + Brave web search)
	http.HandleFunc("/search", search.Handler)

	// serve web search page (Brave-powered, paid)
	http.HandleFunc("/web", search.WebHandler)
	http.HandleFunc("/web/preview", search.PreviewHandler)

	// serve web fetch page (fetch and clean a URL)
	http.HandleFunc("/fetch", search.FetchHandler)

	// serve fact-check page and API
	http.HandleFunc("/factcheck", social.FactCheckPageHandler)

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

	// serve the agent
	http.HandleFunc("/agent", agent.Handler)
	http.HandleFunc("/agent/", agent.Handler) // Handle sub-routes like /agent/flow/...

	// serve mail inbox
	http.HandleFunc("/mail", mail.Handler)

	// serve markets page
	http.HandleFunc("/markets", markets.Handler)

	// serve reminder page
	http.HandleFunc("/reminder", reminder.Handler)

	// serve places page
	http.HandleFunc("/places", places.Handler)
	http.HandleFunc("/places/", places.Handler)

	// serve weather page
	http.HandleFunc("/weather", weather.Handler)

	// serve mini apps
	http.HandleFunc("/apps", apps.Handler)
	http.HandleFunc("/apps/", apps.Handler)

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

	// documentation
	http.HandleFunc("/docs", docs.Handler)
	http.HandleFunc("/docs/", docs.Handler)
	http.HandleFunc("/about", docs.AboutHandler)

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

				// Special case: /post should be public, not confused with /blog
				if strings.HasPrefix(r.URL.Path, "/post") && !strings.HasPrefix(r.URL.Path, "/blog") {
					isAuthed = false
				} else {
					// Check if path requires authentication
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
						// Return JSON 401 for API-style requests
						if app.SendsJSON(r) || app.WantsJSON(r) {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusUnauthorized)
							w.Write([]byte(`{"error":"Authentication required"}`))
							return
						}
						http.Redirect(w, r, "/", 302)
						return
					}
				} else if r.URL.Path == "/" {
					if err := auth.ValidateToken(token); err == nil {
						home.StreamHandler(w, r)
						return
					}
					// Serve dynamic landing page for unauthenticated users
					home.LandingHandler(w, r)
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
		fn   func() bool
	}{
		{"News", func() bool { return len(news.GetFeed()) > 0 }},
		{"Blog", func() bool { return blog.GetTopics() != nil }},
		{"Video", func() bool { return video.GetLatestVideos(1) != nil }},
		{"Chat", func() bool { return os.Getenv("ANTHROPIC_API_KEY") != "" }},
		{"Mail", func() bool { return os.Getenv("MAIL_DOMAIN") != "" }},
		{"Markets", func() bool { return len(markets.GetAllPrices()) > 0 }},
	}

	results := make([]app.ServiceHealth, len(checks))
	ch := make(chan result, len(checks))

	for i, c := range checks {
		go func(idx int, name string, fn func() bool) {
			start := time.Now()
			ok := fn()
			latency := time.Since(start).Round(time.Millisecond).String()
			ch <- result{idx, app.ServiceHealth{Name: name, Status: ok, Latency: latency}}
		}(i, c.name, c.fn)
	}

	for range checks {
		r := <-ch
		results[r.index] = r.check
	}

	return results
}
