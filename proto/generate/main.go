// SPDX-License-Identifier: AGPL-3.0-only

// Command generate derives draft service contracts from the existing RPC types.
// Run from the repository root: go run ./proto/generate [-check]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"mu/internal/service"
)

type field struct {
	Number int    `json:"number"`
	Type   string `json:"type"`
}
type ledger map[string]map[string]field

type schema struct {
	name     string
	types    map[reflect.Type]string
	names    map[string]reflect.Type
	messages map[string]string
	imports  map[string]bool
	fields   ledger
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
func snake(s string) string {
	var b strings.Builder
	rr := []rune(s)
	for i, r := range rr {
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(rr[i-1]) || (i+1 < len(rr) && unicode.IsLower(rr[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
func (s *schema) typ(t reflect.Type, hint string) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		s.imports["google/protobuf/timestamp.proto"] = true
		return "google.protobuf.Timestamp"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int64:
		return "int64"
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return "int32"
	case reflect.Uint, reflect.Uint64:
		return "uint64"
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return "uint32"
	case reflect.Float32:
		return "float"
	case reflect.Float64:
		return "double"
	case reflect.Interface:
		s.imports["google/protobuf/struct.proto"] = true
		return "google.protobuf.Value"
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return "bytes"
		}
		inner := s.typ(t.Elem(), hint+"Item")
		if strings.HasPrefix(inner, "repeated ") || strings.HasPrefix(inner, "map<") {
			inner = s.wrapper(t.Elem(), hint+"Item", inner)
		}
		return "repeated " + inner
	case reflect.Map:
		key := s.typ(t.Key(), hint+"Key")
		if key != "string" && key != "bool" && key != "int32" && key != "int64" && key != "uint32" && key != "uint64" {
			panic("unsupported map key " + t.String())
		}
		value := s.typ(t.Elem(), hint+"Value")
		if strings.HasPrefix(value, "repeated ") || strings.HasPrefix(value, "map<") {
			value = s.wrapper(t.Elem(), hint+"Value", value)
		}
		return "map<" + key + ", " + value + ">"
	case reflect.Struct:
		if name, ok := s.types[t]; ok {
			return name
		}
		name := t.Name()
		if name == "" {
			name = hint
		} else if filepath.Base(t.PkgPath()) != s.name {
			name = title(filepath.Base(t.PkgPath())) + name
		}
		if name == title(s.name) {
			name += "Record"
		}
		if old, ok := s.names[name]; ok && old != t {
			panic("type name collision: " + name)
		}
		s.types[t] = name
		s.names[name] = t
		var body strings.Builder
		active := map[string]bool{}
		var visit func(reflect.Type)
		visit = func(st reflect.Type) {
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				if f.PkgPath != "" {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag == "-" {
					continue
				}
				ft := f.Type
				for ft.Kind() == reflect.Pointer {
					ft = ft.Elem()
				}
				if f.Anonymous && tag == "" && ft.Kind() == reflect.Struct {
					visit(ft)
					continue
				}
				if tag == "" {
					tag = f.Name
				}
				if active[tag] {
					panic("ambiguous embedded field " + name + "." + tag)
				}
				active[tag] = true
				wire := s.typ(f.Type, name+f.Name)
				if f.Type.Kind() == reflect.Pointer && !strings.Contains(wire, ".") && ft.Kind() != reflect.Struct && !strings.HasPrefix(wire, "map<") && !strings.HasPrefix(wire, "repeated ") {
					wire = "optional " + wire
				}
				num := s.number(name, tag, wire)
				if doc := f.Tag.Get("description"); doc != "" {
					fmt.Fprintf(&body, "  // %s\n", strings.ReplaceAll(doc, "\n", " "))
				}
				// Explicit json_name preserves the existing JSON spelling (including acronyms).
				fmt.Fprintf(&body, "  %s %s = %d [json_name = %q];\n", wire, snake(tag), num, tag)
			}
		}
		visit(t)
		s.messages[name] = s.message(name, body.String(), active)
		return name
	default:
		panic("unsupported type " + t.String())
	}
}
func (s *schema) number(message, name, typ string) int {
	key := s.name + "." + message
	if s.fields[key] == nil {
		s.fields[key] = map[string]field{}
	}
	fields := s.fields[key]
	if f, ok := fields[name]; ok {
		if f.Type != typ {
			panic("incompatible field change " + key + "." + name + ": " + f.Type + " -> " + typ)
		}
		return f.Number
	}
	n := 1
	for _, f := range fields {
		if n <= f.Number {
			n = f.Number + 1
		}
	}
	fields[name] = field{n, typ}
	return n
}
func (s *schema) message(name, body string, active map[string]bool) string {
	var retired []int
	for key, f := range s.fields[s.name+"."+name] {
		if !active[key] {
			retired = append(retired, f.Number)
		}
	}
	sort.Ints(retired)
	for _, n := range retired {
		body += fmt.Sprintf("  reserved %d;\n", n)
	}
	return "message " + name + " {\n" + body + "}\n"
}
func (s *schema) wrapper(t reflect.Type, name, wire string) string {
	// Protobuf cannot represent nested containers directly; give the inner value a name.
	if old, ok := s.names[name]; ok && old != t {
		panic("wrapper collision " + name)
	}
	s.names[name] = t
	n := s.number(name, "values", wire)
	s.messages[name] = s.message(name, fmt.Sprintf("  %s values = %d;\n", wire, n), map[string]bool{"values": true})
	return name
}
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func render(spec service.Spec, fields ledger) []byte {
	s := schema{name: spec.Name, types: map[reflect.Type]string{}, names: map[string]reflect.Type{}, messages: map[string]string{}, imports: map[string]bool{}, fields: fields}
	var rpc strings.Builder
	typ := reflect.TypeOf(spec.Handler)
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		mt := m.Type
		if mt.NumIn() != 4 || mt.NumOut() != 1 || mt.In(1) != reflect.TypeFor[context.Context]() || mt.Out(0) != reflect.TypeFor[error]() {
			continue
		}
		ep, ok := spec.Endpoints[m.Name]
		if !ok {
			panic(spec.Name + "." + m.Name + " missing Spec endpoint")
		}
		req := s.typ(mt.In(2), m.Name+"Request")
		rsp := s.typ(mt.In(3), m.Name+"Response")
		fmt.Fprintf(&rpc, "  // %s\n  rpc %s(%s) returns (%s);\n", strings.ReplaceAll(ep.Doc, "\n", " "), m.Name, req, rsp)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "// SPDX-License-Identifier: Apache-2.0\n\n// Code generated by go run ./proto/generate; DO NOT EDIT.\n// Draft service schema. Declaration does not imply MUCP availability.\nsyntax = \"proto3\";\npackage mu.service.%s.v1;\noption go_package = \"mu/proto/service/%s/v1;%sv1\";\noption java_package = \"mu.micro.client.proto.service.%s.v1\";\noption java_multiple_files = true;\n\n", spec.Name, spec.Name, spec.Name, spec.Name)
	for _, imp := range keys(s.imports) {
		fmt.Fprintf(&out, "import %q;\n", imp)
	}
	fmt.Fprintf(&out, "\nservice %s {\n%s}\n\n", title(spec.Name), rpc.String())
	for _, name := range keys(s.messages) {
		out.WriteString(s.messages[name])
		out.WriteByte('\n')
	}
	return []byte(out.String())
}
func main() {
	check := flag.Bool("check", false, "fail if checked-in contracts differ")
	flag.Parse()
	fields := ledger{}
	path := "proto/service/fields.json"
	if b, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(b, &fields); err != nil {
			panic(err)
		}
	} else if !os.IsNotExist(err) {
		panic(err)
	}
	changed := false
	write := func(path string, b []byte) {
		old, _ := os.ReadFile(path)
		if bytes.Equal(old, b) {
			return
		}
		changed = true
		if *check {
			fmt.Fprintln(os.Stderr, "contract drift:", path)
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			panic(err)
		}
	}
	for _, spec := range specs {
		if spec.Name == "contacts" || spec.Name == "chat" || spec.Name == "groups" || spec.Name == "notes" {
			continue
		} // Published pagination contract has an explicit adapter.
		write("proto/service/"+spec.Name+"/v1/"+spec.Name+".proto", render(spec, fields))
	}
	b, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		panic(err)
	}
	write(path, append(b, '\n'))
	if *check && changed {
		os.Exit(1)
	}
}
