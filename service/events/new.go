package events

import "mu/internal/app"

// eventForm keeps creation separate from the agenda and briefing settings.
func eventForm(csrf string) string {
	return `<div class="page-col"><form method="POST" action="/events" class="form" onsubmit="var d=this.whenlocal.value;if(d){this.when.value=new Date(d).toISOString()}">` + app.CSRFField(csrf) +
		`<input type="hidden" name="action" value="create"><input type="hidden" name="when" value="">
 <label class="field-label">Title<input type="text" name="title" required maxlength="140" placeholder="What is happening?"></label>
 <label class="field-label">Date and time<input type="datetime-local" name="whenlocal" required></label>
 <label class="field-label">Note (optional)<textarea name="note" rows="3" maxlength="280"></textarea></label>
 <div class="form-actions"><button type="submit">Schedule</button><a href="/events" class="btn btn-quiet">Cancel</a></div>
 </form></div>`
}
