package docs

import (
	"embed"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/app"
)

//go:embed *.md
var docsFS embed.FS

// Document represents a documentation page
type Document struct {
	Slug        string // URL slug (e.g., "system-design")
	Filename    string // Original filename (e.g., "SYSTEM_DESIGN.md")
	Title       string // Display title
	Description string // Brief description
	Category    string // Category for grouping
}

// docs catalog - maps slugs to document info
var catalog = []Document{
	// Getting Started
	{Slug: "about", Filename: "ABOUT.md", Title: "About Mu", Description: "What Mu is and why it exists", Category: "Getting Started"},
	{Slug: "principles", Filename: "PRINCIPLES.md", Title: "Principles", Description: "Guiding principles for AI and technology", Category: "Getting Started"},
	{Slug: "installation", Filename: "INSTALLATION.md", Title: "Installation", Description: "Self-hosting and deployment guide", Category: "Getting Started"},

	// Features
	{Slug: "activitypub", Filename: "ACTIVITYPUB.md", Title: "ActivityPub", Description: "Federation with Mastodon, Threads, etc.", Category: "Features"},
	{Slug: "messaging", Filename: "MESSAGING_SYSTEM.md", Title: "Messaging", Description: "Email and messaging setup", Category: "Features"},
	{Slug: "wallet", Filename: "WALLET_AND_CREDITS.md", Title: "Wallet & Credits", Description: "Credit system for usage", Category: "Features"},

	// Reference
	{Slug: "environment", Filename: "ENVIRONMENT_VARIABLES.md", Title: "Configuration", Description: "Environment variables", Category: "Reference"},
	{Slug: "vector-search", Filename: "VECTOR_SEARCH.md", Title: "Vector Search", Description: "Semantic search setup", Category: "Reference"},
	{Slug: "mcp", Filename: "MCP.md", Title: "MCP Server", Description: "AI tool integration via MCP", Category: "Reference"},
	{Slug: "screenshots", Filename: "SCREENSHOTS.md", Title: "Screenshots", Description: "Application screenshots", Category: "Reference"},

	// Developer (accessible but not prominent)
	{Slug: "system-design", Filename: "SYSTEM_DESIGN.md", Title: "System Design", Description: "Architecture overview", Category: "Developer"},
	{Slug: "api", Filename: "API_COVERAGE.md", Title: "API Reference", Description: "REST API endpoints", Category: "Developer"},
}

// Handler serves the /docs endpoint
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/docs")
	path = strings.TrimPrefix(path, "/")

	// If no specific doc requested, show index
	if path == "" {
		renderIndex(w, r)
		return
	}

	// Find the document by slug
	var doc *Document
	for _, d := range catalog {
		if d.Slug == path {
			doc = &d
			break
		}
	}

	if doc == nil {
		http.NotFound(w, r)
		return
	}

	// Read and render the markdown file
	content, err := docsFS.ReadFile(doc.Filename)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Render markdown to HTML
	rendered := app.Render(content)

	// Wrap in navigation
	html := fmt.Sprintf(`<div class="docs">
<div class="docs-nav">
<a href="/docs">← All Docs</a>
</div>
<div class="docs-content">%s</div>
</div>`, string(rendered))

	page := app.RenderHTMLForRequest(doc.Title, doc.Description, html, r)
	w.Write([]byte(page))
}

// AboutHandler serves the /about page (shortcut to /docs/about)
func AboutHandler(w http.ResponseWriter, r *http.Request) {
	// Read the about file
	content, err := docsFS.ReadFile("ABOUT.md")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Render markdown to HTML
	rendered := app.Render(content)

	// Wrap with docs styling
	html := fmt.Sprintf(`<div class="docs">
<div class="docs-content">%s</div>
</div>`, string(rendered))

	page := app.RenderHTMLForRequest("About Mu", "What Mu is and why it exists", html, r)
	w.Write([]byte(page))
}

// renderIndex shows the documentation index
func renderIndex(w http.ResponseWriter, r *http.Request) {
	var content strings.Builder

	content.WriteString(`<p>Documentation for using and self-hosting Mu.</p>`)

	// Group by category
	categories := make(map[string][]Document)
	categoryOrder := []string{}

	for _, doc := range catalog {
		if _, exists := categories[doc.Category]; !exists {
			categoryOrder = append(categoryOrder, doc.Category)
		}
		categories[doc.Category] = append(categories[doc.Category], doc)
	}

	// Render each category
	for _, cat := range categoryOrder {
		docs := categories[cat]
		sort.Slice(docs, func(i, j int) bool {
			return docs[i].Title < docs[j].Title
		})

		content.WriteString(fmt.Sprintf(`<div class="docs-category"><h3>%s</h3><div class="docs-grid">`, cat))

		for _, doc := range docs {
			content.WriteString(fmt.Sprintf(`<a href="/docs/%s" class="docs-card">
<h4>%s</h4>
<p>%s</p>
</a>`, doc.Slug, doc.Title, doc.Description))
		}

		content.WriteString(`</div></div>`)
	}

	html := app.RenderHTMLForRequest("Documentation", "Mu documentation and guides", content.String(), r)
	w.Write([]byte(html))
}
