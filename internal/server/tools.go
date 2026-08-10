package server

// The tools an agent can call that are written out by hand.
//
// Most are derived from a service's Spec (internal/api/derive.go) and need no
// entry here at all. What is left is the ones that predate deriving, or that
// map onto something other than a single service method.

import (
	"fmt"

	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
	"mu/service/index"
)

// registerTools declares the hand-written half of the tool registry.
func registerTools() {

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

	// Cards used to be attached here, tool by tool, in six lines that had to be
	// kept in step with the tool names they referenced. They are declared on
	// each service's Spec now and derived wherever one is wanted — see
	// internal/api/card.go.

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
