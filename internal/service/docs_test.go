package service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEveryEndpointIsDescribed keeps the agent's tool list honest.
//
// go-micro describes an undescribed endpoint as "Call Search on news service",
// which tells a model almost nothing about when to choose it. Descriptions
// therefore come from a Docs map passed to Register — and a map is easy to
// forget when adding a method. This reads the source rather than the running
// registry so the failure names the exact method, and so it fails on a service
// that is registered somewhere this test cannot start.
func TestEveryEndpointIsDescribed(t *testing.T) {
	root := repoRoot(t)
	dirs, err := filepath.Glob(filepath.Join(root, "service", "*"))
	dirs = append(dirs, serviceStaples(root)...)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(dirs) < 15 {
		t.Fatalf("found %d service directories, expected the full set — has the layout moved?", len(dirs))
	}

	checked := 0
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		methods, documented, ok := scanService(t, dir)
		if !ok {
			continue
		}
		checked++
		for _, m := range methods {
			if !documented[m] {
				t.Errorf("%s: endpoint %q has no entry in the service's Spec.Endpoints — "+
					"the agent would see it as \"Call %s on %s service\"",
					filepath.Base(dir), m, m, filepath.Base(dir))
			}
		}
	}
	if checked < 15 {
		t.Fatalf("only scanned %d services with handlers; the scan is not finding them", checked)
	}
}

// scanService returns the RPC endpoint method names a service package declares
// and the method names its Docs map covers. ok is false for a directory with no
// handler at all.
func scanService(t *testing.T, dir string) (methods []string, documented map[string]bool, ok bool) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	documented = map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if name, isRPC := rpcEndpoint(d); isRPC {
					methods = append(methods, name)
				}
			case *ast.GenDecl:
				collectDocKeys(d, documented)
			}
		}
	}
	return methods, documented, len(methods) > 0
}

// rpcEndpoint reports whether a declaration is a go-micro endpoint —
// an exported method of the form func(ctx, *Req, *Rsp) error.
func rpcEndpoint(d *ast.FuncDecl) (string, bool) {
	if d.Recv == nil || len(d.Recv.List) == 0 || !d.Name.IsExported() {
		return "", false
	}
	if d.Type.Params == nil || d.Type.Results == nil {
		return "", false
	}
	if fieldCount(d.Type.Params) != 3 || fieldCount(d.Type.Results) != 1 {
		return "", false
	}
	// Second and third parameters must be pointers (*Request, *Response).
	params := flatten(d.Type.Params)
	if _, ptr := params[1].(*ast.StarExpr); !ptr {
		return "", false
	}
	if _, ptr := params[2].(*ast.StarExpr); !ptr {
		return "", false
	}
	return d.Name.Name, true
}

// collectDocKeys records the method names a service.Spec's Endpoints map
// covers.
func collectDocKeys(d *ast.GenDecl, into map[string]bool) {
	if d.Tok != token.VAR {
		return
	}
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, val := range vs.Values {
			lit, ok := val.(*ast.CompositeLit)
			if !ok || !isSelector(lit.Type, "service", "Spec") {
				continue
			}
			for _, field := range lit.Elts {
				kv, ok := field.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if id, ok := kv.Key.(*ast.Ident); !ok || id.Name != "Endpoints" {
					continue
				}
				eps, ok := kv.Value.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, elt := range eps.Elts {
					ep, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if k, ok := ep.Key.(*ast.BasicLit); ok && k.Kind == token.STRING {
						if name, err := strconv.Unquote(k.Value); err == nil {
							into[name] = true
						}
					}
				}
			}
		}
	}
}

func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg && sel.Sel.Name == name
}

func fieldCount(fl *ast.FieldList) int {
	n := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			n++
			continue
		}
		n += len(f.Names)
	}
	return n
}

func flatten(fl *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	for _, f := range fl.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			out = append(out, f.Type)
		}
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod found above the test directory")
		}
		dir = parent
	}
}
