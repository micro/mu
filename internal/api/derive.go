package api

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

	"mu/internal/service"
)

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
			if toolExists(name) {
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

func toolExists(name string) bool {
	for i := range tools {
		if toolMatches(tools[i], name) {
			return true
		}
	}
	return false
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
func params(t reflect.Type) []ToolParam {
	var out []ToolParam
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
		out = append(out, ToolParam{
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

func registerDerived(spec service.Spec, method string, ep service.Endpoint, name string, reqType reflect.Type) {
	endpoint := service.HandlerName(spec.Handler) + "." + method
	svc := spec.Name

	tool := Tool{
		Name:        name,
		Description: ep.Doc,
		WalletOp:    ep.Cost,
		Params:      params(reqType),
	}

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
	if spec.Scoped {
		RegisterToolWithAuth(tool, call)
		return
	}
	tool.Handle = func(args map[string]any) (string, error) { return call(args, "") }
	RegisterTool(tool)
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

// HasTool reports whether a tool of that name (or alias) is registered.
func HasTool(name string) bool { return toolExists(name) }
