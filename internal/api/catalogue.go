package api

import (
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"net/http"
	"strings"
)

// CatalogueHandler derives browser reference data from the same service specs
// and tool schemas as the dispatcher. It contains no account data or card HTML.
func CatalogueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	rows := make([]map[string]any, 0)
	for _, s := range service.Specs() {
		rows = append(rows, map[string]any{"name": s.Name, "label": s.NavLabel(), "description": s.Description, "page": s.Page, "scoped": s.Scoped, "methods": restMethodsFor(s.Name)})
	}
	app.RespondJSON(w, rows)
}

// AppCallHandler is the browser transport for first-party apps. Public methods
// retain their existing guest access; ExecuteTool still enforces each Spec's
// identity, scope, quota and write requirements. Header credentials use the API.
func AppCallHandler(w http.ResponseWriter, r *http.Request) {
	if headerCredential(r) {
		app.RespondError(w, 403, "Use the public API")
		return
	}
	if r.Method != http.MethodPost || !auth.StrictCSRF(r) {
		app.RespondError(w, 403, "Use POST with X-CSRF-Token")
		return
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	clone.URL = &u
	clone.URL.Path = RESTPrefix + strings.TrimPrefix(r.URL.Path, "/client/call/")
	RESTHandler(w, clone)
}
