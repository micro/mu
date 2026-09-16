package home

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"mu/account"
	"mu/admin"
	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/events"
)

//go:embed command.js
var commandJS string

// ConsoleHandler is the web front door. Nothing runs until a request is sent.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	auth.SetCSRFCookie(w, r)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, acc := auth.TrySession(r)
	initial := ""
	session := r.URL.Query().Get("session")
	if session == "" {
		session = r.URL.Query().Get("continue")
	}
	if session != "" {
		if acc == nil || thread.Get(acc.ID, session) == nil {
			http.Error(w, "Conversation not found", 404)
			return
		}
		for _, message := range thread.Messages(acc.ID, session, 100) {
			class := "answer"
			content := app.RenderString(message.Text)
			if message.Role == thread.RolePerson {
				class = "request"
				content = html.EscapeString(message.Text)
			}
			initial += `<section class="turn"><div class="` + class + `">` + content + `</div></section>`
		}
	}
	fmt.Fprint(w, app.ConsoleHTML("Micro", `<div id="responses" role="log" aria-label="Requests and responses"><section class="welcome"><h1>What do you need?</h1><p>Type a command or ask a question.</p><div class="examples"><button data-command="news">news</button><button data-command="weather">weather</button><button data-command="markets">markets</button><button data-command="brief">brief</button><button data-command="inbox">inbox</button><button data-command="help">help</button></div></section>`+initial+`</div><form id="command-form"><label for="command-input">Request</label><div class="composer"><textarea id="command-input" rows="1" maxlength="8000" placeholder="Type a command or question…" autocomplete="off" spellcheck="false" required></textarea><button id="send" type="submit">Send</button></div><p id="status" role="status"></p></form><script>`+commandJS+`</script>`, acc))
}

