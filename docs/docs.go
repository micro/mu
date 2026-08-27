// Package docs serves the one documentation page this site has.
//
// It served nine, behind an index grouping them into categories: About, Use
// cases, MCP, CLI, Installation, Configuration, Architecture, Security,
// Principles. Nine pages is a manual, and a product that needs a manual to be
// used has already lost the argument it is making — the tools are discoverable
// at /tools, the protocol is a URL, the price list is a page. Most of those
// documents existed because writing one is easier than making the thing explain
// itself.
//
// Then three, then one. /about went last, and for the reason the other eight
// went: the landing is the page that says what this is, so an About page was a
// second answer to a question already answered, kept in a file that nothing
// fails when it goes stale. A visitor deciding whether to care reads the
// landing; nobody has ever read both.
//
// So /install, which is how you run your own, and which earns its place by
// holding what the code cannot — ports, records, an operator's decisions.
//
// The old addresses still resolve. /docs/<slug> was every one of these until
// Docs became the name of a service, and /about redirects to the landing.
package docs

import (
	"fmt"
	"net/http"
	"strings"

	"embed"

	"mu/internal/app"
)

//go:embed *.md
var docsFS embed.FS

// page is one of them. A slice rather than a constant because /install is
// unlikely to be the last, and one-of-a-kind is how the nine started.
type page struct {
	Path        string
	Filename    string
	Title       string
	Description string
}

var pages = []page{
	{Path: "/install", Filename: "INSTALL.md", Title: "Install",
		Description: "Run your own instance"},
}

// Redirects maps every address the documentation used to answer on to the page
// that replaced it. The router registers one exact pattern each, which is what
// lets them survive /docs belonging to a service now.
var Redirects = map[string]string{
	// /about is here rather than at a handler of its own because it is exactly
	// what this map is for: an address the documentation used to answer on.
	// Without it the pattern is unregistered, "/" matches everything left over,
	// and the landing quietly serves at two URLs — which is the thing deleting
	// the page was meant to stop.
	"/about":             "/",
	"/docs":              "/tools",
	"/docs/about":        "/",
	"/docs/usecases":     "/tools",
	"/docs/mcp":          "/tools",
	"/docs/cli":          "/tools",
	"/docs/architecture": "/tools",
	"/docs/security":     "/tools",
	"/docs/principles":   "/tools",
	"/docs/installation": "/install",
	"/docs/environment":  "/install",
	"/help":              "/tools",
	"/help/mcp":          "/tools",
	"/help/cli":          "/tools",
	"/help/about":        "/",
	"/help/installation": "/install",
	"/help/environment":  "/install",
}

// Load initializes the docs building block.
func Load() {}

// Indexed rather than positional. These were pages[0], pages[1], pages[2], so
// removing a page silently repointed the handlers after it — deleting /help
// would have made /install serve nothing and /about serve Install. Two of the
// three have since been deleted, which is exactly when that bug would have
// fired.
func pageAt(path string) page {
	for _, p := range pages {
		if p.Path == path {
			return p
		}
	}
	return page{}
}

// InstallHandler serves /install.
func InstallHandler(w http.ResponseWriter, r *http.Request) { serve(w, r, pageAt("/install")) }

func serve(w http.ResponseWriter, r *http.Request, p page) {
	content, err := docsFS.ReadFile(p.Filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	rendered := app.RenderTrusted(stripTitle(content))
	html := fmt.Sprintf(`<div class="docs"><div class="docs-content">%s</div></div>`, string(rendered))
	app.Respond(w, r, app.Response{Title: p.Title, Description: p.Description, HTML: html})
}

// stripTitle drops a document's leading H1.
//
// Each page is a standalone markdown file, so it opens with its own title —
// correct in the repository, where nothing else supplies one. The page shell
// already renders the title above the content, so served as a page that heading
// appears twice ("Install / Install"). This removes the second rather than
// editing the heading out of a file that reads fine on its own.
func stripTitle(md []byte) []byte {
	s := string(md)
	trimmed := strings.TrimLeft(s, "\n")
	if !strings.HasPrefix(trimmed, "# ") {
		return md
	}
	_, rest, ok := strings.Cut(trimmed, "\n")
	if !ok {
		return nil
	}
	return []byte(strings.TrimLeft(rest, "\n"))
}
