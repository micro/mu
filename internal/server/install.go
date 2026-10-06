package server

import "net/http"

// Keep the script maintained with the software; curl -L follows this alias.
func installScriptHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "https://raw.githubusercontent.com/micro/mu/main/install.sh", http.StatusTemporaryRedirect)
}
