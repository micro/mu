package main

import (
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/micro/mu/api"
	"github.com/micro/mu/app"
	"github.com/micro/mu/chat"
	"github.com/micro/mu/news"
	"github.com/micro/mu/video"
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

	// load the news
	news.Load()

	// serve video
	http.HandleFunc("/video", video.Handler)

	// serve news
	http.HandleFunc("/news", news.Handler)

	// serve chat
	http.HandleFunc("/chat", chat.Handler)

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

		http.DefaultServeMux.ServeHTTP(w, r)
	})); err != nil {
		fmt.Printf("Server error: %v\n", err)
		return
	}
}
