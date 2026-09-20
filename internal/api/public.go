package api

// Public operations describe outcomes. Service tools remain the agent's internal
// implementation and the app bridge's capability catalogue.
import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/usage"
)

type Operation struct {
	Name        string                                     `json:"name"`
	Description string                                     `json:"description"`
	Params      []ToolParam                                `json:"parameters"`
	Writes      bool                                       `json:"writes"`
	Handle      func(string, json.RawMessage) (any, error) `json:"-"`
}

// Operations is assembled at startup by internal/server, never by services.
var Operations []Operation

type Failure struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Failure) Error() string                  { return e.Message }
func Fail(status int, code, message string) error { return &Failure{status, code, message} }
func failure(err error) *Failure {
	var e *Failure
	if errors.As(err, &e) {
		return e
	}
	return &Failure{500, "internal_error", "The operation failed. Check its state before retrying."}
}
func Decode(raw json.RawMessage, target any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return Fail(400, "invalid_arguments", err.Error())
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Fail(400, "invalid_arguments", "Send one JSON object")
	}
	return nil
}
func operation(name string) *Operation {
	for i := range Operations {
		if Operations[i].Name == name {
			return &Operations[i]
		}
	}
	return nil
}
func permitted(t *auth.Token, op *Operation) bool {
	if t == nil {
		return true
	}
	if op.Writes && !t.HasPermission("write") {
		return false
	}
	if !op.Writes && !t.HasPermission("read") {
		return false
	}
	if !t.Scoped() {
		return true
	}
	// Service-scoped credentials never gain authority to run an agent.
	for _, p := range t.Permissions {
		if strings.HasPrefix(p, auth.ScopePrefix) {
			return false
		}
	}
	for _, p := range t.Permissions {
		if p == "api:"+strings.SplitN(op.Name, "_", 2)[0] {
			return true
		}
	}
	return false
}

// CredentialRequest makes an explicit credential select the identity. Never let a browser cookie
// silently override a token belonging to another account or rescue an invalid key.
func CredentialRequest(r *http.Request) *http.Request {
	if r.Header.Get("Authorization") == "" && r.Header.Get(TokenHeader) == "" {
		return r
	}
	clone := r.Clone(r.Context())
	clone.Header.Del("Cookie")
	h := strings.TrimSpace(clone.Header.Get("Authorization"))
	if clone.Header.Get("Authorization") != "" {
		clone.Header.Del(TokenHeader)
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		clone.Header.Set("Authorization", "Bearer "+strings.TrimSpace(h[7:]))
	}
	return clone
}

// Call executes a public operation with the same identity and policy on every carrier.
func Call(r *http.Request, name string, args map[string]any) (any, error) {
	op := operation(name)
	if op == nil {
		return nil, Fail(404, "not_found", "Unknown public operation")
	}
	r = CredentialRequest(r)
	sess, acc, err := auth.RequireSession(r)
	if err != nil || acc == nil {
		return nil, Fail(401, "unauthenticated", "Authentication required")
	}
	if !permitted(auth.TokenFromRequest(r), op) {
		return nil, Fail(403, "forbidden", "This token does not grant this operation")
	}
	if r.Header.Get("Authorization") == "" && r.Header.Get(TokenHeader) == "" && op.Writes && !auth.StrictCSRF(r) {
		return nil, Fail(403, "csrf", "A session call needs X-CSRF-Token")
	}
	allowed := map[string]bool{}
	for _, p := range op.Params {
		allowed[p.Name] = true
		v, ok := args[p.Name]
		if p.Required && (!ok || v == nil || v == "") {
			return nil, Fail(400, "invalid_arguments", p.Name+" is required")
		}
	}
	for key := range args {
		if !allowed[key] {
			return nil, Fail(400, "invalid_arguments", "Unknown argument: "+key)
		}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, Fail(400, "invalid_arguments", "Invalid JSON arguments")
	}
	if op.Writes {
		if err := auth.CheckPostRate(sess.Account); err != nil {
			return nil, Fail(429, "rate_limited", err.Error())
		}
	}
	usage.Record("api", op.Name, sess.Account)
	return op.Handle(sess.Account, raw)
}
func writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	e := failure(err)
	if e.Status == 401 {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+app.BaseURL(r)+`/.well-known/oauth-protected-resource"`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	json.NewEncoder(w).Encode(map[string]any{"error": e})
}

// AuthorizeProduct applies product scopes before a resource handler serves a client.
func AuthorizeProduct(w http.ResponseWriter, r *http.Request, capability string, writes bool) bool {
	r = CredentialRequest(r)
	if _, _, err := auth.RequireSession(r); err != nil {
		writeFailure(w, r, Fail(401, "unauthenticated", "Authentication required"))
		return false
	}
	if !permitted(auth.TokenFromRequest(r), &Operation{Name: capability + "_resource", Writes: writes}) {
		writeFailure(w, r, Fail(403, "forbidden", "This token does not grant this operation"))
		return false
	}
	if writes && r.Header.Get("Authorization") == "" && r.Header.Get(TokenHeader) == "" && !auth.StrictCSRF(r) {
		writeFailure(w, r, Fail(403, "csrf", "A session write needs X-CSRF-Token"))
		return false
	}
	return true
}

// RespondOperation reuses domain operations from a resource's JSON representation.
func RespondOperation(w http.ResponseWriter, r *http.Request, name string, args map[string]any) {
	result, err := Call(r, name, args)
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	app.RespondJSON(w, result)
}

// JSONAction reads an explicit action from a resource's JSON request body.
func JSONAction(w http.ResponseWriter, r *http.Request, owner, defaultAction string) {
	var args map[string]any
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err := decoder.Decode(&args); err != nil || args == nil {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Send a JSON object"))
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Send one JSON object"))
		return
	}
	action := defaultAction
	if v, ok := args["action"]; ok {
		action, _ = v.(string)
		delete(args, "action")
	}
	op := operation(owner + "_" + action)
	if op == nil || !op.Writes {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Unknown action"))
		return
	}
	RespondOperation(w, r, op.Name, args)
}

// ServiceCallHandler is the first-party service playground, not a public API.
// A PAT (even an unscoped one) cannot use it. App JavaScript keeps its existing
// isolated SDK bridge, which applies its own per-app capabilities.
func ServiceCallHandler(w http.ResponseWriter, r *http.Request) {
	if headerCredential(r) {
		writeFailure(w, r, Fail(403, "forbidden", "Use the public API"))
		return
	}
	if _, _, err := auth.RequireSession(r); err != nil {
		writeFailure(w, r, Fail(401, "unauthenticated", "Sign in to use the playground"))
		return
	}
	if r.Method != http.MethodPost || !auth.StrictCSRF(r) {
		writeFailure(w, r, Fail(403, "csrf", "Use POST with X-CSRF-Token"))
		return
	}
	clone := r.Clone(r.Context())
	u := *r.URL
	clone.URL = &u
	clone.URL.Path = RESTPrefix + strings.TrimPrefix(r.URL.Path, "/services/call/")
	RESTHandler(w, clone)
}
