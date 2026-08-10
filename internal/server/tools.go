package server

// The tools an agent can call that are written out by hand.
//
// Most are derived from a service's Spec (internal/api/derive.go) and need no
// entry here at all. What is left is the ones that predate deriving, or that
// map onto something other than a single service method.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/blog"
	"mu/service/contacts"
	"mu/service/events"
	"mu/service/files"
	"mu/service/images"
	"mu/service/index"
	"mu/service/mail"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/social"
	"mu/service/tasks"
	"mu/service/video"
	"mu/service/weather"
	"mu/service/web"
	"mu/wallet"
)

// registerTools declares the hand-written half of the tool registry.
func registerTools() {
	// web_search — cached Brave web search, returned as model-ready text (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "web_search",
		Aliases:     []string{"search_web"},
		Description: "Search the web for current information and news",
		WalletOp:    quota.OpWebSearch,
		Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			q := api.QueryArg(args)
			var rsp web.SearchResponse
			if err := service.Call(context.Background(), "web", "Server.Search",
				&web.SearchRequest{Query: q}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// contacts_* — the address book. mail could send to an address but nothing
	// could turn a name into one, so "email Sarah about Thursday" was not a
	// request Mu could act on.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "contacts_find",
		Description: "Look someone up in your address book by name, part of a name, or address. Use this before sending mail to a person named rather than addressed.",
		Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "A name, part of a name, or an address", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		return contacts.Render(contacts.Find(accountID, api.QueryArg(args))), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "contacts_add",
		Description: "Save someone to your address book. Adding a name already there updates it rather than making a second card.",
		Params: []api.ToolParam{
			{Name: "name", Type: "string", Description: "The person's name", Required: true},
			{Name: "email", Type: "string", Description: "Their email address", Required: false},
			{Name: "phone", Type: "string", Description: "Their phone number", Required: false},
			{Name: "note", Type: "string", Description: "Anything worth remembering about them", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		str := func(k string) string { v, _ := args[k].(string); return v }
		c, err := contacts.Add(accountID, str("name"), str("email"), str("phone"), str("note"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(c)
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "contacts_list",
		Description: "List everyone in your address book.",
	}, func(args map[string]any, accountID string) (string, error) {
		return contacts.Render(contacts.List(accountID)), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "contacts_delete",
		Description: "Remove someone from your address book.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The contact's id", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		if err := contacts.Remove(accountID, id); err != nil {
			return "", err
		}
		return `{"status":"ok"}`, nil
	})

	// tasks_* — what is to be done. Everything else here answers a question the
	// moment it is asked; a task outlives the conversation, which is what turns
	// an agent that answers into an agent you can give a job to.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "tasks_create",
		Description: "Add a task to your list. Set assignee to \"agent\" for work the agent should pick up itself.",
		Params: []api.ToolParam{
			{Name: "title", Type: "string", Description: "What is to be done", Required: true},
			{Name: "detail", Type: "string", Description: "Context the doer needs: links, constraints, what good looks like"},
			{Name: "assignee", Type: "string", Description: "me (default) or agent"},
			{Name: "due", Type: "string", Description: "Optional deadline, e.g. 2026-08-09 09:00"},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		title, _ := args["title"].(string)
		detail, _ := args["detail"].(string)
		assignee, _ := args["assignee"].(string)
		dueStr, _ := args["due"].(string)
		due, err := tasks.ParseDue(dueStr)
		if err != nil {
			return "", err
		}
		t, err := tasks.Create(accountID, title, detail, assignee, due)
		if err != nil {
			return "", err
		}
		return tasks.Render([]*tasks.Task{t}), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "tasks_list",
		Description: "List your tasks, open ones first. Filter by state to see only what is outstanding.",
		Params: []api.ToolParam{
			{Name: "status", Type: "string", Description: "Optional: todo, doing or done"},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		status, _ := args["status"].(string)
		return tasks.Render(tasks.List(accountID, status)), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "tasks_next",
		Description: "The next task assigned to the agent — what to work on now. Returns nothing when there is nothing assigned.",
	}, func(args map[string]any, accountID string) (string, error) {
		t := tasks.Next(accountID)
		if t == nil {
			return "Nothing assigned to the agent.", nil
		}
		return tasks.Render([]*tasks.Task{t}), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "tasks_update",
		Description: "Change a task: mark it doing or done, and record what came of it. Record the result — a task with no result is work nobody can check.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The task's id", Required: true},
			{Name: "status", Type: "string", Description: "todo, doing or done"},
			{Name: "result", Type: "string", Description: "What came of it: the answer, the outcome, what was found"},
			{Name: "title", Type: "string", Description: "New title"},
			{Name: "detail", Type: "string", Description: "New detail"},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		status, _ := args["status"].(string)
		result, _ := args["result"].(string)
		title, _ := args["title"].(string)
		detail, _ := args["detail"].(string)
		t, err := tasks.Update(accountID, id, title, detail, status, "", result)
		if err != nil {
			return "", err
		}
		return tasks.Render([]*tasks.Task{t}), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "tasks_delete",
		Description: "Remove a task from your list.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The task's id", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		if err := tasks.Remove(accountID, id); err != nil {
			return "", err
		}
		return `{"status":"ok"}`, nil
	})

	// mail_address — an address an agent can be reached at.
	//
	// An agent that can be *sent* mail is a different thing from one that can
	// only be called: a receipt, a form, a reply or a newsletter can reach it,
	// and wake it. Mu runs a real SMTP server with DKIM, so it can offer this
	// where almost nothing else can. It does not make an account per agent —
	// accounts are people, holding credits and files — it gives the agent a
	// facet of its owner's address, which is what plus-addressing already means
	// to anyone who has used Gmail.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "mail_address",
		Description: "Get an email address that reaches you. Pass a tag to get your own address (you+tag@) — give that out, then read only its mail with mail_inbox(tag).",
		Params: []api.ToolParam{
			{Name: "tag", Type: "string", Description: "A label for this address, e.g. \"research\" or \"receipts\". Omit for your plain address.", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		tag, _ := args["tag"].(string)
		// The account id, not the display name. Delivery resolves the local
		// part with auth.GetAccount, which is an exact lookup keyed by id — an
		// address built from a display name ("Asim+research@") would look right
		// and silently fail to arrive.
		if _, err := auth.GetAccount(accountID); err != nil {
			return "", err
		}
		addr := mail.AliasFor(accountID, tag)
		out := map[string]any{"address": addr}
		if tag != "" {
			out["read_with"] = fmt.Sprintf(`mail_inbox {"tag": %q}`, tag)
		}
		b, _ := json.Marshal(out)
		return string(b), nil
	})

	// events_free — the question a calendar is actually asked. events could
	// schedule a thing and list what was coming, which makes it a reminder
	// queue; "when am I free on Thursday" is the other half.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "events_free",
		Description: "Find when you have nothing booked. Give it how long you need and it returns the open slots, within working hours.",
		Params: []api.ToolParam{
			{Name: "from", Type: "string", Description: "Start of the window, RFC3339 (default now)", Required: false},
			{Name: "to", Type: "string", Description: "End of the window, RFC3339 (default a week later)", Required: false},
			{Name: "minutes", Type: "number", Description: "How long a slot you need, in minutes (default 30)", Required: false},
			{Name: "day_start", Type: "number", Description: "Earliest hour to offer, 0-23 (default 9)", Required: false},
			{Name: "day_end", Type: "number", Description: "Latest hour to offer, 0-23 (default 18)", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		num := func(k string) int { v, _ := args[k].(float64); return int(v) }
		str := func(k string) string { v, _ := args[k].(string); return v }
		var rsp events.FreeResponse
		err := service.Call(service.WithAccount(context.Background(), accountID),
			"events", "Server.Free", &events.FreeRequest{
				From: str("from"), To: str("to"), Minutes: num("minutes"),
				DayStart: num("day_start"), DayEnd: num("day_end"),
			}, &rsp)
		if err != nil {
			return "", err
		}
		return rsp.Text, nil
	})

	// files_* — keep a file, get a URL, read it back. db holds JSON records and
	// images holds pictures; neither holds a file, so an agent that produced a
	// report had nowhere to leave it and no link to hand over.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "files_put",
		Description: "Store a file and get a URL for it. Use it to keep something you produced — a report, a CSV, a transcript — and hand back a link.",
		Params: []api.ToolParam{
			{Name: "name", Type: "string", Description: "File name including its extension, e.g. report.csv", Required: true},
			{Name: "content", Type: "string", Description: "The file's contents — plain text, or base64 when encoding is base64", Required: true},
			{Name: "encoding", Type: "string", Description: "\"base64\" for binary files; omit for text", Required: false},
			{Name: "type", Type: "string", Description: "Optional content type, e.g. text/csv. Guessed from the name when omitted", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		str := func(k string) string { v, _ := args[k].(string); return v }
		f, err := files.Put(accountID, str("name"), str("type"), str("content"), str("encoding"))
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(f)
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "files_list",
		Description: "List your stored files, newest first, with their URLs and ids.",
	}, func(args map[string]any, accountID string) (string, error) {
		b, _ := json.Marshal(files.List(accountID))
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "files_get",
		Description: "Read a stored file back by its id. Text comes back as text, anything else as base64.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The file's id", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		var rsp files.GetResponse
		if err := service.Call(service.WithAccount(context.Background(), accountID),
			"files", "Server.Get", &files.GetRequest{ID: id}, &rsp); err != nil {
			return "", err
		}
		b, _ := json.Marshal(rsp)
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "files_share",
		Description: "Make a stored file readable by anyone holding its URL, or private again.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The file's id", Required: true},
			{Name: "public", Type: "boolean", Description: "True to share, false to make private again", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		public, _ := args["public"].(bool)
		f, err := files.Share(accountID, id, public)
		if err != nil {
			return "", err
		}
		b, _ := json.Marshal(f)
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "files_delete",
		Description: "Delete a file you own, and its contents.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The file's id", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		if err := files.Delete(accountID, id); err != nil {
			return "", err
		}
		return `{"status":"ok"}`, nil
	})

	// places_eta — how long a journey takes, by road. Registered here rather
	// than as a path-backed tool in mcp.go because it has no page of its own:
	// it answers a question, it does not render anything.
	api.RegisterTool(api.Tool{
		Name:        "places_eta",
		Description: "How long it takes to travel between two places, by road rather than as the crow flies. Use it to answer whether somewhere is worth going to, or when to leave.",
		WalletOp:    quota.OpPlacesETA,
		Params: []api.ToolParam{
			{Name: "from", Type: "string", Description: "Where the journey starts, e.g. \"King's Cross, London\"", Required: false},
			{Name: "to", Type: "string", Description: "Where the journey ends, e.g. \"Heathrow Airport\"", Required: false},
			{Name: "from_lat", Type: "number", Description: "Start latitude, if already known", Required: false},
			{Name: "from_lon", Type: "number", Description: "Start longitude, if already known", Required: false},
			{Name: "to_lat", Type: "number", Description: "End latitude, if already known", Required: false},
			{Name: "to_lon", Type: "number", Description: "End longitude, if already known", Required: false},
			{Name: "mode", Type: "string", Description: "How to travel: drive (default), walk, cycle or transit", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			num := func(k string) float64 { v, _ := args[k].(float64); return v }
			str := func(k string) string { v, _ := args[k].(string); return v }
			var rsp places.ETAResponse
			err := service.Call(context.Background(), "places", "Server.ETA", &places.ETARequest{
				From: str("from"), To: str("to"), Mode: str("mode"),
				FromLat: num("from_lat"), FromLon: num("from_lon"),
				ToLat: num("to_lat"), ToLon: num("to_lon"),
			}, &rsp)
			if err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// web_fetch tool — fetch a URL and return cleaned readable content
	api.RegisterTool(api.Tool{
		Name:        "web_fetch",
		Aliases:     []string{"search_fetch"},
		Description: "Fetch a web page and return its cleaned readable content (strips ads, popups, navigation). Needs an account.",
		Method:      "GET",
		Path:        "/web/fetch",
		WalletOp:    quota.OpWebFetch,
		// Costs us nothing — it is an http.Get and a readability pass in this
		// process — but "fetch any URL you name" pointed at an anonymous
		// caller is a request this server makes on their behalf, to wherever
		// they say. That wants somebody accountable behind it, which is a
		// different question from what it costs. Its handler already required
		// an account; this says so at the MCP layer instead of advertising the
		// tool and refusing it one call later.
		AccountOnly: true,
		Params: []api.ToolParam{
			{Name: "url", Type: "string", Description: "The URL to fetch", Required: true},
		},
	})

	// news_headlines tool — topic-balanced headlines for scanning before reading
	api.RegisterTool(api.Tool{
		Name:        "news_list",
		Aliases:     []string{"news", "news_headlines"},
		Description: "Get recent news headlines with short summaries balanced across all topics (not dominated by one topic like crypto). Use for general news and briefing requests, then news_read for any article worth expanding.",
		Params: []api.ToolParam{
			{Name: "topic", Type: "string", Description: "Optional topic/category filter (e.g. tech, world, business)", Required: false},
			{Name: "limit", Type: "string", Description: "Optional max number of headlines (default 30)", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			topic, _ := args["topic"].(string)
			limit := 0
			switch v := args["limit"].(type) {
			case float64:
				limit = int(v)
			case string:
				fmt.Sscanf(v, "%d", &limit)
			}
			var rsp news.ListResponse
			if err := service.Call(context.Background(), "news", "Server.List",
				&news.ListRequest{Topic: topic, Limit: limit}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// news_read tool — fetch one full article by id (from news_headlines) or URL
	api.RegisterTool(api.Tool{
		Name:        "news_read",
		Description: "Read one news article in full (title, source, summary and body) by its id from news_headlines, or by article URL.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "Article id (from news_headlines) or article URL", Required: true},
			// Declared because it is accepted. The handler has always taken
			// url as an alias for id — an agent holding a link reaches for the
			// obvious name — but it was not in the schema, so the leniency
			// only helped a caller who guessed it was there.
			{Name: "url", Type: "string", Description: "Article URL, if you have the link rather than the id", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			id, _ := args["id"].(string)
			if id == "" {
				id, _ = args["url"].(string)
			}
			var rsp news.ReadResponse
			if err := service.Call(context.Background(), "news", "Server.Read",
				&news.ReadRequest{ID: id}, &rsp); err != nil {
				return err.Error(), err
			}
			return rsp.Text, nil
		},
	})

	// recall tool — unified search across everything mu knows for the caller:
	// the public indexed corpus (news, blog, social, video) plus their own mail.
	if err := service.Register(index.Spec); err != nil {
		app.Log("main", "recall service register failed: %v", err)
	}
	api.RegisterToolWithAuth(api.Tool{
		Name:         "index_search",
		Aliases:      []string{"index", "recall", "search"},
		OptionalAuth: true,
		Description:  "Search across everything mu knows — indexed news, blog, social and video, plus the user's own mail — and return the most relevant items with ids. Use for 'do you remember', 'what did I get about X', 'search my stuff' and cross-source lookups.",
		Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "What to look for", Required: true},
			{Name: "limit", Type: "string", Description: "Optional max results (default 12)", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		query, _ := args["query"].(string)
		if strings.TrimSpace(query) == "" {
			return "query is required", fmt.Errorf("missing query")
		}
		limit := 12
		switch v := args["limit"].(type) {
		case float64:
			if int(v) > 0 {
				limit = int(v)
			}
		case string:
			var n int
			if _, e := fmt.Sscanf(v, "%d", &n); e == nil && n > 0 {
				limit = n
			}
		}
		var rsp index.Response
		if err := service.Call(service.WithAccount(context.Background(), accountID), "index", "Server.Search",
			&index.Request{Query: strings.TrimSpace(query), Limit: limit}, &rsp); err != nil {
			return "", err
		}
		return rsp.Text, nil
	})

	// db_* is derived from service/db's Spec now, not hand-registered here.
	//
	// It was written out because db was held to be storage rather than a
	// service — but a tool with no service behind it sits outside the scoping
	// model, so no scoped agent token could reach it and there was no box on
	// /agents to grant it. See the package comment in service/db.

	// markets — live prices, returned as model-ready text (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "markets_list",
		Aliases:     []string{"markets"},
		Description: "Get live market prices for cryptocurrencies, stocks, futures, commodities and currencies.",
		Params: []api.ToolParam{
			{Name: "category", Type: "string", Description: "crypto, stocks, futures, commodities or currencies (default crypto)", Required: false},
		},
		Handle: func(args map[string]any) (string, error) {
			category, _ := args["category"].(string)
			var rsp markets.ListResponse
			if err := service.Call(context.Background(), "markets", "Server.List",
				&markets.ListRequest{Category: category}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// image_generate — text-to-image via Atlas Cloud (metered, per-user).
	// Charging happens inside images.Generate so every path bills exactly once;
	// WalletOp here gates affordability and advertises the per-call price.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "images_generate",
		Aliases:     []string{"image_generate"},
		Description: "Generate an image from a text prompt. Returns a URL to the generated image.",
		WalletOp:    quota.OpImageGenerate,
		Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Describe the image to generate", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		prompt, _ := args["prompt"].(string)
		url, err := images.Generate(accountID, prompt)
		if err != nil {
			return "", err
		}
		// Absolute, because a caller outside this site cannot open a path.
		if id, ok := strings.CutPrefix(url, "/images/file/"); ok {
			url = images.AbsoluteURL(id)
		}
		return fmt.Sprintf("Generated image: %s\n\n![image](%s)", url, url), nil
	})

	// events_create / events_list — personal scheduling. Account-scoped: the
	// caller's id is bound by the platform, never supplied by the model.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "events_create",
		Description: "Schedule a reminder or event. Pass repeat for something recurring, and prompt to have the agent do the work when it fires and mail you the answer — that is how a standing instruction like a morning briefing is set up.",
		Params: []api.ToolParam{
			{Name: "title", Type: "string", Description: "What to be reminded about", Required: true},
			{Name: "when", Type: "string", Description: "When to fire, RFC3339 with timezone offset, e.g. 2026-07-22T15:00:00+01:00", Required: true},
			{Name: "note", Type: "string", Description: "Optional extra detail"},
			{Name: "minutes", Type: "number", Description: "How long it lasts in minutes (default 30). What events_free subtracts.", Required: false},
			{Name: "repeat", Type: "string", Description: "How often it recurs: hourly, daily, weekly or monthly. Omit for once.", Required: false},
			{Name: "prompt", Type: "string", Description: "Optional instruction to run through the agent when it fires, e.g. \"brief me on today's news\". The answer is mailed to you. This is how you set up something recurring like a morning briefing.", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		title, _ := args["title"].(string)
		when, _ := args["when"].(string)
		note, _ := args["note"].(string)
		minutes, _ := args["minutes"].(float64)
		repeat, _ := args["repeat"].(string)
		prompt, _ := args["prompt"].(string)
		var rsp events.CreateResponse
		if err := service.Call(service.WithAccount(context.Background(), accountID), "events", "Server.Create",
			&events.CreateRequest{Title: title, When: when, Note: note, Minutes: int(minutes),
				Repeat: repeat, Prompt: prompt},
			&rsp); err != nil {
			return "", err
		}
		return rsp.Result, nil
	})

	api.RegisterToolWithAuth(api.Tool{
		Name:        "events_list",
		Description: "List your upcoming scheduled events and reminders, soonest first.",
	}, func(args map[string]any, accountID string) (string, error) {
		var rsp events.ListResponse
		if err := service.Call(service.WithAccount(context.Background(), accountID), "events", "Server.List",
			&events.ListRequest{}, &rsp); err != nil {
			return "", err
		}
		return rsp.Events, nil
	})

	// events_delete — the other half of scheduling. Without it an agent could
	// set a standing instruction to run every morning and nobody, including
	// whoever pays for each run, had a way to stop it. The id comes from
	// events_list, which is why that now prints one on every line.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "events_delete",
		Description: "Cancel a scheduled event or reminder by id. Use events_list to find the id.",
		Params: []api.ToolParam{
			{Name: "id", Type: "string", Description: "The event's id, as given by events_list", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		id, _ := args["id"].(string)
		var rsp events.DeleteResponse
		if err := service.Call(service.WithAccount(context.Background(), accountID), "events", "Server.Delete",
			&events.DeleteRequest{ID: id}, &rsp); err != nil {
			return "", err
		}
		return rsp.Status, nil
	})

	// prayer_times — today's prayer times for a location. Public: no account
	// data involved, same as weather.
	api.RegisterTool(api.Tool{
		Name:        "prayer_times",
		Aliases:     []string{"islam_prayer"},
		Description: "Get today's Islamic prayer times (Fajr, Dhuhr, Asr, Maghrib, Isha) for a location, and which prayer is next.",
		Params: []api.ToolParam{
			{Name: "lat", Type: "number", Description: "Latitude of the location", Required: true},
			{Name: "lon", Type: "number", Description: "Longitude of the location", Required: true},
			{Name: "tz", Type: "string", Description: "IANA timezone of the location, e.g. Europe/London"},
		},
		Handle: func(args map[string]any) (string, error) {
			lat := argFloat(args["lat"])
			lon := argFloat(args["lon"])
			if lat == 0 && lon == 0 {
				return "Provide lat and lon for the location.", fmt.Errorf("missing coordinates")
			}
			tz, _ := args["tz"].(string)
			var rsp prayer.TimesResponse
			if err := service.Call(context.Background(), "prayer", "Server.Times",
				&prayer.TimesRequest{Lat: lat, Lon: lon, TZ: tz}, &rsp); err != nil {
				return "", err
			}
			return rsp.Times, nil
		},
	})

	// prayer_qibla — direction to face for prayer. Pure computation, public.
	api.RegisterTool(api.Tool{
		Name:        "prayer_qibla",
		Aliases:     []string{"islam_qibla"},
		Description: "Get the qibla direction (compass bearing to the Kaaba in Mecca) for a location, and the distance to Mecca.",
		Params: []api.ToolParam{
			{Name: "lat", Type: "number", Description: "Latitude of the location", Required: true},
			{Name: "lon", Type: "number", Description: "Longitude of the location", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			lat := argFloat(args["lat"])
			lon := argFloat(args["lon"])
			if lat == 0 && lon == 0 {
				return "Provide lat and lon for the location.", fmt.Errorf("missing coordinates")
			}
			var rsp prayer.QiblaResponse
			if err := service.Call(context.Background(), "prayer", "Server.Qibla",
				&prayer.QiblaRequest{Lat: lat, Lon: lon}, &rsp); err != nil {
				return "", err
			}
			return rsp.Direction, nil
		},
	})

	// saved_list — the items you've bookmarked for later, with links. Read-only,
	// per-user, free; the save/unsave tools already exist for writing.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "saved_list",
		Description: "List the items you've saved for later (bookmarks), with their links.",
	}, func(args map[string]any, accountID string) (string, error) {
		items := app.GetSavedList(accountID)
		if len(items) == 0 {
			return "You have no saved items.", nil
		}
		var sb strings.Builder
		for _, it := range items {
			sb.WriteString(fmt.Sprintf("- %s (%s)\n  %s\n", it.Title, it.Type, it.URL))
		}
		return sb.String(), nil
	})

	// image_search — search the public image stock pool by description. No auth:
	// public stock is reusable by anyone or any agent. Free.
	api.RegisterTool(api.Tool{
		Name:        "images_search",
		Aliases:     []string{"image_search"},
		Description: "Search the public image library (community stock) by description. Returns image URLs to reuse.",
		Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "Search text", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			q := api.QueryArg(args)
			recs := images.Search("", q)
			if len(recs) == 0 {
				return "No matching images in the stock pool.", nil
			}
			var sb strings.Builder
			for i, rec := range recs {
				if i >= 20 {
					break
				}
				prompt, _ := rec.Data["prompt"].(string)
				// Our copy, not the provider's link: the caller is being handed
				// a URL to keep, and the provider's expires.
				sb.WriteString(fmt.Sprintf("- %s\n  %s\n", prompt, images.AbsoluteURL(rec.ID)))
			}
			return sb.String(), nil
		},
	})

	// weather_forecast — current conditions plus the next few days (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "weather_forecast",
		Description: "Get the weather forecast for a location (current conditions plus the next few days).",
		WalletOp:    quota.OpWeatherForecast,
		Params: []api.ToolParam{
			{Name: "lat", Type: "number", Description: "Latitude of the location", Required: true},
			{Name: "lon", Type: "number", Description: "Longitude of the location", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			lat := argFloat(args["lat"])
			lon := argFloat(args["lon"])
			if lat == 0 && lon == 0 {
				return "Provide lat and lon for the location.", fmt.Errorf("missing coordinates")
			}
			// Call the weather capability through go-micro.
			var rsp weather.ForecastResponse
			if err := service.Call(context.Background(), "weather", "Server.Forecast",
				&weather.ForecastRequest{Lat: lat, Lon: lon}, &rsp); err != nil {
				return "", err
			}
			return rsp.Summary, nil
		},
	})

	// social — latest social feed (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "social_list",
		Aliases:     []string{"social"},
		Description: "Get the latest social posts from the network.",
		Handle: func(args map[string]any) (string, error) {
			var rsp social.ListResponse
			if err := service.Call(context.Background(), "social", "Server.List",
				&social.ListRequest{}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// video — latest videos from curated channels (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "video_list",
		Aliases:     []string{"video"},
		Description: "Get the latest videos from curated channels.",
		Handle: func(args map[string]any) (string, error) {
			var rsp video.ListResponse
			if err := service.Call(context.Background(), "video", "Server.List",
				&video.ListRequest{}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// blog_list — recent blog posts (AI-first).
	api.RegisterTool(api.Tool{
		Name:        "blog_list",
		Description: "Get recent blog posts (titles, snippets and ids; use blog_read for one in full).",
		Handle: func(args map[string]any) (string, error) {
			var rsp blog.ListResponse
			if err := service.Call(context.Background(), "blog", "Server.List",
				&blog.ListRequest{}, &rsp); err != nil {
				return "", err
			}
			return rsp.Text, nil
		},
	})

	// Cards used to be attached here, tool by tool, in six lines that had to be
	// kept in step with the tool names they referenced. They are declared on
	// each service's Spec now and derived wherever one is wanted — see
	// internal/api/card.go.

	// Register apps MCP tools
	api.RegisterTool(api.Tool{
		Name:        "apps_search",
		Description: "Search the apps directory for small, useful tools",
		Method:      "GET",
		Path:        "/apps",
		Params: []api.ToolParam{
			{Name: "query", Type: "string", Description: "Search query (name, description, or tag)", Required: false},
			{Name: "tag", Type: "string", Description: "Filter by tag", Required: false},
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_read",
		Description: "Read details of a specific app by its slug",
		Method:      "GET",
		Path:        "/apps",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "The app's URL slug (e.g. pomodoro-timer)", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			slug, _ := args["slug"].(string)
			if slug == "" {
				return `{"error":"slug is required"}`, fmt.Errorf("missing slug")
			}
			a := apps.GetApp(slug)
			if a == nil {
				return `{"error":"app not found"}`, fmt.Errorf("not found")
			}
			b, _ := json.Marshal(a)
			return string(b), nil
		},
	})
	api.RegisterTool(api.Tool{
		Name:        "apps_create",
		Description: "Create a new app — a small, self-contained HTML tool hosted on Mu",
		Method:      "POST",
		Path:        "/apps/new",
		Params: []api.ToolParam{
			{Name: "name", Type: "string", Description: "App name (e.g. Pomodoro Timer)", Required: true},
			{Name: "slug", Type: "string", Description: "URL-friendly ID (e.g. pomodoro-timer)", Required: true},
			{Name: "description", Type: "string", Description: "Short description of what the app does", Required: true},
			{Name: "tags", Type: "string", Description: "Comma-separated tags (optional)", Required: false},
			{Name: "html", Type: "string", Description: "The app's HTML content (can include inline CSS and JavaScript, max 256KB)", Required: true},
			{Name: "price", Type: "number", Description: "Credits charged per use (0 = free, max 1000)", Required: false},
		},
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "apps_edit",
		Description: "Edit an existing app you own — update its name, description, tags, icon, HTML code, or price",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "The app's URL slug (e.g. pomodoro-timer)", Required: true},
			{Name: "name", Type: "string", Description: "New app name", Required: false},
			{Name: "description", Type: "string", Description: "New description", Required: false},
			{Name: "tags", Type: "string", Description: "New comma-separated tags", Required: false},
			{Name: "html", Type: "string", Description: "New HTML content (max 256KB)", Required: false},
			{Name: "icon", Type: "string", Description: "New SVG icon", Required: false},
			{Name: "price", Type: "number", Description: "Credits charged per use (0 = free, max 1000)", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		slug, _ := args["slug"].(string)
		if slug == "" {
			return `{"error":"slug is required"}`, fmt.Errorf("missing slug")
		}
		name, _ := args["name"].(string)
		description, _ := args["description"].(string)
		tags, _ := args["tags"].(string)
		html, _ := args["html"].(string)
		icon, _ := args["icon"].(string)
		price := -1 // -1 means "not set"
		if p, ok := args["price"].(float64); ok {
			price = int(p)
		}
		// Ownership is bound to the authenticated caller — a user can only edit
		// their own app, never one named by slug alone.
		a, err := apps.UpdateAppOwned(accountID, slug, name, description, tags, html, icon, price)
		if err != nil {
			return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
		}
		b, _ := json.Marshal(a)
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "apps_build",
		Description: "Build a small app from a natural language description, save it, and return the app details with URL. Apps are one of: a tracker (a list you add entries to, optionally totalling a number), a checklist, or a counter.",
		WalletOp:    quota.OpAppBuild,
		Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Description of the app to build (e.g. 'an expense tracker', 'a packing checklist', 'a water intake counter')", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return `{"error":"prompt is required"}`, fmt.Errorf("missing prompt")
		}
		// Owner is the authenticated caller; the author name is resolved
		// server-side (see apps.AuthorNameFor), never taken from the model.
		var rsp apps.BuildResponse
		if err := service.Call(service.WithAccount(context.Background(), accountID), "apps", "Server.Build",
			&apps.BuildRequest{Prompt: prompt}, &rsp); err != nil {
			return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
		}
		b, _ := json.Marshal(map[string]string{
			"name": rsp.Name,
			"slug": rsp.Slug,
			"url":  rsp.URL,
			"run":  rsp.Run,
		})
		return string(b), nil
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "apps_fork",
		Description: "Fork an existing app — creates a copy under your account that you can modify independently",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "Slug of the app to fork", Required: true},
			{Name: "new_slug", Type: "string", Description: "Slug for the forked copy (optional, auto-generated if empty)", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		slug, _ := args["slug"].(string)
		newSlug, _ := args["new_slug"].(string)
		if slug == "" {
			return `{"error":"slug is required"}`, fmt.Errorf("missing slug")
		}
		authorName := "Agent"
		if acc, err := auth.GetAccount(accountID); err == nil {
			authorName = acc.Name
		}
		a, err := apps.ForkApp(slug, newSlug, accountID, authorName)
		if err != nil {
			return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
		}
		b, _ := json.Marshal(map[string]string{
			"name": a.Name,
			"slug": a.Slug,
			"url":  "/apps/" + a.Slug,
		})
		return string(b), nil
	})
	api.RegisterTool(api.Tool{
		Name: "apps_run",
		// What this does, rather than what it sounded like it did.
		//
		// It said "Run JavaScript code in a sandboxed environment and return
		// the result", and it does not return the result — it cannot. Nothing
		// executes here: the code is stored and a URL comes back, and the code
		// runs later, in the browser of whoever opens that URL. An agent
		// calling this for "what is 17% of 4,318" gets a link and no answer,
		// and then has to explain a link to somebody who asked a question.
		//
		// It is a good tool for "show me this working" — a chart, a converter,
		// a small interactive page — and the description now says that instead.
		Description: "Publish a snippet of JavaScript and get back a URL that runs it in a browser. " +
			"Returns a link, not an answer: nothing executes until somebody opens the page, and " +
			"whatever the code returns is displayed there. Use it to show something working — a " +
			"chart, a converter, a small interactive page. It cannot compute a value and hand it " +
			"back, so do not use it for arithmetic or data processing you need the result of.",
		WalletOp: quota.OpAgentQuery,
		Params: []api.ToolParam{
			// The old text promised mu.ai(), mu.web.fetch(), mu.db and mu.store.
			// Two of those do not exist in this page's shim, and the ones that do
			// post messages to a parent window that has no listener, so they
			// never resolve. Advertising them sent agents down a path that hangs.
			{Name: "code", Type: "string", Description: "JavaScript to run in the page. It runs as a " +
				"function body — return a value and it is displayed. Sandboxed in an opaque origin: no " +
				"cookies, no account, no access to this instance's tools.", Required: true},
		},
		Handle: func(args map[string]any) (string, error) {
			code, _ := args["code"].(string)
			if code == "" {
				return `{"error":"code is required"}`, fmt.Errorf("missing code")
			}
			id := apps.CreateRun(code, "agent")
			b, _ := json.Marshal(map[string]string{
				"id":  id,
				"url": "/apps/run?id=" + id,
				"run": "/apps/run?id=" + id + "&raw=1",
			})
			return string(b), nil
		},
	})
	api.RegisterToolWithAuth(api.Tool{
		Name:        "apps_test",
		Description: "Test an app by checking its HTML structure and executing its mu.api calls server-side. Returns which API calls work and which fail.",
		Params: []api.ToolParam{
			{Name: "slug", Type: "string", Description: "The app's URL slug", Required: true},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		slug, _ := args["slug"].(string)
		if slug == "" {
			return `{"error":"slug required"}`, fmt.Errorf("missing slug")
		}
		result := apps.TestApp(slug, accountID)
		b, _ := json.Marshal(result)
		return string(b), nil
	})

	// Register agent MCP tool (also exposed as POST /agent/run on the REST page).
	api.RegisterToolWithAuth(api.Tool{
		Name:        "agent_ask",
		Aliases:     []string{"agent"},
		Description: "Ask an agent on this instance a question in natural language and get a written answer. It has every tool you do and will use several to answer, so reach for it to delegate a whole task — for one fact call that tool directly, and to find content this instance already holds use index_search, which costs nothing. Pass agent to ask one of your own by name instead of the default; agent_list names them.",
		Method:      "POST",
		Path:        "/agent/run",
		WalletOp:    quota.OpAgentQuery,
		Params: []api.ToolParam{
			{Name: "prompt", Type: "string", Description: "Your question or request", Required: true},
			{Name: "agent", Type: "string", Description: "Name or id of one of your agents. Omit for the default", Required: false},
		},
	}, func(args map[string]any, accountID string) (string, error) {
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return `{"error":"prompt is required"}`, fmt.Errorf("missing prompt")
		}
		// Which agent. Without this, agents were reachable only from this
		// instance's own web chat: every client — MCP, Discord, Telegram, the
		// CLI, anything calling agent_ask — got the default assistant no matter
		// how many you had built. An agent you cannot invoke from outside is a
		// preset on a settings page, not something you can use.
		//
		// By name as well as id, because a caller writing this by hand knows
		// what they called it and does not know its uuid.
		opts, err := agent.AskAs(accountID, argString(args, "agent"))
		if err != nil {
			return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
		}
		answer, err := agent.QueryWithOpts(accountID, prompt, opts)
		if err != nil {
			return fmt.Sprintf(`{"error":"%s"}`, err.Error()), err
		}
		return answer, nil
	})

	// agent_list — what you can ask for by name.
	//
	// agent_ask grew an agent parameter, and a parameter naming a thing needs a
	// way to find out what the things are called. Otherwise the only route to
	// your own agent's name is opening the web app, which is the thing an MCP
	// caller is trying not to do.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "agent_list",
		Description: "List the agents on your account, with what each one is for. Use the name with agent_ask.",
	}, func(args map[string]any, accountID string) (string, error) {
		return agent.ListForCaller(accountID)
	})

	// Wallet: one tool, one answer about your money.
	//
	// There were two — wallet_balance for credits and wallet for the Base
	// address — so "what's in my wallet" had two answers depending on which the
	// planner picked. Credits are what calls are charged in and USDC is how you
	// top them up, which makes them two fields of one thing, not two tools. The
	// old name stays as an alias so "my wallet" still resolves.
	//
	// It is also no longer path-backed. Pointing a tool at /wallet meant
	// scraping the wallet web page for data, which is how it came to return
	// 20KB of HTML to anyone who didn't pass a magic query flag.
	api.RegisterToolWithAuth(api.Tool{
		Name:        "wallet_balance",
		Aliases:     []string{"wallet"},
		Description: "Get your credit balance — credits are what calls are charged in.",
	}, func(args map[string]any, accountID string) (string, error) {
		out := map[string]any{"credits": wallet.GetBalance(accountID)}
		// The Base address only when this instance offers crypto top-up, or
		// when the caller already holds USDC there — the same rule the /wallet
		// page follows, so an agent is not told about a way to pay that a
		// person is not offered, and money already held is never hidden.
		if bw, err := wallet.GetOrCreateWallet(accountID); err == nil {
			usdc, raw := wallet.USDCBalance(bw.Address)
			if wallet.CryptoTopupEnabled() || (raw != nil && raw.Sign() > 0) {
				out["address"], out["network"], out["usdc"] = bw.Address, "base", usdc
			}
		}
		b, _ := json.Marshal(out)
		return string(b), nil
	})

	// There is no `pay` tool.
	//
	// It called a paid tool on another MCP server and settled it from the user's
	// Base wallet. The guardrails were sound — only servers the operator listed
	// in X402_SERVERS could be paid, so a prompt-injected agent could not name an
	// attacker's URL, and spendlimit.go capped it at $1 a call and $10 a day.
	//
	// What it did not have was a server to call. X402_SERVERS is unset, so the
	// registry held only "self" — this instance — and calls here are charged in
	// credits, not settled over the wire. The tool could reach exactly one server
	// and its own description told the agent not to use it for that one. It
	// occupied a slot in every tools/list to do nothing.
	//
	// wallet.PayAndCallMCP and the spend limits stay: they are what an operator
	// needs the day there is somewhere worth paying, and the CLI still reaches
	// them. Bringing the tool back is re-registering it here.

	// Deriving from the Specs, and the announcement that the registry is
	// complete, are steps of their own in Run. They used to be the last two
	// lines of this function, which made the hand-written half and the derived
	// half indistinguishable to anything looking at the result — including a
	// test asking which hand-written registrations have become copies of what
	// their Spec already says.
}