// CommandHandler performs explicit commands before considering the assistant.
func CommandHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPost {
		app.RespondError(w, 405, "Use POST")
		return
	}
	if r.Header.Get("Authorization") != "" || r.Header.Get("X-Micro-Token") != "" || !auth.StrictCSRF(r) {
		app.RespondError(w, 403, "Reopen the page to refresh its security token")
		return
	}
	var req struct {
		Command string `json:"command"`
		Thread  string `json:"thread"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16000)).Decode(&req); err != nil || strings.TrimSpace(req.Command) == "" || len(req.Command) > 8000 {
		app.RespondError(w, 400, "Enter a command or question")
		return
	}
	input := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.Command), "/"))
	if input == "" {
		app.RespondError(w, 400, "Enter a command or question")
		return
	}
	words := strings.Fields(input)
	_, acc := auth.TrySession(r)
	owner := ""
	if acc != nil {
		owner = acc.ID
	}
	if req.Thread != "" && (owner == "" || thread.Get(owner, req.Thread) == nil) {
		app.RespondError(w, 404, "Conversation not found")
		return
	}
	var value any
	var rendered string
	var readThread string
	var err error
	switch strings.ToLower(words[0]) {
	case "help":
		value = append([]string{"Ask a question to use the assistant.", "inbox", "inbox read ID", "work", "work get ID", "brief", "brief on", "brief off", "admin", "Use SERVICE METHOD with a JSON object for explicit tool arguments."}, service.CommandExamples(service.Services(), acc != nil)...)
	case "admin":
		value, err = admin.Command(r, input)
	case "brief":
		if acc == nil {
			app.RespondError(w, 401, "Log in to read your brief")
			return
		}
		switch {
		case len(words) == 1:
			if body, id, at := latestBrief(owner); id != "" {
				rendered = deliveredBrief(owner)
				value = map[string]any{"brief": body, "delivered": at}
				break
			}
			if schedule := events.Brief(owner); schedule != nil {
				value = schedule
			} else {
				value = "No brief yet. Type brief on to enable your daily brief."
			}
		case len(words) == 2 && (words[1] == "on" || words[1] == "off"):
			zone := acc.Zone
			if zone == "" {
				zone = "UTC"
			}
			err = events.ConfigureBrief(owner, words[1] == "on", events.BriefWorldNews(events.Brief(owner)), zone)
			value = "Daily brief " + words[1] + "."
		default:
			err = fmt.Errorf("use brief, brief on, or brief off")
		}
	case "account", "settings":
		if acc == nil {
			app.RespondError(w, 401, "Log in to view your account")
			return
		}
		value = map[string]any{"id": owner, "name": acc.Name, "timezone": acc.Zone, "credits": account.Balance(owner)}
	case "inbox", "work":
		name := strings.ToLower(words[0]) + "_list"
		args := map[string]any{}
		if len(words) > 1 {
			if len(words) != 3 {
				err = fmt.Errorf("use inbox read ID or work get ID")
				break
			}
			name = strings.ToLower(words[0] + "_" + words[1])
			args["id"] = words[2]
		}
		value, err = api.Call(r, name, args)
		if err == nil && name == "inbox_read" {
			readThread, _ = args["id"].(string)
		}
	default:
		calls, matched := service.MatchCommandsFor(input, service.Services(), acc != nil)
		if !matched {
			for _, spec := range service.Specs() {
				if !strings.EqualFold(spec.Name, words[0]) {
					continue
				}
				if acc == nil {
					app.RespondError(w, 401, "Log in to use service commands")
					return
				}
				if len(words) < 2 {
					err = fmt.Errorf("name a method for %s", spec.Name)
					break
				}
				method := ""
				for m := range spec.Endpoints {
					if strings.EqualFold(m, words[1]) {
						method = m
					}
				}
				if method == "" {
					err = fmt.Errorf("unknown method for %s", spec.Name)
					break
				}
				args := map[string]any{}
				if len(words) > 2 {
					start := strings.Index(input, "{")
					if start < 0 || json.Unmarshal([]byte(input[start:]), &args) != nil {
						err = fmt.Errorf("supply arguments as a JSON object")
						break
					}
				}
				calls = []service.CommandCall{{Service: spec.Name, Method: method, Args: args}}
				matched = true
				break
			}
		}
		if err != nil {
			break
		}
		if !matched {
			app.RespondJSON(w, map[string]any{"assistant": true})
			return
		}
		ctx := service.WithAccount(r.Context(), owner)
		if acc == nil {
			ctx = service.WithRestrictedCaller(ctx)
		}
		if acc != nil && acc.Admin {
			ctx = service.WithOperator(ctx)
		}
		var results []any
		for _, call := range calls {
			for _, spec := range service.Specs() {
				if spec.Name != call.Service {
					continue
				}
				ep := spec.Endpoints[call.Method]
				if ep.Writes || ep.Destructive {
					if acc == nil || !auth.CanPost(owner) {
						app.RespondError(w, 403, "Your account cannot perform this operation")
						return
					}
					if rateErr := auth.CheckPostRate(owner); rateErr != nil {
						app.RespondError(w, 429, rateErr.Error())
						return
					}
				}
			}
			result, callErr := service.CallDynamic(ctx, call.Service, call.Method, call.Args)
			if callErr != nil {
				err = callErr
				break
			}
			results = append(results, result)
		}
		value = results
	}
	if err != nil {
		app.RespondError(w, 400, err.Error())
		return
	}
	if rendered == "" {
		rendered = formatCommand(value)
	}
	result := map[string]any{"html": rendered}
	if readThread != "" {
		result["thread"] = readThread
		app.RespondJSON(w, result)
		return
	}
	// Keep requested service results in the same conversation the assistant uses.
	// Operator commands (which can contain credentials) never enter that history.
	if owner != "" && !strings.EqualFold(words[0], "admin") && !strings.EqualFold(words[0], "help") {
		id := req.Thread
		if id == "" {
			id = thread.Open(owner, thread.WebClient, uuid.NewString()).ID
		}
		raw, _ := json.Marshal(value)
		agent.Said(owner, id, input, "", "")
		agent.Answered(owner, id, string(raw), "")
		result["thread"] = id
	}
	app.RespondJSON(w, result)
}

func formatCommand(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `<p>Unable to display this response.</p>`
	}
	var normalized any
	if json.Unmarshal(b, &normalized) != nil {
		return ""
	}
	return commandValue(normalized)
}

// All service responses share one renderer instead of acquiring a page each.
func commandValue(value any) string {
	switch v := value.(type) {
	case nil:
		return `<p>No results.</p>`
	case string:
		if u, err := url.Parse(v); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(v, "\n\r ") {
			return `<p><a rel="noopener noreferrer" href="` + html.EscapeString(v) + `">` + html.EscapeString(v) + `</a></p>`
		}
		return `<p class="text">` + html.EscapeString(v) + `</p>`
	case []any:
		if table := commandTable(v); table != "" {
			return table
		}
		if len(v) == 0 {
			return `<p>No results.</p>`
		}
		var b strings.Builder
		b.WriteString(`<div class="results">`)
		for _, row := range v {
			b.WriteString(`<section class="result">` + commandValue(row) + `</section>`)
		}
		b.WriteString(`</div>`)
		return b.String()
	case map[string]any:
		if rows, ok := v["conversations"].([]any); ok {
			var b strings.Builder
			b.WriteString(`<div class="results">`)
			for _, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				id, _ := row["id"].(string)
				title, _ := row["subject"].(string)
				if title == "" {
					title = "Conversation"
				}
				client, _ := row["client"].(string)
				updated, _ := row["updated"].(string)
				b.WriteString(`<section class="result"><button class="record-link" type="button" data-command="` + html.EscapeString("inbox read "+id) + `">` + html.EscapeString(title) + `</button><small class="record-meta">` + html.EscapeString(client+" · "+updated) + `</small></section>`)
			}
			if len(rows) == 0 {
				b.WriteString(`<p>No conversations yet.</p>`)
			}
			b.WriteString(`</div>`)
			return b.String()
		}
		if rows, ok := v["messages"].([]any); ok {
			var b strings.Builder
			b.WriteString(`<div class="results">`)
			for _, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				text, _ := row["text"].(string)
				role, _ := row["role"].(string)
				at, _ := row["at"].(string)
				b.WriteString(`<section class="result"><small class="record-meta">` + html.EscapeString(role+" · "+at) + `</small>` + app.RenderString(text) + `</section>`)
			}
			if len(rows) == 0 {
				b.WriteString(`<p>No messages yet.</p>`)
			}
			b.WriteString(`</div>`)
			return b.String()
		}
		if items, ok := v["items"].([]any); ok && len(items) > 0 {
			return commandValue(items)
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<dl>`)
		for _, key := range keys {
			b.WriteString(`<div><dt>` + html.EscapeString(strings.ReplaceAll(key, "_", " ")) + `</dt><dd>` + commandValue(v[key]) + `</dd></div>`)
		}
		b.WriteString(`</dl>`)
		return b.String()
	default:
		return html.EscapeString(fmt.Sprint(v))
	}
}

func commandTable(rows []any) string {
	if len(rows) < 2 {
		return ""
	}
	keys := []string{}
	for i, row := range rows {
		obj, ok := row.(map[string]any)
		if !ok || len(obj) > 8 || len(obj) == 0 {
			return ""
		}
		if i == 0 {
			for key := range obj {
				keys = append(keys, key)
			}
			sort.Strings(keys)
		}
		if len(obj) != len(keys) {
			return ""
		}
		for _, key := range keys {
			value, exists := obj[key]
			if !exists {
				return ""
			}
			switch v := value.(type) {
			case map[string]any, []any:
				return ""
			case string:
				if len(v) > 240 {
					return ""
				}
			}
		}
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr>`)
	for _, key := range keys {
		b.WriteString(`<th scope="col">` + html.EscapeString(key) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody>`)
	for _, row := range rows {
		b.WriteString(`<tr>`)
		for _, key := range keys {
			b.WriteString(`<td>` + commandValue(row.(map[string]any)[key]) + `</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
