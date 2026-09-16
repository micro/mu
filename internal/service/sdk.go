package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// JavaScriptSDK declares named methods for every registered service. The supplied
// transport owns authentication and permissions; the SDK grants no capabilities.
func JavaScriptSDK(specs []Spec) string {
	var b strings.Builder
	b.WriteString(`export function createClient(options = {}) {
 const call = typeof options === 'function' ? options : async (service, method, args) => {
  const headers = {'Content-Type':'application/json', Accept:'application/json'};
  if (options.token) headers.Authorization = 'Bearer ' + options.token;
  const response = await fetch((options.baseURL || '').replace(/\/$/, '') + '/api/v1/' + encodeURIComponent(service) + '/' + encodeURIComponent(method), {method:'POST',headers,body:JSON.stringify(args)});
  const result = await response.json();
  if (!response.ok) {const error = new Error(result.error?.message || result.error || 'Request failed');error.status=response.status;throw error;}
  return result.data ?? result.result ?? result;
 };
 return {
`)
	for _, s := range specs {
		name, _ := json.Marshal(s.Name)
		fmt.Fprintf(&b, "  %s: {\n", name)
		for _, method := range sdkMethods(s) {
			m, _ := json.Marshal(strings.ToLower(method))
			fmt.Fprintf(&b, "   %s: (args = {}) => call(%s, %s, args),\n", m, name, m)
		}
		b.WriteString("  },\n")
	}
	b.WriteString(" };\n}\n")
	return b.String()
}

func sdkMethods(s Spec) []string {
	var methods []string
	for name := range s.Endpoints {
		methods = append(methods, name)
	}
	sort.Strings(methods)
	return methods
}

// TypeScriptSDK describes actual RPC request and response fields, including
// their JSON names. It is generated beside the JavaScript from the same Spec.
func TypeScriptSDK(specs []Spec) string {
	var b strings.Builder
	b.WriteString("export interface Client {\n")
	for _, s := range specs {
		fmt.Fprintf(&b, " %q: {\n", s.Name)
		handler := reflect.TypeOf(s.Handler)
		for _, name := range sdkMethods(s) {
			if handler == nil {
				continue
			}
			method, ok := handler.MethodByName(name)
			if !ok || method.Type.NumIn() != 4 {
				continue
			}
			doc := strings.ReplaceAll(s.Endpoints[name].Doc, "*/", "* /")
			args := "args?"
			requestType := method.Type.In(2)
			if requestType.Kind() == reflect.Pointer {
				requestType = requestType.Elem()
			}
			if requestType.Kind() == reflect.Struct {
				for i := 0; i < requestType.NumField(); i++ {
					field := requestType.Field(i)
					if field.IsExported() && field.Tag.Get("required") == "true" && field.Tag.Get("json") != "-" {
						args = "args"
					}
				}
			}
			fmt.Fprintf(&b, "  /** %s */\n  %q(%s: %s): Promise<%s>;\n", doc, strings.ToLower(name), args, sdkType(method.Type.In(2), map[reflect.Type]bool{}, true), sdkType(method.Type.In(3), map[reflect.Type]bool{}, false))
		}
		b.WriteString(" };\n")
	}
	b.WriteString("}\nexport declare function createClient(options?: {baseURL?: string; token?: string} | ((service: string, method: string, args: unknown) => Promise<unknown>)): Client;\n")
	return b.String()
}

func sdkType(t reflect.Type, seen map[reflect.Type]bool, request bool) string {
	if t.Kind() == reflect.Pointer {
		return sdkType(t.Elem(), seen, request)
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		return "string"
	}
	if t.PkgPath() == "encoding/json" && t.Name() == "RawMessage" {
		return "unknown"
	}
	if seen[t] {
		return "unknown"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return "string" // encoding/json represents byte slices as base64.
		}
		return "Array<" + sdkType(t.Elem(), seen, request) + ">"
	case reflect.Map:
		return "Record<string, " + sdkType(t.Elem(), seen, request) + ">"
	case reflect.Struct:
		seen[t] = true
		defer delete(seen, t)
		var fields []string
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := strings.Split(f.Tag.Get("json"), ",")
			if tag[0] == "-" {
				continue
			}
			name := tag[0]
			if name == "" {
				name = f.Name
			}
			optional := ""
			if strings.Contains(f.Tag.Get("json"), "omitempty") || request && f.Tag.Get("required") != "true" {
				optional = "?"
			}
			fields = append(fields, fmt.Sprintf("%q%s: %s", name, optional, sdkType(f.Type, seen, request)))
		}
		return "{ " + strings.Join(fields, "; ") + " }"
	default:
		return "unknown"
	}
}
