package docs

import (
	"embed"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"mu/internal/app"
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
	{Slug: "vision", Filename: "VISION.md", Title: "Vision", Description: "Why Mu exists and the design choices behind it", Category: "Getting Started"},
	{Slug: "principles", Filename: "PRINCIPLES.md", Title: "Principles", Description: "Guiding principles for AI and technology", Category: "Getting Started"},
	{Slug: "installation", Filename: "INSTALLATION.md", Title: "Installation", Description: "Self-hosting and deployment guide", Category: "Getting Started"},

	// Features
	{Slug: "apps", Filename: "APPS.md", Title: "Apps", Description: "Build and launch small web apps", Category: "Features"},
	{Slug: "activitypub", Filename: "ACTIVITYPUB.md", Title: "ActivityPub", Description: "Federation with Mastodon, Threads, etc.", Category: "Features"},
	{Slug: "messaging", Filename: "MESSAGING_SYSTEM.md", Title: "Messaging", Description: "Email and messaging setup", Category: "Features"},
	{Slug: "wallet", Filename: "WALLET_AND_CREDITS.md", Title: "Wallet & Credits", Description: "Credit system for usage", Category: "Features"},
	{Slug: "discord", Filename: "DISCORD.md", Title: "Discord", Description: "Discord bot setup and usage", Category: "Features"},
	{Slug: "telegram", Filename: "TELEGRAM.md", Title: "Telegram", Description: "Telegram bot setup and usage", Category: "Features"},

	// Reference
	{Slug: "environment", Filename: "ENVIRONMENT_VARIABLES.md", Title: "Configuration", Description: "Environment variables", Category: "Reference"},
	{Slug: "mcp", Filename: "MCP.md", Title: "MCP Server", Description: "AI tool integration via MCP", Category: "Reference"},
	{Slug: "screenshots", Filename: "SCREENSHOTS.md", Title: "Screenshots", Description: "Application screenshots", Category: "Reference"},

	// Developer (accessible but not prominent)
	{Slug: "system-design", Filename: "SYSTEM_DESIGN.md", Title: "System Design", Description: "Architecture overview", Category: "Developer"},
	{Slug: "whitepaper", Filename: "WHITEPAPER.md", Title: "Whitepaper", Description: "Full network overview and future direction", Category: "Developer"},
}

// Load initializes the docs building block.
func Load() {}

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

// renderIndex shows the documentation index
func renderIndex(w http.ResponseWriter, r *http.Request) {
	var content strings.Builder

	content.WriteString(`<p style="color:#555;font-size:15px;line-height:1.6;margin-bottom:24px">Learn how to use Mu — an agent for everyday: news, mail, markets, weather, search and video, handled by one AI you just talk to.</p>`)

	// Start here
	essentials := []string{"about", "installation", "discord", "telegram", "mcp"}
	content.WriteString(`<h3 style="margin-bottom:12px">Start here</h3>`)
	content.WriteString(`<div style="margin-bottom:32px">`)
	for _, slug := range essentials {
		for _, doc := range catalog {
			if doc.Slug == slug {
				content.WriteString(fmt.Sprintf(`<div style="margin-bottom:12px"><a href="/docs/%s" style="font-weight:600;font-size:15px">%s</a><br><span style="color:#666;font-size:13px">%s</span></div>`, doc.Slug, doc.Title, doc.Description))
			}
		}
	}
	content.WriteString(`</div>`)

	// Group remaining by category
	categories := make(map[string][]Document)
	categoryOrder := []string{}
	essentialSet := map[string]bool{}
	for _, s := range essentials {
		essentialSet[s] = true
	}

	for _, doc := range catalog {
		if essentialSet[doc.Slug] {
			continue
		}
		if _, exists := categories[doc.Category]; !exists {
			categoryOrder = append(categoryOrder, doc.Category)
		}
		categories[doc.Category] = append(categories[doc.Category], doc)
	}

	for _, cat := range categoryOrder {
		docs := categories[cat]
		sort.Slice(docs, func(i, j int) bool {
			return docs[i].Title < docs[j].Title
		})

		content.WriteString(fmt.Sprintf(`<h3 style="margin:24px 0 8px">%s</h3>`, cat))
		for _, doc := range docs {
			content.WriteString(fmt.Sprintf(`<div style="margin-bottom:8px"><a href="/docs/%s" style="font-size:14px">%s</a> <span style="color:#888;font-size:13px">— %s</span></div>`, doc.Slug, doc.Title, doc.Description))
		}
	}

	html := app.RenderHTMLForRequest("Documentation", "Mu documentation and guides", content.String(), r)
	w.Write([]byte(html))
}
