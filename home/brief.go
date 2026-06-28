package home

import (
	"strings"

	"mu/internal/api"
	"mu/internal/app"
)

// Summary returns the cached at-a-glance summary text (may be empty).
func Summary() string {
	summaryMu.RLock()
	defer summaryMu.RUnlock()
	return summaryCache
}

// HomeOverviewHTML is the always-on summary that makes `/` a home rather than a
// blank chat box: a short AI "at a glance" line plus every capability's live
// card (reminder, markets, news, social, video, blog) — the same cards as the
// inline chat, from one registry. Returns "" only when nothing is available yet.
func HomeOverviewHTML() string {
	var b strings.Builder

	if s := strings.TrimSpace(Summary()); s != "" {
		b.WriteString(`<div class="card"><h4>At a glance</h4>` + app.RenderString(s) + `</div>`)
	}
	for _, t := range api.CardTools() {
		b.WriteString(api.CardForTool(t.Name))
	}
	return b.String()
}
