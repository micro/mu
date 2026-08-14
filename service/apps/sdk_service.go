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
//   - account-scoped services (mail, index, images, events) require a signed-in
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

	// Nothing irreversible, whatever the service.
	//
	// The allowlist above is per service and that is one level too coarse.
	// Scoped keeps an app out of mail, files, notes and the rest, but blog is
	// deliberately unscoped — reading a blog is public — while blog.Delete is
	// marked Destructive because it is not. So an app, which is public,
	// forkable and one click to open, could delete the viewer's posts by
	// calling mu.blog.delete. Nobody had to be tricked; opening the app was
	// enough.
	//
	// Destructive already names exactly this: an irreversible effect nobody
	// asked for. The agent honours it because the agent reads text strangers
	// wrote; an app *is* text a stranger wrote, which is the same argument with
	// one fewer step.
	if service.Destructive(svc, req.Method) {
		app.RespondError(w, http.StatusForbidden,
			"This action can only be taken by the person themselves, not by an app")
		return
	}

	// Bind identity server-side, on the context rather than in the arguments.
	// CallDynamic stamps it into the request and drops anything the app sent,
	// so an app can never read or write another user's data by naming them.
	args := req.Args
	if args == nil {
		args = map[string]any{}
	}

	ctx, cancel := context.WithTimeout(service.WithAccount(r.Context(), caller), sdkServiceTimeout)
	defer cancel()

	rsp, err := service.CallDynamic(ctx, svc, req.Method, args)
	if err != nil {
		app.RespondError(w, http.StatusBadRequest, err.Error())
		return
	}
	app.RespondJSON(w, rsp)
}

// sdkServiceAllowed reports whether apps may call a registered service.
//
// Apps are public, forkable and one click to open, and this dispatches with the
// viewer bound as the caller. So "any registered service" meant an app could
// call wallet.Charge and spend the person's credits, mail.Inbox and read their
// mail, db and read the records they keep for themselves — all as them, all
// without asking. Moving app code into a sandbox closed the raw-fetch door and
// left this one open, which made the rest of it theatre.
//
// The line is one that already exists: Scoped on a service's Spec declares that
// it holds one person's own things. contacts, db, events, files, images, mail,
// memory, tasks and wallet say so; news, markets, weather, blog, places,
// prayer, social, stream, video and web do not. An app gets the instance's
// data and not the viewer's.
//
// An app is not left without storage. mu.store and mu.db go through /sdk/store
// and /sdk/db, which are namespaced per app — its own records, not the
// account's — and those are unaffected.
//
// What this cannot yet express is an app that legitimately wants your tasks,
// with you agreeing to it. That needs a permission model and a prompt, and
// until there is one the default has to be no: an app you opened once should
// not be able to read your mail because nobody thought to stop it.
//
// "apps" is excluded on top, because app management is not something a
// sandboxed app should drive — it would let one rewrite or run another.
func sdkServiceAllowed(name string) bool {
	if name == "apps" {
		return false
	}
	if service.AccountScoped(name) {
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
			m := ep
			if _, after, ok := strings.Cut(ep, "."); ok {
				m = after
			}
			// Not advertised either. The refusal above is what enforces it; this
			// is so the SDK does not build an accessor for something that will
			// always say no.
			if service.Destructive(name, m) {
				continue
			}
			methods = append(methods, strings.ToLower(m))
		}
		out = append(out, svcInfo{Name: name, Methods: methods, RequiresLogin: scoped})
	}
	app.RespondJSON(w, out)
}
