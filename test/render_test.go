package test

// One way to render a page.
//
// There were six exported ways — RenderHTML, RenderHTMLForRequest,
// RenderHTMLWithLang, RenderHTMLWithLangAndAuth, RenderHTMLWithLangAndBody and
// Respond — which is what happens when each new need adds a parameter and a
// name rather than a field on a struct. Four had one caller or none. The two
// that were used differed only in whether the caller remembered to wrap the
// result in w.Write([]byte(...)), and 88 of them did, by hand, one at a time.
//
// Now there is app.Respond, and app.RenderHTML for the two places with no
// request to render against. Everything a page can vary is a field on
// app.Response, so the next thing a page needs is a field rather than a seventh
// function.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gone are the render functions that were folded into Respond. Naming them
// keeps the fold from quietly coming undone: a new RenderHTMLWithX is the exact
// move this replaced.
var gone = []string{
	"RenderHTMLForRequest",
	"RenderHTMLWithLang",
	"RenderHTMLWithLangAndAuth",
	"RenderHTMLWithLangAndBody",
}

func TestThereIsOneWayToRenderAPage(t *testing.T) {
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		src := string(b)
		// This file names them on purpose.
		if strings.HasSuffix(path, "render_test.go") {
			return nil
		}
		for _, name := range gone {
			if strings.Contains(src, "app."+name+"(") || strings.Contains(src, "func "+name+"(") {
				t.Errorf("%s uses %s. A page is app.Respond(w, r, app.Response{...}); "+
					"if it needs something Response cannot carry, that is a field on "+
					"Response, not a sixth render function.", path, name)
			}
		}
		// The shape that made the old door easy to reach for.
		if strings.Contains(src, "w.Write([]byte(app.Render") {
			t.Errorf("%s renders a page and writes it by hand. app.Respond does both, "+
				"and it is the only thing that also answers JSON.", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
}
