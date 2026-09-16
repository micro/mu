package home

import "net/http"

// Index shares the same command surface before and after sign-in.
func Index(w http.ResponseWriter, r *http.Request) { ConsoleHandler(w, r) }
