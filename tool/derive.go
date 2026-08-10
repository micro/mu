package tool

// Package tool builds the catalogue an agent calls.
//
// It sits at the top level because it is a staple: Tools is one of the two
// doors onto the services, and the thing behind that door has to be somewhere a
// person can find. It used to be built inside internal/api, which is the MCP
// protocol server — JSON-RPC framing, the /mcp endpoint, the payment gate — and
// that had a consequence nobody chose.
//
// internal/ may not import the product. So a tool registered there could not
// call a service; it could only name a URL, and calling it synthesised an HTTP
// request and pushed it through http.DefaultServeMux to reach the web handler
// for that route. Twenty-one tools worked that way. An agent asking for places
// went through the places *page* — its form parsing, its content negotiation,
// its own second auth check — and got back whatever that page renders for
// Accept: application/json. Nobody designed that. It was the only shape
// available to a package that is not allowed to import what it is describing.
//
// Building the catalogue up here removes the constraint rather than working
// around it. main calls Load with the registered Specs; what comes out is
// handed to internal/api, which serves it and no longer builds it.

// Every service endpoint is a tool, without anyone writing it down twice.
//
// Tools were registered by hand in main.go while the agent's own tools derive
// from the Specs. So adding an endpoint gave the agent something to call and an
// MCP client nothing, and the two lists drifted quietly: by the time anyone
// counted, six endpoints existed that no client could reach — mail_search,
// places_geocode, chat_rooms, chat_messages, wallet_check, wallet_charge. None
// of them was withheld on purpose. They were just never typed out a second
// time.
//
// This fills the gap from the Spec. A hand-written registration always wins,
// because those carry descriptions and parameter docs written for a model and
// often return one field of a response rather than the whole struct — better
// than anything reflection produces. What derivation is for is the endpoint
// nobody remembered, which now cannot go missing.
//
// The metering problem that kept this open (docs/ARCHITECTURE.md, #1445) is
// already solved: an Endpoint declares its Cost, so a derived tool is charged
// exactly like a written one instead of being a free path to a paid service.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"mu/internal/api"
	"mu/internal/service"
)

// Load builds the catalogue from the Specs registered so far and hands it to
// the protocol server.
//
// Called by main once the services are up. Anything written out by hand is
// registered before this and wins the name, so this only fills gaps — see
// DeriveTools.
func Load(specs []service.Spec) {
	DeriveTools()
	api.ToolsRegistered()
}

// DeriveTools registers a tool for every Spec endpoint that has none.
//
// Call it after the hand-written registrations and before ToolsRegistered.
func DeriveTools() {
	for _, spec := range service.Specs() {
		if spec.Handler == nil {
			continue
		}
		for method, ep := range spec.Endpoints {
			name := spec.Tool(method)
			if api.HasTool(name) {
				continue
			}
			reqType, ok := requestType(spec.Handler, method)
			if !ok {
				continue
			}
			registerDerived(spec, method, ep, name, reqType)
		}
	}
}

// requestType finds the request struct of an endpoint: go-micro handlers are
// func(ctx, *Req, *Rsp) error.
func requestType(handler any, method string) (reflect.Type, bool) {
	m, ok := reflect.TypeOf(handler).MethodByName(method)
	if !ok || m.Type.NumIn() != 4 {
		return nil, false
	}
	req := m.Type.In(2)
	if req.Kind() != reflect.Ptr || req.Elem().Kind() != reflect.Struct {
		return nil, false
	}
	return req.Elem(), true
}

// params reads a request struct's fields the way the rest of the codebase
// documents them: a json name and a description tag.
func params(t reflect.Type) []api.ToolParam {
	var out []api.ToolParam
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		// A derived tool used to have no required arguments at all: the schema
		// said every field was optional, so a model could omit the one the
		// method cannot work without and find out from an error. Opt-in by tag,
		// so a service that says nothing keeps exactly the schema it had.
		out = append(out, api.ToolParam{
			Name:        name,
			Type:        jsonType(f.Type),
			Description: f.Tag.Get("description"),
			Required:    f.Tag.Get("required") == "true",
		})
	}
	return out
}

func jsonType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Struct, reflect.Ptr:
		return "object"
	}
	return "string"
}

// PreviewDerived is the tool every Spec endpoint would produce, whether or not
// one is already registered under that name.
//
// Nothing in the running server calls it. It exists so a hand-written
// registration can be held next to the one its Spec would give, which is the
// only way to answer the question that decides whether the hand-written one is
// still earning its place: what would be lost by deleting it. Descriptions and
// parameter docs written for a model usually beat reflection — but only usually,
// and after enough endpoints have grown Doc strings and description tags of
// their own, some of the hand-written ones are carrying nothing.
func PreviewDerived() []api.Tool {
	var out []api.Tool
	for _, spec := range service.Specs() {
		if spec.Handler == nil {
			continue
		}
		for method, ep := range spec.Endpoints {
			reqType, ok := requestType(spec.Handler, method)
			if !ok {
				continue
			}
			out = append(out, derivedTool(spec, ep, spec.Tool(method), reqType))
		}
	}
	return out
}

func derivedTool(spec service.Spec, ep service.Endpoint, name string, reqType reflect.Type) api.Tool {
	return api.Tool{
		Name:        name,
		Aliases:     ep.Aliases,
		Description: ep.Doc,
		WalletOp:    ep.Cost,
		// A method where paying cannot stand in for having an account says so
		// here, or the tool is advertised to an anonymous caller and refused
		// one call later — and, for web_fetch, is a request this server makes
		// to wherever a stranger names.
		AccountOnly: ep.AccountOnly,
		Params:      params(reqType),
	}
}

func registerDerived(spec service.Spec, method string, ep service.Endpoint, name string, reqType reflect.Type) {
	endpoint := service.HandlerName(spec.Handler) + "." + method
	svc := spec.Name

	t := derivedTool(spec, ep, name, reqType)

	call := func(args map[string]any, accountID string) (string, error) {
		// Through JSON so the argument map lands in the request struct by the
		// same tags everything else reads.
		raw, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		req := reflect.New(reqType).Interface()
		if err := json.Unmarshal(raw, req); err != nil {
			return "", fmt.Errorf("bad arguments: %w", err)
		}

		rsp := map[string]any{}
		ctx := service.WithAccount(context.Background(), accountID)
		if err := service.Call(ctx, svc, endpoint, req, &rsp); err != nil {
			return "", err
		}
		return renderResponse(rsp), nil
	}

	// Scoped services need the caller bound from the session; public ones can
	// be called by anybody, and registering them as auth-only would put an
	// account behind news and weather.
	//
	// A single method on an open service can still need a caller — posting to a
	// discussion anyone may read — and that is Endpoint.Account. Without it the
	// tool was registered with a hard-coded empty account, so the handler read
	// no caller and refused its own call.
	if spec.Scoped || ep.Account {
		api.RegisterToolWithAuth(t, call)
		return
	}
	t.Handle = func(args map[string]any) (string, error) { return call(args, "") }
	api.RegisterTool(t)
}

// renderResponse prefers the one text field a response carries, because that is
// what these endpoints are written to return — model-ready prose, not a struct
// to be unpacked. Anything else goes back as JSON.
func renderResponse(rsp map[string]any) string {
	if len(rsp) == 1 {
		for _, v := range rsp {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	for _, key := range []string{"text", "result", "answer", "status"} {
		if s, ok := rsp[key].(string); ok && s != "" {
			return s
		}
	}
	b, err := json.Marshal(rsp)
	if err != nil {
		return ""
	}
	return string(b)
}
