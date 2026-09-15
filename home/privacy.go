package home

import (
	"mu/web"
	"net/http"
)

func PrivacyHandler(w http.ResponseWriter, r *http.Request) {
	if !web.Page(w, r, "Privacy") {
		http.NotFound(w, r)
	}
}
