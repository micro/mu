// Package docs serves minimal installation and optional operator guides.
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

type page struct {
	Path        string
	Filename    string
	Title       string
	Description string
}

var pages = []page{
	{Path: "/install", Filename: "INSTALL.md", Title: "Install",
		Description: "Run your own instance"},
	{Path: "/help", Filename: "README.md", Title: "Documentation"},
	{Path: "/help/hosting", Filename: "HOSTING.md", Title: "Hosting and maintenance"},
	{Path: "/help/configuration", Filename: "CONFIGURATION.md", Title: "Configuration"},
	{Path: "/help/channels", Filename: "CHANNELS.md", Title: "Mail and messaging"},
	{Path: "/help/architecture", Filename: "ARCHITECTURE.md", Title: "Architecture and development"},
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
	"/docs/architecture": "/help/architecture",
	"/docs/security":     "/tools",
	"/docs/principles":   "/tools",
	"/docs/installation": "/install",
	"/docs/environment":  "/help/configuration",

	"/help/mcp":          "/tools",
	"/help/cli":          "/tools",
	"/help/about":        "/",
	"/help/installation": "/install",
	"/help/environment":  "/help/configuration",
}

// Load initializes the docs building block.
func Load() {}

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

// Handler serves the optional guides, with exact paths and explicit legacy redirects.
func Handler(w http.ResponseWriter, r *http.Request) {
	if target, ok := Redirects[r.URL.Path]; ok {
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	serve(w, r, pageAt(r.URL.Path))
}

func serve(w http.ResponseWriter, r *http.Request, p page) {
	content, err := docsFS.ReadFile(p.Filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	rendered := app.RenderTrusted(stripTitle(content))
	body := string(rendered)
	for _, target := range pages {
		body = strings.ReplaceAll(body, `href="`+target.Filename+`"`, `href="`+target.Path+`"`)
		body = strings.ReplaceAll(body, `href="`+target.Filename+`#`, `href="`+target.Path+`#`)
	}
	html := fmt.Sprintf(`<article class="reader-content">%s</article>`, body)
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
