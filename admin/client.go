package admin

import (
	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/backup"
	"mu/internal/data"
	"mu/internal/flag"
	"mu/internal/settings"
	"mu/internal/usage"
	"mu/service/mail"
	"net/http"
)

// ClientHandler provides explicit, admin-only view data. Configuration secrets
// are never serialized; empty password controls retain their existing values.
func ClientHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireAdmin(r); err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	switch r.URL.Query().Get("page") {
	case "log":
		switch r.URL.Query().Get("tab") {
		case "api":
			app.RespondJSON(w, app.APILog())
		case "mail":
			rows := []map[string]any{}
			for _, m := range mail.RecentMessages(50) {
				rows = append(rows, map[string]any{"time": m.CreatedAt, "from": m.From, "to": m.To, "subject": m.Subject})
			}
			app.RespondJSON(w, map[string]any{"messages": rows, "relays": mail.RecentRelays(50), "stats": mail.GetEmailStats()})
		default:
			app.RespondJSON(w, app.SysLog())
		}
	case "config":
		groups := []map[string]any{}
		for _, g := range settingGroups {
			fields := []map[string]any{}
			for _, key := range g.Vars {
				value := ""
				if !secret(key) {
					value = settings.Get(key)
				}
				fields = append(fields, map[string]any{"name": key, "value": value, "secret": secret(key), "source": settings.Source(key), "set": settings.Get(key) != ""})
			}
			groups = append(groups, map[string]any{"name": g.Name, "description": g.Does, "fields": fields})
		}
		app.RespondJSON(w, map[string]any{"groups": groups})
	case "server":
		app.RespondJSON(w, map[string]any{"status": app.InternalStatus(), "stores": data.Stores()})
	case "status":
		test := r.URL.Query().Get("test")
		checks := runHealthChecks(test == "digest", test == "federation")
		diagnosis := ""
		if r.URL.Query().Get("diagnose") == "1" {
			diagnosis = aiDiagnose(checks)
		}
		app.RespondJSON(w, map[string]any{"checks": checks, "diagnosis": diagnosis})
	case "backup":
		at, key, failure := backup.LastPush()
		app.RespondJSON(w, map[string]any{"snapshots": backup.List(), "quarantined": data.Quarantined(), "offsite": backup.PushEnabled(), "last_push": at, "key": key, "failure": failure})
	case "oauth":
		clients := []map[string]any{}
		for _, c := range auth.AllOAuthClients() {
			clients = append(clients, map[string]any{"id": c.ClientID, "name": c.Name, "redirects": c.RedirectURIs, "owner": c.Account})
		}
		app.RespondJSON(w, map[string]any{"clients": clients})
	case "spam":
		app.RespondJSON(w, map[string]any{"filter": mail.GetSpamFilter(), "blocklist": mail.GetBlocklist()})
	case "moderate":
		items := []any{}
		for _, f := range flag.All() {
			if f.ContentType == "social" && r.URL.Query().Get("source") != "social" {
				continue
			}
			var content any
			if d, ok := flag.Deleter(f.ContentType); ok {
				if p, ok := d.Get(f.ContentID).(PostContent); ok {
					content = map[string]any{"title": p.Title, "body": p.Content, "author": p.Author}
				}
			}
			items = append(items, map[string]any{"content_type": f.ContentType, "content_id": f.ContentID, "flag_count": f.FlagCount, "flagged_at": f.FlaggedAt, "content": content})
		}
		app.RespondJSON(w, map[string]any{"items": items})
	case "invite":
		app.RespondJSON(w, map[string]any{"requests": auth.ListInviteRequests()})
	case "alerts":
		app.RespondJSON(w, map[string]any{"enabled": enabled(), "recipients": admins(), "alerts": app.AlertCount(), "calls_last_hour": usage.TotalOver(usage.Minute, 60)})
	case "traffic":
		win := usage.WindowFor(r.URL.Query().Get("window"))
		app.RespondJSON(w, map[string]any{"series": usage.Series(win.Res, win.Points), "endpoints": usage.Top(win.Res, win.Points, usage.ByName, 20), "callers": usage.Top(win.Res, win.Points, usage.ByUser, 20), "surfaces": usage.Top(win.Res, win.Points, usage.BySurface, 10), "spend": app.GetUsageSummary(), "agent_health": agent.Summary()})
	default:
		app.NotFound(w, r, "No such admin page")
	}
}
