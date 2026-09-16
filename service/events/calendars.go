package events

import (
	"html"
	"strings"

	"mu/internal/google"
)

func calendarsHTML(owner, csrf string) string {
	if !google.HasScope(owner, google.CalendarScope) {
		return ""
	}
	calendars, err := google.Calendars(owner)
	if err != nil {
		return `<p class="notice bad">Could not load your calendars. Reload to try again.</p>`
	}
	selected := map[string]bool{}
	for _, id := range google.SelectedCalendars(owner) {
		selected[id] = true
	}
	var b strings.Builder
	b.WriteString(`<form method="POST" action="/events" class="form page-section"><input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `"><input type="hidden" name="action" value="calendars"><fieldset class="form-group"><legend class="field-label">Calendars</legend><p class="text-sm text-muted">Choose which calendars appear in your agenda, brief and upcoming events, and count towards your availability.</p>`)
	for _, c := range calendars {
		checked := ""
		if selected[c.ID] || (c.Primary && selected["primary"]) {
			checked = " checked"
		}
		b.WriteString(`<label class="check-label"><input type="checkbox" name="calendars" value="` + html.EscapeString(c.ID) + `"` + checked + `><span>` + html.EscapeString(c.Summary) + `</span></label>`)
	}
	b.WriteString(`</fieldset><div class="form-actions"><button type="submit">Save calendars</button></div></form>`)
	return b.String()
}
