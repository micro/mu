package apps

import (
	"errors"
	"net/http"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

// handleSDKDB serves mu.db at /apps/{slug}/sdk/db. It is a thin wrapper over the
// shared userdb store, namespaced per app so each app's data is isolated. The
// owner is bound from the session, never the client.
func handleSDKDB(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	if !slugRe.MatchString(slug) {
		app.RespondError(w, http.StatusBadRequest, "Invalid app")
		return
	}
	_, acc := auth.TrySession(r)
	caller := ""
	if acc != nil {
		caller = acc.ID
	}
	dbDispatch(w, r, "apps/"+slug, caller)
}

// dbDispatch decodes a db request and runs it against the given namespace as the
// given caller. Shared by the app SDK and the MCP/REST db surface.
func dbDispatch(w http.ResponseWriter, r *http.Request, ns, caller string) {
	var req struct {
		Op         string                 `json:"op"`
		Collection string                 `json:"collection"`
		ID         string                 `json:"id"`
		Data       map[string]interface{} `json:"data"`
		Public     bool                   `json:"public"`
		Scope      string                 `json:"scope"`
		Where      map[string]interface{} `json:"where"`
		Sort       string                 `json:"sort"`
		Order      string                 `json:"order"`
		Limit      int                    `json:"limit"`
	}
	if err := app.DecodeJSON(r, &req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Op {
	case "create":
		rec, err := userdb.Create(ns, caller, req.Collection, req.Data, req.Public)
		respondDB(w, map[string]interface{}{"record": rec}, err)
	case "get":
		rec, err := userdb.Get(ns, caller, req.Collection, req.ID)
		respondDB(w, map[string]interface{}{"record": rec}, err)
	case "list":
		recs, err := userdb.List(ns, caller, req.Collection, req.Scope, req.Where, req.Sort, req.Order, req.Limit)
		respondDB(w, map[string]interface{}{"records": recs}, err)
	case "update":
		rec, err := userdb.Update(ns, caller, req.Collection, req.ID, req.Data, req.Public)
		respondDB(w, map[string]interface{}{"record": rec}, err)
	case "delete":
		err := userdb.Delete(ns, caller, req.Collection, req.ID)
		respondDB(w, map[string]string{"status": "ok"}, err)
	default:
		app.RespondError(w, http.StatusBadRequest, "Invalid operation. Use create, get, list, update or delete")
	}
}

// respondDB maps a userdb error to an HTTP status or returns the payload.
func respondDB(w http.ResponseWriter, payload interface{}, err error) {
	if err == nil {
		app.RespondJSON(w, payload)
		return
	}
	switch {
	case errors.Is(err, userdb.ErrAuth):
		app.RespondError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, userdb.ErrForbidden):
		app.RespondError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, userdb.ErrNotFound):
		app.RespondError(w, http.StatusNotFound, err.Error())
	default:
		app.RespondError(w, http.StatusBadRequest, err.Error())
	}
}
