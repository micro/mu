// Package docs serves the two documentation pages this site has.
//
// It served nine, behind an index that grouped them into categories: About,
// Use cases, MCP, CLI, Installation, Configuration, Architecture, Security,
// Principles. Nine pages is a manual, and a product that needs a manual to be
// used has already lost the argument the product is making — the tools are
// discoverable at /tools, the protocol is a URL, and the price list is a page.
// Most of those documents existed because writing one is easier than making the
// thing explain itself.
//
// So three: /about, which is what this is; /help, which is how you point an
// agent at it and what that costs; and /install, which is how you run your own.
// Everything else that was worth keeping is in the repository, where the people
// it is for already are.
//
// The old addresses still resolve. /docs/<slug> was every one of these until
// Docs became the name of a service, and each redirects to whichever of the two
// pages replaced it.
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

// page is one of the two.
type page struct {
	Path        string
	Filename    string
	Title       string
	Description string
}

var pages = []page{
	{Path: "/about", Filename: "ABOUT.md", Title: "About",
		Description: "A personal agent — what Mu is and why"},
	{Path: "/install", Filename: "INSTALL.md", Title: "Install",
		Description: "Run your own instance"},
}

// Redirects maps every address the documentation used to answer on to the page
// that replaced it. The router registers one exact pattern each, which is what
// lets them survive /docs belonging to a service now.
var Redirects = map[string]string{
	"/docs":              "/tools",
	"/docs/about":        "/about",
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
	"/help/about":        "/about",
	"/help/installation": "/install",
	"/help/environment":  "/install",
}

// Load initializes the docs building block.
func Load() {}

// Indexed rather than positional. These were pages[0], pages[1], pages[2], so
// removing a page silently repointed the handlers after it — deleting /help
// would have made /install serve nothing and /about serve Install.
func pageAt(path string) page {
	for _, p := range pages {
		if p.Path == path {
			return p
		}
	}
	return page{}
}

// AboutHandler serves /about.
func AboutHandler(w http.ResponseWriter, r *http.Request) { serve(w, r, pageAt("/about")) }

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
