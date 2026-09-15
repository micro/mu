package home

import (
	"mu/internal/app"
	"mu/web"
	"net/http"
)

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") != "json" && web.Page(w, r, "Status") {
		return
	}
	app.StatusHandler(w, r)
}
