package api

import (
	"html"
	"strings"
)

// Visual cards on top of the tool registry.
//
// A registered tool is the unit of capability: it has a handler (what the agent
// can do) and, optionally, a Card (how the service presents itself visually).
// Attaching the card here — to the same registry the agent already uses — keeps
// a single catalog rather than a parallel one. The card body is rendered from
// the owning service's own live data; main.go wires it via SetCard so this
// package stays free of service imports.

// SetCard attaches a visual card renderer (and display title) to an already
// registered tool. No-op if the tool isn't found.
func SetCard(toolName, title string, render func() string) {
	for i := range tools {
		if tools[i].Name == toolName {
			tools[i].Card = render
			if title != "" {
				tools[i].Title = title
			}
			return
		}
	}
}

// CardForTool renders the named tool's card wrapped in the standard card
// container, or "" if it has no card or an empty body.
func CardForTool(name string) string {
	for i := range tools {
		if tools[i].Name == name {
			return tools[i].cardHTML()
		}
	}
	return ""
}

// CardTools returns the registered tools that expose a visual card, in
// registration order. Used by the dashboard and the daily brief.
func CardTools() []*Tool {
	var out []*Tool
	for i := range tools {
		if tools[i].Card != nil {
			out = append(out, &tools[i])
		}
	}
	return out
}

func (t *Tool) cardHTML() string {
	if t == nil || t.Card == nil {
		return ""
	}
	body := strings.TrimSpace(t.Card())
	if body == "" {
		return ""
	}
	title := t.Title
	if title == "" {
		title = t.Description
	}
	return `<div class="card"><h4>` + html.EscapeString(title) + `</h4>` + body + `</div>`
}
