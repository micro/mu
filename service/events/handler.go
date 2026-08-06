package events

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves the /events page: schedule a reminder, see what's upcoming,
// and cancel. GET with an Accept: application/json header returns the caller's
// upcoming events as JSON.
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.TrySession(r)
	if sess == nil {
		http.Redirect(w, r, "/login?next=/events", http.StatusSeeOther)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "cancel":
			Cancel(owner, r.FormValue("id"))
		case "create":
			when, err := parseWhen(r.FormValue("when"))
			if err == nil {
				Create(owner, r.FormValue("title"), when, r.FormValue("note"))
			}
		}
		http.Redirect(w, r, "/events", http.StatusSeeOther)
		return
	}

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		app.RespondJSON(w, Upcoming(owner))
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder

	// Add form. The datetime-local value is local to the browser; a tiny script
	// converts it to an RFC3339 UTC instant on submit so 3pm means the user's
	// 3pm regardless of the server's timezone.
	b.WriteString(`<div style="max-width:640px">`)
	b.WriteString(`<form method="POST" action="/events" onsubmit="var d=this.whenlocal.value;if(d){this.when.value=new Date(d).toISOString()}" style="display:flex;flex-direction:column;gap:8px;margin:0 0 24px">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">`)
	b.WriteString(`<input type="hidden" name="action" value="create">`)
	b.WriteString(`<input type="hidden" name="when" value="">`)
	b.WriteString(`<input type="text" name="title" placeholder="Remind me to…" required maxlength="140" style="padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit">`)
	b.WriteString(`<div style="display:flex;gap:8px;flex-wrap:wrap">`)
	b.WriteString(`<input type="datetime-local" name="whenlocal" required style="flex:1;min-width:200px;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit">`)
	b.WriteString(`<button type="submit">Schedule</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`<input type="text" name="note" placeholder="Note (optional)" maxlength="280" style="padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit">`)
	b.WriteString(`</form>`)

	up := Upcoming(owner)
	now := time.Now()
	ext := externalEntries(owner, now, now.Add(30*24*time.Hour))

	if len(up) == 0 && len(ext) == 0 {
		b.WriteString(`<p style="color:#888;font-size:14px">Nothing scheduled. Add a reminder above, or just ask the agent: <em>"remind me to call the dentist tomorrow at 3pm"</em>.</p>`)
	} else {
		b.WriteString(`<h3 style="font-size:15px;margin:0 0 10px">Upcoming</h3>`)
		b.WriteString(`<div style="display:flex;flex-direction:column;gap:8px">`)
		for _, row := range mergedRows(up, ext) {
			if row.Event != nil {
				b.WriteString(eventRow(row.Event, csrf))
			} else {
				b.WriteString(externalRow(row.External))
			}
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(calendarCard(owner, r.URL.Query().Get("calendar"), csrf))
	b.WriteString(`</div>`)

	w.Write([]byte(app.RenderHTMLForRequest("Events", "Your scheduled reminders and events", b.String(), r)))
}

func eventRow(e *Event, csrf string) string {
	note := ""
	if e.Note != "" {
		note = `<div style="font-size:12px;color:#888">` + html.EscapeString(e.Note) + `</div>`
	}
	return fmt.Sprintf(`<div style="display:flex;align-items:center;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px">
<div style="flex:1">
  <div style="font-weight:600;font-size:14px">%s</div>
  <div style="font-size:13px;color:var(--link,#0066cc)">%s</div>
  %s
</div>
<a href="%s" target="_blank" rel="noopener" title="Add to Google Calendar" style="font-size:12px;color:#888;text-decoration:none;white-space:nowrap">+ Calendar</a>
<form method="POST" action="/events" style="margin:0">
  <input type="hidden" name="_csrf" value="%s">
  <input type="hidden" name="action" value="cancel">
  <input type="hidden" name="id" value="%s">
  <button type="submit" title="Cancel" style="background:none;border:0;color:#bbb;font-size:18px;cursor:pointer;line-height:1">&times;</button>
</form>
</div>`,
		html.EscapeString(e.Title),
		e.When.Local().Format("Mon 2 Jan, 15:04"),
		note,
		html.EscapeString(GoogleCalendarURL(e.Title, e.When, e.Note)),
		html.EscapeString(csrf),
		html.EscapeString(e.ID),
	)
}

// row is one line of the upcoming list, from either calendar.
type row struct {
	When     time.Time
	Event    *Event
	External External
}

// mergedRows interleaves both calendars in time order, which is the only order
// a day happens in. Two separate lists would have left the reader doing the
// merge, and the merge is the whole point of attaching one.
func mergedRows(up []*Event, ext []External) []row {
	rows := make([]row, 0, len(up)+len(ext))
	for _, e := range up {
		rows = append(rows, row{When: e.When, Event: e})
	}
	for _, x := range ext {
		rows = append(rows, row{When: x.Start, External: x})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].When.Before(rows[j].When) })
	return rows
}

// externalRow renders an entry Mu does not own: no cancel button, because Mu
// cannot cancel it, and a source label so nobody reads it as something Mu
// scheduled.
func externalRow(x External) string {
	when := x.Start.Local().Format("Mon 2 Jan, 15:04")
	if x.AllDay {
		when = x.Start.Local().Format("Mon 2 Jan") + ", all day"
	}
	where := ""
	if x.Location != "" {
		where = `<div style="font-size:12px;color:#888">` + html.EscapeString(x.Location) + `</div>`
	}
	source := x.Source
	if source == "" {
		source = ExternalName
	}
	return fmt.Sprintf(`<div style="display:flex;align-items:center;gap:12px;border:1px solid #eee;border-radius:8px;padding:10px 14px;background:#fafafa">
<div style="flex:1">
  <div style="font-weight:600;font-size:14px">%s</div>
  <div style="font-size:13px;color:var(--link,#0066cc)">%s</div>
  %s
</div>
<span style="font-size:11px;color:#aaa;white-space:nowrap">%s</span>
</div>`, html.EscapeString(x.Title), html.EscapeString(when), where, html.EscapeString(source))
}

// calendarCard is the ask, and afterwards the receipt.
//
// It sits below the list rather than above it, because someone arriving at
// /events came to see their events. An empty page that leads with a permission
// request is a product asking before it has done anything.
func calendarCard(owner, status, csrf string) string {
	if !CanConnectExternal() {
		return ""
	}

	note := ""
	switch status {
	case "connected":
		note = `<p style="font-size:13px;color:#0a7d33;margin:0 0 8px">Connected. Your calendar is now included in what's scheduled and when you're free.</p>`
	case "disconnected":
		note = `<p style="font-size:13px;color:#888;margin:0 0 8px">Disconnected. The access was revoked at Google and forgotten here.</p>`
	case "declined":
		note = `<p style="font-size:13px;color:#888;margin:0 0 8px">No calendar access granted — nothing changed.</p>`
	case "failed":
		note = `<p style="font-size:13px;color:#b00;margin:0 0 8px">That didn't complete. Try again.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="card" style="margin-top:24px">`)
	b.WriteString(note)

	if HasExternal(owner) {
		who := ""
		if ExternalAccount != nil {
			if e := ExternalAccount(owner); e != "" {
				who = " (" + html.EscapeString(e) + ")"
			}
		}
		b.WriteString(`<h4 style="margin:0 0 6px;font-size:14px">` + html.EscapeString(ExternalName) + who + `</h4>`)
		b.WriteString(`<p style="font-size:13px;color:#666;margin:0 0 10px">Read-only. Mu can see what's on it, and cannot change it.</p>`)
		// No disconnect here. Withdrawing access is one action covering
		// everything granted — Google revokes the whole grant at once — so it
		// belongs with the rest of the inventory rather than repeated on every
		// page that happens to use a piece of it.
		b.WriteString(`<p style="font-size:13px;color:#888;margin:0">Manage it in ` +
			`<a href="/account">your account</a>.</p>`)
	} else {
		b.WriteString(`<h4 style="margin:0 0 6px;font-size:14px">Connect your ` + html.EscapeString(ExternalName) + `</h4>`)
		b.WriteString(`<p style="font-size:13px;color:#666;margin:0 0 10px">Right now "when am I free" only counts what you scheduled here. Connect your calendar and it counts everything. Read-only — Mu can see what's on it, and cannot change it.</p>`)
		b.WriteString(`<a href="/oauth2/google/calendar" style="display:inline-block;background:#111;color:#fff;text-decoration:none;padding:8px 16px;border-radius:8px;font-weight:600;font-size:13px">Connect ` + html.EscapeString(ExternalName) + `</a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// CardHTMLFor renders the events card for a specific viewer: the next few
// upcoming events, or a prompt to schedule one.
func CardHTMLFor(owner string) string {
	if owner == "" {
		return ""
	}
	up := Upcoming(owner)
	if len(up) == 0 {
		return `<p style="font-size:13px;color:#888;margin:0">Nothing scheduled. Ask the agent to remind you about something.</p>`
	}
	var b strings.Builder
	now := time.Now()
	limit := 4
	for i, e := range up {
		if i >= limit {
			break
		}
		when := e.When.Local().Format("Mon 15:04")
		if e.When.Local().YearDay() == now.YearDay() && e.When.Local().Year() == now.Year() {
			when = "Today " + e.When.Local().Format("15:04")
		}
		fmt.Fprintf(&b, `<div style="display:flex;justify-content:space-between;gap:10px;padding:4px 0;font-size:13px"><span>%s</span><span style="color:#888;white-space:nowrap">%s</span></div>`,
			html.EscapeString(e.Title), when)
	}
	return b.String()
}
