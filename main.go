package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mu/admin"
	"mu/api"
	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/chat"
	"mu/data"
	"mu/home"
	"mu/news"
	"mu/user"
	"mu/video"
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

	// load the home cards
	home.Load()

	authenticated := map[string]bool{
		"/video":      true,
		"/news":       true,
		"/chat":       true,
		"/posts":      true,
		"/home":       true,
		"/logout":     true,
		"/account":    true,
		"/session":    true,
		"/api":        true,
		"/flag":       true,
		"/moderate":   true,
		"/admin":      true,
		"/membership": false,
		"/donate":     false,
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
	http.HandleFunc("/posts", blog.Handler)

	// serve individual blog post (public, no auth)
	http.HandleFunc("/post", blog.PostHandler)

	// flag content
	http.HandleFunc("/flag", admin.FlagHandler)

	// moderation queue
	http.HandleFunc("/moderate", admin.ModerateHandler)

	// admin user management
	http.HandleFunc("/admin", admin.AdminHandler)

	// membership page (public - handles GoCardless redirects)
	http.HandleFunc("/membership", app.Membership)

	// donate page (public - handles GoCardless redirects)
	http.HandleFunc("/donate", app.Donate)

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

	http.HandleFunc("/mail", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/home", 302)
	})

	http.HandleFunc("/markets", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://coinmarketcap.com/", 302)
	})

	// auth
	http.HandleFunc("/login", app.Login)
	http.HandleFunc("/logout", app.Logout)
	http.HandleFunc("/signup", app.Signup)
	http.HandleFunc("/account", app.Account)
	http.HandleFunc("/session", app.Session)

	// presence ping endpoint
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		sess, err := auth.GetSession(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		acc, err := auth.GetAccount(sess.Account)
		if acc == nil || err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

		// set via session
		if c, err := r.Cookie("session"); err == nil && c != nil {
			token = c.Value
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

			// Special case: /post should be public, not confused with /posts
			if strings.HasPrefix(r.URL.Path, "/post") && !strings.HasPrefix(r.URL.Path, "/posts") {
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
			user.Profile(w, r)
			return
		}

		http.DefaultServeMux.ServeHTTP(w, r)
	}),
	}

	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

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
