package home

import (
	"mu/web"
	"net/http"
)

func AboutHandler(w http.ResponseWriter, r *http.Request) {
	if !web.Page(w, r, "About Micro") {
		http.NotFound(w, r)
	}
}
