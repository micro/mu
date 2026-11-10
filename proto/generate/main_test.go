package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogueCoversServices(t *testing.T) {
	found := map[string]bool{}
	for _, s := range specs {
		found[s.Name] = true
	}
	files, err := filepath.Glob("../../service/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "var Spec = service.Spec{") && !found[filepath.Base(filepath.Dir(file))] {
			t.Errorf("missing service in generator: %s", file)
		}
	}
}
func TestFieldNumbersSurviveReorderingAndRemoval(t *testing.T) {
	s := schema{name: "test", fields: ledger{}}
	if n := s.number("Record", "a", "string"); n != 1 {
		t.Fatal(n)
	}
	if n := s.number("Record", "b", "bool"); n != 2 {
		t.Fatal(n)
	}
	// Generate in the opposite order, then remove a. Never recycle its number.
	if n := s.number("Record", "b", "bool"); n != 2 {
		t.Fatal(n)
	}
	if n := s.number("Record", "a", "string"); n != 1 {
		t.Fatal(n)
	}
	if n := s.number("Record", "c", "string"); n != 3 {
		t.Fatal(n)
	}
	if !strings.Contains(s.message("Record", "", map[string]bool{"b": true, "c": true}), "reserved 1;") {
		t.Fatal("deleted field not reserved")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("incompatible type silently accepted")
		}
	}()
	s.number("Record", "b", "string")
}
func TestNestedContainersAndEmbeddedJSON(t *testing.T) {
	type Embedded struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
	}
	type Record struct {
		Embedded
		Rows  [][]string             `json:"rows"`
		Extra map[string]interface{} `json:"extra"`
	}
	s := schema{name: "main", fields: ledger{}, types: map[reflect.Type]string{}, names: map[string]reflect.Type{}, messages: map[string]string{}, imports: map[string]bool{}}
	name := s.typ(reflect.TypeOf(Record{}), "Record")
	body := s.messages[name]
	if !strings.Contains(body, `json_name = "visible"`) || strings.Contains(body, "hidden") || !strings.Contains(body, "repeated "+name+"RowsItem") || !strings.Contains(body, "map<string, google.protobuf.Value>") {
		t.Fatal(body)
	}
}
