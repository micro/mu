package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"mu/api"
	"mu/app"
	"mu/auth"
	"mu/blog"
	"mu/chat"
	"mu/home"
	"mu/news"
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

	// load the chat
	chat.Load()

	// load the news
	news.Load()

	// load the videos
	video.Load()

	// load the blog
	blog.Load()

	authenticated := map[string]bool{
		"/video":   true,
		"/news":    true,
		"/chat":    true,
		"/blog":    true,
		"/home":    true,
		"/logout":  true,
		"/session": true,
		"/api":     true,
	}
	// serve video
	http.HandleFunc("/video", video.Handler)

	// serve news
	http.HandleFunc("/news", news.Handler)

	// serve chat
	http.HandleFunc("/chat", chat.Handler)

	// serve blog
	http.HandleFunc("/blog", blog.Handler)

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
	http.HandleFunc("/session", app.Session)

	// serve the api doc
	http.Handle("/api", app.ServeHTML(apiHTML))

	// serve the app
	http.Handle("/", app.Serve())

	fmt.Println("Starting server on", *AddressFlag)

	if err := http.ListenAndServe(*AddressFlag, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		var isAuthed bool

		for url, authed := range authenticated {
			if strings.HasPrefix(r.URL.Path, url) {
				isAuthed = authed
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

		http.DefaultServeMux.ServeHTTP(w, r)
	})); err != nil {
		fmt.Printf("Server error: %v\n", err)
		return
	}
}
