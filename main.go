package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"mu/api"
	"mu/app"
	"mu/auth"
	"mu/chat"
	"mu/home"
	"mu/news"
	"mu/video"
)

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ModelFlag = flag.String("model", "", "Set the model e.g Fanar, gpt-4o-mini, gemini-2.5-flash")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8080", "Address for server")

func main() {
	flag.Parse()

	if !*ServeFlag {
		fmt.Errorf("--serve not set")
		return
	}

	if md := *ModelFlag; len(md) > 0 {
		chat.DefaultModel = md
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

	// serve video
	http.HandleFunc("/video", video.Handler)

	// serve news
	http.HandleFunc("/news", news.Handler)

	// serve chat
	http.HandleFunc("/chat", chat.Handler)

	// serve the home screen
	http.HandleFunc("/home", home.Handler)

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

		if r.Method == "GET" {
			if len(token) == 0 {
				var secure bool

				if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
					secure = true
				}

				// set a new token
				http.SetCookie(w, &http.Cookie{
					Name:   "session",
					Value:  auth.GenerateToken(),
					Secure: secure,
				})
			} else {
				// deny access if invalid
				if err := auth.ValidateToken(token); err != nil {
					http.Error(w, "invalid token", 401)
					return
				}

				// got a valid token

				if r.URL.Path == "/" {
					// let's redirect to home
					http.Redirect(w, r, "/home", 302)
					return
				}
			}
		}

		// check for session
		if r.Method == "POST" {
			// if token is invalid throw 401
			if len(token) == 0 {
				http.Error(w, "invalid token", 401)
				return
			}

			// check the validity of the token
			// deny access if invalid
			if err := auth.ValidateToken(token); err != nil {
				http.Error(w, "invalid token", 401)
				return
			}
		}

		http.DefaultServeMux.ServeHTTP(w, r)
	})); err != nil {
		fmt.Printf("Server error: %v\n", err)
		return
	}
}
