package test

// The reference is derived, or it is wrong by the second release.
//
// /api documents every callable method: its URL, its arguments and their types,
// what it costs and whether it needs an account. All of that already exists on
// the Spec, which is the only reason publishing it is safe — a page that
// restates any of it in prose is a second copy, and the second copy is the one
// that goes stale.
//
// So this does not check wording. It checks that the page and the door agree
// about what exists, which is the property a reader depends on: every method
// they can call is documented, and every method documented can be called.

import (
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/service"
)

func apiPage(t *testing.T) string {
	t.Helper()
	registerAll(t)
	loadTools(t)

	w := httptest.NewRecorder()
	api.RESTPageHandler(w, httptest.NewRequest("GET", "/api", nil))
	if w.Code != 200 {
		t.Fatalf("/api answered %d", w.Code)
	}
	return w.Body.String()
}

func TestEveryCallableMethodIsDocumented(t *testing.T) {
	body := apiPage(t)

	var callable, documented int
	for _, sp := range service.Specs() {
		for name := range sp.Endpoints {
			tool := sp.Name + "_" + strings.ToLower(name)
			if _, ok := api.ToolByName(tool); !ok {
				// No tool derived from it, so the door answers 404 and the page
				// is right not to advertise a URL that does not work.
				if strings.Contains(body, `id="api-`+tool+`"`) {
					t.Errorf("%s is documented and has no tool behind it — the page "+
						"publishes a URL that answers 404", tool)
				}
				continue
			}
			callable++

			if !strings.Contains(body, `id="api-`+tool+`"`) {
				t.Errorf("%s can be called at %s%s/%s and is not on the page",
					tool, api.RESTPrefix, sp.Name, strings.ToLower(name))
				continue
			}
			documented++

			// And the URL it publishes is the one that resolves back to it. A
			// reference whose paths are built a second way is a reference that
			// can send somebody to the wrong method.
			path := api.RESTPrefix + sp.Name + "/" + strings.ToLower(name)
			if !strings.Contains(body, path) {
				t.Errorf("%s is on the page without its path %s", tool, path)
			}
			if got := api.RESTToolName(path); got != tool {
				t.Errorf("the page publishes %s, which resolves to %q", path, got)
			}
		}
	}

	if callable < 50 {
		t.Fatalf("only %d callable methods — this scan is broken, not the code", callable)
	}
	if documented != callable {
		t.Errorf("%d of %d callable methods documented", documented, callable)
	}
}

// A reader has to be able to tell, before they call, whether a method will
// charge them and whether it will refuse them for having no account. Both are
// on the Spec; both have to reach the page.
func TestThePageSaysWhatCostsAndWhatNeedsAnAccount(t *testing.T) {
	body := apiPage(t)

	var priced, scoped int
	for _, sp := range service.Specs() {
		for name, ep := range sp.Endpoints {
			tool := sp.Name + "_" + strings.ToLower(name)
			if _, ok := api.ToolByName(tool); !ok {
				continue
			}
			i := strings.Index(body, `id="api-`+tool+`"`)
			if i < 0 {
				continue
			}
			// The card runs until the next one starts.
			card := body[i:]
			if j := strings.Index(card[10:], `id="api-`); j >= 0 {
				card = card[:j+10]
			}

			if ep.Cost != "" {
				priced++
				if !strings.Contains(card, "per call") {
					t.Errorf("%s costs %q and its card does not say so", tool, ep.Cost)
				}
			}
			if api.ToolNeedsAuth(tool) {
				scoped++
				if !strings.Contains(card, "Needs an account") {
					t.Errorf("%s needs an account and its card does not say so", tool)
				}
			}
			if ep.Destructive && !strings.Contains(card, "POST") {
				t.Errorf("%s is destructive and its card does not say it is POST only", tool)
			}
		}
	}

	if priced == 0 || scoped == 0 {
		t.Fatalf("scan found %d priced and %d account-only methods — it is not "+
			"reaching the specs", priced, scoped)
	}
}

// The two pages point at each other. Somebody building a client who lands on
// /mcp has found the wrong one, and being handed a tool-calling protocol
// because it was the only door documented is how they conclude this is not for
// them.
func TestTheTwoDoorsPointAtEachOther(t *testing.T) {
	if page := apiPage(t); !strings.Contains(page, `href="/mcp"`) {
		t.Error("/api does not link to /mcp — an agent author landing here is left " +
			"to guess that a tool endpoint exists")
	}

	w := httptest.NewRecorder()
	api.MCPHandler(w, httptest.NewRequest("GET", "/mcp", nil))
	if !strings.Contains(w.Body.String(), `href="/api"`) {
		t.Error("/mcp does not link to /api — a client author landing here is shown " +
			"JSON-RPC and told nothing about the plain HTTP door")
	}
}
