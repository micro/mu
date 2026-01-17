package main

import (
	"context"
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
	"mu/api"
	"mu/app"
	"mu/apps"
	"mu/auth"
	"mu/blog"
	"mu/chat"
	"mu/data"
	"mu/docs"
	"mu/home"
	"mu/mail"
	"mu/news"
	"mu/user"
	"mu/video"
	"mu/wallet"
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

	// load the home cards
	home.Load()

	// load user presence tracking
	user.Load()

	// load micro apps
	apps.Load()

	// Enable indexing after all content is loaded
	// This allows the priority queue to process new items first
	data.StartIndexing()

	authenticated := map[string]bool{
		"/video":           false, // Public viewing, auth for interactive features
		"/news":            false, // Public viewing, auth for search
		"/chat":            false, // Public viewing, auth for chatting
		"/home":            false, // Public viewing
		"/blog":            false, // Public viewing, auth for posting
		"/mail":            true,  // Require auth for inbox
		"/logout":          true,
		"/account":         true,
		"/token":           true,  // PAT token management
		"/session":         false, // Public - used to check auth status
		"/api":             true,
		"/flag":            true,
		"/admin":           true,
		"/admin/moderate":  true,
		"/admin/blocklist": true,
		"/membership":      false,
		"/plans":           false, // Public - shows pricing options
		"/donate":          false,
		"/wallet":          true,  // Require auth for wallet
		"/apps":            false, // Public viewing, auth for creating
		"/agent":           true,  // Require auth for agent
		"/status":          false, // Public - server health status
		"/docs":            false, // Public - documentation
		"/about":           false, // Public - about page
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
	http.HandleFunc("/post", blog.PostHandler)

	// handle comments on posts /post/{id}/comment
	http.HandleFunc("/post/", blog.CommentHandler)

	// flag content
	http.HandleFunc("/flag", admin.FlagHandler)

	// admin user management
	http.HandleFunc("/admin", admin.AdminHandler)

	// moderation queue
	http.HandleFunc("/admin/moderate", admin.ModerateHandler)

	// mail blocklist management
	http.HandleFunc("/admin/blocklist", admin.BlocklistHandler)

	// membership page (public - handles GoCardless redirects)
	http.HandleFunc("/membership", app.Membership)

	// plans page (public - overview of options)
	http.HandleFunc("/plans", app.Plans)

	// donate page (public - handles GoCardless redirects)
	http.HandleFunc("/donate", app.Donate)

	// wallet - credits and payments
	http.HandleFunc("/wallet", wallet.Handler)
	http.HandleFunc("/wallet/", wallet.Handler) // Handle sub-routes like /wallet/topup

	// serve micro apps
	http.HandleFunc("/apps", apps.Handler)
	http.HandleFunc("/apps/", apps.Handler)
	http.HandleFunc("/apps/api", apps.HandleAPIRequest)

	// serve agent
	http.HandleFunc("/agent", agent.Handler)
	http.HandleFunc("/agent/", agent.Handler)

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

	// serve mail inbox
	http.HandleFunc("/mail", mail.Handler)

	http.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://coinmarketcap.com/", 302)
	})

	// auth
	http.HandleFunc("/login", app.Login)
	http.HandleFunc("/logout", app.Logout)
	http.HandleFunc("/signup", app.Signup)
	http.HandleFunc("/account", app.Account)
	http.HandleFunc("/session", app.Session)
	http.HandleFunc("/token", app.TokenHandler)

	// status page - public health check
	app.DKIMStatusFunc = mail.DKIMStatus
	http.HandleFunc("/status", app.StatusHandler)

	// documentation
	http.HandleFunc("/docs", docs.Handler)
	http.HandleFunc("/docs/", docs.Handler)
	http.HandleFunc("/about", docs.AboutHandler)

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

	// serve the app
	http.Handle("/", app.Serve())

	// Create server with handler
	server := &http.Server{
		Addr: *AddressFlag,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						http.Redirect(w, r, "/", 302)
						return
					}
				} else if r.URL.Path == "/" {
					if err := auth.ValidateToken(token); err == nil {
						http.Redirect(w, r, "/home", 302)
						return
					}
				}
			}

			// Check if this is a user profile request (/@username)
			if strings.HasPrefix(r.URL.Path, "/@") && !strings.Contains(r.URL.Path[2:], "/") {
				user.Handler(w, r)
				return
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
