package api

// Public operations describe outcomes. Service tools remain the agent's internal
// implementation and the app bridge's capability catalogue.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strings"

	gwmcp "go-micro.dev/v6/gateway/mcp"
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
	if r.Header.Get("Authorization") == "" && r.Header.Get(TokenHeader) == "" && !auth.StrictCSRF(r) {
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

// PublicRESTHandler accepts JSON POST for every operation. Only discovery is a
// GET, so prompts and private queries cannot leak into URLs or caches.
func PublicRESTHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == RESTRoot || r.URL.Path == RESTPrefix {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeFailure(w, r, Fail(405, "method_not_allowed", "Use GET for discovery"))
			return
		}
		app.RespondJSON(w, map[string]any{"operations": Operations})
		return
	}
	name := RESTToolName(r.URL.Path)
	if operation(name) == nil {
		writeFailure(w, r, Fail(404, "not_found", "Unknown public operation"))
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeFailure(w, r, Fail(405, "method_not_allowed", "Use POST with JSON arguments"))
		return
	}
	if r.URL.RawQuery != "" {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Send arguments in the JSON body"))
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeFailure(w, r, Fail(413, "too_large", "Request too large"))
		return
	}
	var args map[string]any
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := Decode(raw, &args); err != nil || args == nil {
		writeFailure(w, r, Fail(400, "invalid_arguments", "Send a JSON object"))
		return
	}
	result, err := Call(r, name, args)
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	app.RespondJSON(w, map[string]any{"data": result})
}
func publicResolver(r *http.Request) gwmcp.Resolver {
	res := gwmcp.NewManualResolver()
	for _, op := range Operations {
		op := op
		props := map[string]any{}
		required := []string{}
		for _, p := range op.Params {
			props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
			if p.Required {
				required = append(required, p.Name)
			}
		}
		res.Add(gwmcp.Tool{Name: op.Name, Description: op.Description, InputSchema: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}, func(ctx context.Context, args map[string]any) (*gwmcp.CallResult, error) {
			result, err := Call(r, op.Name, args)
			var payload any = map[string]any{"data": result}
			if err != nil {
				payload = map[string]any{"error": failure(err)}
			}
			raw, _ := json.Marshal(payload)
			if len(raw) > maxResultBytes {
				raw, _ = json.Marshal(map[string]any{"error": &Failure{413, "result_too_large", "Request fewer items or use HTTP to read the full result. The operation may already have completed; do not repeat a write."}})
				err = errors.New("result too large")
			}
			return &gwmcp.CallResult{Text: string(raw), IsError: err != nil}, nil
		})
	}
	return res
}
func PublicMCPHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		publicMCPPage(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeFailure(w, r, Fail(405, "method_not_allowed", "Use POST for MCP"))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeFailure(w, r, Fail(413, "too_large", "Request too large"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var call struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(body, &call)
	if call.Method == "tools/call" {
		if _, _, err := auth.RequireSession(CredentialRequest(r)); err != nil {
			writeFailure(w, r, Fail(401, "unauthenticated", "Authentication required"))
			return
		}
	}
	gwmcp.NewHandler(publicResolver(r), gwmcp.WithServerInfo("micro", "1.0.0"), gwmcp.WithProtocolVersion(MCPVersion)).ServeHTTP(w, r)
}

// publicMCPPage documents the protocol served at this endpoint.
func publicMCPPage(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<p>Connect an MCP client to Micro to ask questions, manage work and read your inbox.</p><h2>Connect</h2><p>Server URL: <code>` + html.EscapeString(app.BaseURL(r)+"/mcp") + `</code></p><p>Choose HTTP in your client. Sign in when prompted, or use an access token from <a href="/account/tokens?access=agent">Tokens</a> as <code>Authorization: Bearer &lt;token&gt;</code>.</p><h2>Tools</h2><p>The client discovers tools with <code>tools/list</code> and invokes them with <code>tools/call</code>. Access follows your account and token permissions.</p>`)
	for _, op := range Operations {
		b.WriteString(`<section class="page-section"><h3>` + html.EscapeString(op.Name) + `</h3><p>` + html.EscapeString(op.Description) + `</p><dl>`)
		for _, p := range op.Params {
			b.WriteString(`<dt><code>` + html.EscapeString(p.Name) + `</code> (` + html.EscapeString(p.Type) + `)</dt><dd>` + html.EscapeString(p.Description) + `</dd>`)
		}
		b.WriteString(`</dl></section>`)
	}
	app.Respond(w, r, app.Response{Title: "MCP", HTML: b.String()})
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
