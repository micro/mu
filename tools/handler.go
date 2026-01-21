package tools

import (
	"fmt"
	"net/http"
	"strings"

	"mu/app"
)

// Handler serves /tools routes
func Handler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tools")
	path = strings.TrimPrefix(path, "/")

	// JSON response
	if app.WantsJSON(r) {
		handleJSON(w, r, path)
		return
	}

	// HTML response
	handleHTML(w, r, path)
}

func handleJSON(w http.ResponseWriter, r *http.Request, path string) {
	if path == "" {
		// List all tools
		tools := List()
		app.RespondJSON(w, map[string]any{
			"tools": tools,
			"count": len(tools),
		})
		return
	}

	// Get specific tool
	tool := Get(path)
	if tool == nil {
		app.RespondError(w, http.StatusNotFound, "tool not found")
		return
	}
	app.RespondJSON(w, tool)
}

func handleHTML(w http.ResponseWriter, r *http.Request, path string) {
	if path != "" {
		// Single tool view
		tool := Get(path)
		if tool == nil {
			app.NotFound(w, r, "Tool not found")
			return
		}
		renderToolDetail(w, tool)
		return
	}

	// List all tools
	renderToolList(w)
}

func renderToolList(w http.ResponseWriter) {
	tools := List()
	categories := Categories()

	var b strings.Builder

	b.WriteString(`<p class="text-muted mb-4">Tools available for the agent to invoke.</p>`)

	// Group by category
	for _, cat := range categories {
		catTools := ByCategory(cat)
		if len(catTools) == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf(`<h3 class="mt-4 mb-2">%s</h3>`, cat))
		b.WriteString(`<div class="card-list">`)

		for _, t := range catTools {
			b.WriteString(`<div class="card">`)
			b.WriteString(fmt.Sprintf(`<div class="card-title"><a href="/tools/%s">%s</a></div>`, t.Name, t.Name))
			b.WriteString(fmt.Sprintf(`<div class="card-desc">%s</div>`, t.Description))

			if len(t.Input) > 0 {
				b.WriteString(`<div class="card-meta">`)
				var params []string
				for name, p := range t.Input {
					req := ""
					if p.Required {
						req = "*"
					}
					params = append(params, fmt.Sprintf("%s%s", name, req))
				}
				b.WriteString(fmt.Sprintf("params: %s", strings.Join(params, ", ")))
				b.WriteString(`</div>`)
			}

			b.WriteString(`</div>`)
		}

		b.WriteString(`</div>`)
	}

	// Summary
	b.WriteString(fmt.Sprintf(`<p class="text-muted mt-4">%d tools across %d categories</p>`, len(tools), len(categories)))

	html := app.RenderHTML("Tools", "Agent tools registry", b.String())
	w.Write([]byte(html))
}

func renderToolDetail(w http.ResponseWriter, t *Tool) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`<p class="text-muted">Category: %s</p>`, t.Category))
	b.WriteString(fmt.Sprintf(`<p>%s</p>`, t.Description))

	if len(t.Input) > 0 {
		b.WriteString(`<h3>Parameters</h3>`)
		b.WriteString(`<table class="data-table"><thead><tr><th>Name</th><th>Type</th><th>Required</th><th>Description</th></tr></thead><tbody>`)

		for name, p := range t.Input {
			req := "no"
			if p.Required {
				req = "yes"
			}
			b.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				name, p.Type, req, p.Description))
		}

		b.WriteString(`</tbody></table>`)
	} else {
		b.WriteString(`<p class="text-muted">No parameters</p>`)
	}

	b.WriteString(`<p class="mt-4"><a href="/tools">← Back to tools</a></p>`)

	html := app.RenderHTML(t.Name, t.Description, b.String())
	w.Write([]byte(html))
}
