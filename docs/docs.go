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

	// Architecture
	{Slug: "system-design", Filename: "SYSTEM_DESIGN.md", Title: "System Design", Description: "Architecture and technical decisions", Category: "Architecture"},
	{Slug: "design-system", Filename: "DESIGN_SYSTEM.md", Title: "Design System", Description: "UI/UX patterns and components", Category: "Architecture"},
	{Slug: "data-publishing", Filename: "DATA_PUBLISHING_ARCHITECTURE.md", Title: "Data Publishing", Description: "Data sync and event architecture", Category: "Architecture"},

	// Features
	{Slug: "messaging", Filename: "MESSAGING_SYSTEM.md", Title: "Messaging", Description: "Email and messaging setup", Category: "Features"},
	{Slug: "discussions", Filename: "CONTEXTUAL_DISCUSSIONS.md", Title: "Discussions", Description: "Chat context and conversations", Category: "Features"},
	{Slug: "vector-search", Filename: "VECTOR_SEARCH.md", Title: "Vector Search", Description: "Semantic search with embeddings", Category: "Features"},
	{Slug: "wallet", Filename: "WALLET_AND_CREDITS.md", Title: "Wallet & Credits", Description: "Credit system for usage", Category: "Features"},

	// Reference
	{Slug: "api", Filename: "API_COVERAGE.md", Title: "API Reference", Description: "API endpoint documentation", Category: "Reference"},
	{Slug: "environment", Filename: "ENVIRONMENT_VARIABLES.md", Title: "Configuration", Description: "Environment variables", Category: "Reference"},
	{Slug: "screenshots", Filename: "SCREENSHOTS.md", Title: "Screenshots", Description: "Application screenshots", Category: "Reference"},
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

	content.WriteString(`<style>
.docs-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
.docs-card { padding: 20px; border: 1px solid #ddd; border-radius: 8px; }
.docs-card h4 { margin: 0 0 8px 0; }
.docs-card p { margin: 0; color: #666; font-size: 14px; }
.docs-category { margin-top: 30px; }
.docs-category:first-child { margin-top: 0; }
.docs-category h3 { margin-bottom: 15px; border-bottom: 1px solid #eee; padding-bottom: 10px; }
</style>`)

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
