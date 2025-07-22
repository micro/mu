package mu

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/micro/mu/api"
	"github.com/micro/mu/app"
)

var EnvFlag = flag.String("env", "dev", "Set the environment")
var ServeFlag = flag.Bool("serve", false, "Run the server")
var AddressFlag = flag.String("address", ":8080", "Address for server")

// Handle registers an app handler
func Handle(path string, fn http.HandlerFunc) {
	http.HandleFunc(path, fn)
}

// Serve starts the server
func Serve() error {
	flag.Parse()

	if !*ServeFlag {
		return errors.New("--serve not set")
	}

	// serve the api
	http.Handle("/api/", api.Serve())

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

		http.DefaultServeMux.ServeHTTP(w, r)
	})); err != nil {
		fmt.Printf("Server error: %v\n", err)
		return err
	}

	return nil
}
