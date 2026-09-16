package admin

import (
	"fmt"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/backup"
	"mu/internal/data"
	"mu/internal/flag"
	"mu/internal/settings"
	"mu/internal/usage"
	"mu/service/mail"
	"mu/service/tasks"
)

// Command executes operator commands without involving a model. The request
// must retain its original browser credentials and CSRF token.
func Command(r *http.Request, input string) (any, error) {
	if _, _, err := auth.RequireAdmin(r); err != nil {
		return nil, fmt.Errorf("admin access required")
	}
	if r.Method != http.MethodPost || !auth.StrictCSRF(r) {
		return nil, fmt.Errorf("use POST with a valid CSRF token")
	}
	words := strings.Fields(input)
	if len(words) == 1 {
		return []string{"admin logs", "admin config list", "admin config set KEY value", "admin users", "admin alerts", "admin status", "admin server", "admin backup", "admin moderate", "admin spam", "admin usage", "admin oauth", "admin work ID"}, nil
	}
	if len(words) == 3 && strings.EqualFold(words[1], "logs") {
		switch strings.ToLower(words[2]) {
		case "api":
			return app.APILog(), nil
		case "mail":
			return mail.RecentRelays(100), nil
		}
	}
	if len(words) == 3 && strings.EqualFold(words[1], "work") {
		for _, owner := range auth.AllAccounts() {
			if task, err := tasks.Get(owner.ID, words[2]); err == nil {
				return map[string]any{"id": task.ID, "status": task.Status, "steps": task.Steps, "result": task.Result}, nil
			}
		}
		return nil, fmt.Errorf("work not found")
	}
	if len(words) == 2 {
		switch strings.ToLower(words[1]) {
		case "work":
			return "Use admin work ID to inspect a job.", nil
		case "logs":
			return app.SysLog(), nil
		case "alerts":
			return app.Alerts(), nil
		case "status", "diagnostics":
			return runHealthChecks(false, false), nil
		case "server":
			return map[string]any{"status": app.InternalStatus(), "stores": data.Stores()}, nil
		case "backup":
			return backup.List(), nil
		case "moderate":
			return flag.All(), nil
		case "spam":
			return map[string]any{"filter": mail.GetSpamFilter(), "blocklist": mail.GetBlocklist()}, nil
		case "usage", "traffic":
			return usage.Series(usage.Hour, 24), nil
		case "users":
			rows := []map[string]any{}
			for _, u := range auth.AllAccounts() {
				rows = append(rows, map[string]any{"id": u.ID, "name": u.Name, "admin": u.Admin, "banned": u.Banned, "approved": u.Approved})
			}
			return rows, nil
		case "oauth":
			rows := []map[string]any{}
			for _, c := range auth.AllOAuthClients() {
				rows = append(rows, map[string]any{"id": c.ClientID, "name": c.Name, "account": c.Account})
			}
			return rows, nil
		}
	}
	if len(words) < 3 || !strings.EqualFold(words[1], "config") {
		return nil, fmt.Errorf("unknown admin command; type admin for help")
	}
	known := func(key string) bool {
		for _, group := range settingGroups {
			for _, k := range group.Vars {
				if k == key {
					return true
				}
			}
		}
		return false
	}
	switch strings.ToLower(words[2]) {
	case "list":
		if len(words) != 3 {
			return nil, fmt.Errorf("use admin config list")
		}
		type entry struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Source string `json:"source"`
		}
		var rows []entry
		for _, group := range settingGroups {
			for _, key := range group.Vars {
				value := settings.Get(key)
				if secret(key) && value != "" {
					value = "[configured]"
				}
				rows = append(rows, entry{key, value, settings.Source(key)})
			}
		}
		return rows, nil
	case "set":
		if len(words) < 5 {
			return nil, fmt.Errorf("use admin config set KEY value")
		}
		key := words[3]
		if !known(key) {
			return nil, fmt.Errorf("unknown configuration key")
		}
		if settings.Source(key) == "env" {
			return nil, fmt.Errorf("%s is set by the environment; change it on the server", key)
		}
		// Preserve spaces in the value, but never echo it or send it to a model.
		rest := strings.TrimSpace(input)
		for i := 0; i < 4; i++ {
			at := strings.IndexAny(rest, " \t\r\n")
			if at < 0 {
				return nil, fmt.Errorf("value required")
			}
			rest = strings.TrimSpace(rest[at:])
		}
		if err := settings.Store(key, rest); err != nil {
			return nil, fmt.Errorf("could not save configuration")
		}
		return key + " saved. Settings read at startup take effect after a restart.", nil
	default:
		return nil, fmt.Errorf("use admin config list or admin config set KEY value")
	}
}
