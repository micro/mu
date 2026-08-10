package server

// The tools an agent can call that are written out by hand.
//
// Most are derived from a service's Spec (internal/api/derive.go) and need no
// entry here at all. What is left is the ones that predate deriving, or that
// map onto something other than a single service method.

import (
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
	"mu/service/images"
	"mu/service/index"
	"mu/service/mail"
)

// registerTools declares the hand-written half of the tool registry.
func registerTools() {

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

	// recall tool — unified search across everything mu knows for the caller:
	// the public indexed corpus (news, blog, social, video) plus their own mail.
	if err := service.Register(index.Spec); err != nil {
		app.Log("main", "recall service register failed: %v", err)
	}

	// db_* is derived from service/db's Spec now, not hand-registered here.
	//
	// It was written out because db was held to be storage rather than a
	// service — but a tool with no service behind it sits outside the scoping
	// model, so no scoped agent token could reach it and there was no box on
	// /agents to grant it. See the package comment in service/db.

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

	// Cards used to be attached here, tool by tool, in six lines that had to be
	// kept in step with the tool names they referenced. They are declared on
	// each service's Spec now and derived wherever one is wanted — see
	// internal/api/card.go.

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
