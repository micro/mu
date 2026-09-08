package home

import (
	"html"
	"mu/internal/service"
	"strings"
)

func shortcutsHTML(private bool) string {
	allowed := service.Guest()
	if private {
		allowed = service.Services()
	}
	examples := service.CommandExamples(allowed, private)
	if len(examples) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="page-section shortcuts"><h3>Shortcuts</h3><div class="form-actions">`)
	for _, example := range examples {
		b.WriteString(`<button type="button" class="btn btn-quiet" data-shortcut="` + html.EscapeString(example) + `" onclick="window.muChatAsk(this.dataset.shortcut)">` + html.EscapeString(strings.ToUpper(example[:1])+example[1:]) + `</button>`)
	}
	b.WriteString(`</div></section>`)
	return b.String()
}
