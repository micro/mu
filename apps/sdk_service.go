package apps

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// sdkServiceTimeout bounds a single service call made from an app, so a slow
// upstream can't pin a request open indefinitely.
const sdkServiceTimeout = 30 * time.Second

// handleSDKService serves mu.service at /apps/{slug}/sdk/service. It lets an app
// call any registered Mu service by name — weather, places, news, markets and
// the rest — without the SDK having to hardcode a wrapper per service. Register
// a new service and every app can use it immediately.
//
// Security model, mirroring the agent's tool path:
//   - the service set is the live registry, so an app can only reach real,
//     registered capabilities;
//   - account-scoped services (mail, recall, images, events) require a signed-in
//     caller, and the account id is bound from the session — an app can never
//     name whose data it wants;
//   - any account_id supplied by the app is discarded before dispatch.
func handleSDKService(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method != "POST" {
		app.MethodNotAllowed(w, r)
		return
	}
	if !slugRe.MatchString(slug) {
		app.RespondError(w, http.StatusBadRequest, "Invalid app")
		return
	}

	var req struct {
		Service string         `json:"service"`
		Method  string         `json:"method"`
		Args    map[string]any `json:"args"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		app.RespondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	svc := strings.ToLower(strings.TrimSpace(req.Service))
	if svc == "" {
		app.RespondError(w, http.StatusBadRequest, "service is required")
		return
	}

	// Only registered services are reachable, and only the ones exposed to apps.
	if !sdkServiceAllowed(svc) {
		app.RespondError(w, http.StatusNotFound, "Unknown service")
		return
	}

	caller := ""
	if _, acc := auth.TrySession(r); acc != nil {
		caller = acc.ID
	}
	if service.AccountScoped(svc) && caller == "" {
		app.RespondError(w, http.StatusUnauthorized, "Sign in to use this service")
		return
	}

	// Bind identity server-side. Whatever the app sent for account_id is
	// dropped; a signed-in caller's id is injected. An app must never be able
	// to read or write another user's data by naming them.
	args := req.Args
	if args == nil {
		args = map[string]any{}
	} else {
		delete(args, "account_id")
	}
	if caller != "" {
		args["account_id"] = caller
	}

	ctx, cancel := context.WithTimeout(r.Context(), sdkServiceTimeout)
	defer cancel()

	rsp, err := service.CallDynamic(ctx, svc, req.Method, args)
	if err != nil {
		app.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	app.RespondJSON(w, rsp)
}

// sdkServiceAllowed reports whether apps may call a registered service. "apps"
// itself is excluded: app management is not something a sandboxed app should
// drive, and exposing it would let an app rewrite or run other apps.
func sdkServiceAllowed(name string) bool {
	if name == "apps" {
		return false
	}
	for _, s := range service.Services() {
		if s == name {
			return true
		}
	}
	return false
}

// handleSDKServices serves the catalogue at /apps/{slug}/sdk/services: which
// services an app can call and which methods each exposes, read from the live
// registry. The SDK uses it to build mu.<service>.<method>() accessors, so a
// newly registered service appears to apps with no SDK change.
func handleSDKServices(w http.ResponseWriter, r *http.Request, slug string) {
	if !slugRe.MatchString(slug) {
		app.RespondError(w, http.StatusBadRequest, "Invalid app")
		return
	}
	signedIn := false
	if _, acc := auth.TrySession(r); acc != nil {
		signedIn = true
	}

	type svcInfo struct {
		Name          string   `json:"name"`
		Methods       []string `json:"methods"`
		RequiresLogin bool     `json:"requires_login"`
	}
	out := []svcInfo{}
	for _, name := range service.Services() {
		if !sdkServiceAllowed(name) {
			continue
		}
		scoped := service.AccountScoped(name)
		if scoped && !signedIn {
			continue // don't advertise what this visitor can't call
		}
		eps, err := service.Endpoints(name)
		if err != nil {
			continue
		}
		methods := make([]string, 0, len(eps))
		for _, ep := range eps {
			if _, m, ok := strings.Cut(ep, "."); ok {
				methods = append(methods, strings.ToLower(m))
				continue
			}
			methods = append(methods, strings.ToLower(ep))
		}
		out = append(out, svcInfo{Name: name, Methods: methods, RequiresLogin: scoped})
	}
	app.RespondJSON(w, out)
}
