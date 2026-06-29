# go-micro migration — living status

Goal: rebuild mu so go-micro v6 is the core of the system. Every domain
capability is a go-micro service; consumers (agent tools, HTTP) reach them
through go-micro. HTTP only fronts what must be HTTP. Dogfood the framework —
when go-micro is missing something, fix it upstream (we own it).

## Working rules (do not break these)

- **Keep it green.** After every change: `go build ./...` and
  `env -u NO_PROXY -u no_proxy go test ./... -short` must pass before committing.
- **Merge each green increment to `main` yourself** (the user does not review
  PRs): commit on `claude/guest-query-limits-rxda1o`, then
  `git checkout main && git merge --ff-only <branch> && go build ./... && git push origin main && git checkout <branch>`.
  If `main` diverged, `git merge origin/main` into the branch first, resolve, retest.
- **Commit trailer** (every commit):
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>` and
  `Claude-Session: https://claude.ai/code/session_01KdcPjN9ndJwMGKQSRrAGPE`.
- **Dogfood loop:** if go-micro lacks something, fix it there. Use the
  `GITHUB_TOKEN` PAT (admin on micro/go-micro) via the GitHub API: copy the
  module from the go cache to a writable dir, edit, verify with a `go.mod`
  `replace` against a live test, then branch+PR+merge+release via the API
  (API commits are GitHub-verified), bump mu with `go get go-micro.dev/v6@vX`.
  Already shipped: v6.3.1 (Messages), v6.3.2 (WithMaxTokens), v6.3.3 (Atlas streaming).
- **Proxy:** `internal/mesh` bypasses HTTP(S)_PROXY for loopback automatically;
  in-process RPC works without external NO_PROXY. Live AI tests need
  `ATLAS_API_KEY` (the user's test key has been used inline; rotate later).

## The service pattern (how each domain was converted)

1. `pkg/service.go`: a `Server` struct with typed methods
   `func (Server) M(ctx, *Req, *Rsp) error` wrapping existing logic, each with a
   doc-comment + `@example` (go-micro derives the AI-tool schema from these).
2. In `pkg.Load()`: `mesh.Register("pkg", new(Server))`.
3. In `main.go`: reroute the agent tool handler to call the service via
   `mesh.Call(context.Background(), "pkg", "Server.M", &Req{...}, &rsp)`.
4. Add a `pkg/service_test.go` round-trip test.

## Status: core migration COMPLETE and LIVE on micro.mu

go-micro v6 is the core of mu in production (verified via `curl micro.mu/version`
→ `agent:native`, `mcp:go-micro/gateway`, `go_micro:v6.3.8`, 11 services; auto-
deploys on every merge to `main`). What remains is deliberately deferred, not
pending: (a) retiring the hand-rolled planner + `agent/micro` — held until the
native agent's answer quality is validated on real traffic, since it's the
fallback; (b) the `/a2a` cutover — would regress (see #6). The dogfood loop shipped
**8 go-micro releases (v6.3.1–v6.3.8)**, all consumed live.

## Done

- ✅ `internal/mesh` runtime core (registry/client/broker/store, Register, Call, loopback proxy bypass).
- ✅ AI core: `internal/ai.Ask` and `AskStream` both run through go-micro `ai`
  (Atlas/Anthropic/local), history + per-caller token caps preserved.
- ✅ Services (11): weather, news, markets, social, video, blog, search, trade, recall, apps, mail.
- ✅ Native go-micro agent is the **default** for `agent.Query` (AGENT_NATIVE=off opts out), full tool coverage.
- ✅ `/mcp` served by go-micro's `gateway/mcp` (manual resolver of mu's tools; metering/auth preserved).
- ✅ `/version` + `/status` for deploy verification.

## Overnight / unsupervised policy

Only do **additive, low-risk** work unsupervised: new services, new tests,
framework fixes via the dogfood loop (verify before release), cleanups. These
keep mu's behaviour identical. **Do NOT merge behaviour-changing or
contract-changing swaps unsupervised** — specifically: replacing `agent.Query`
with a go-micro agent, and replacing the hand-rolled `/mcp` and `/a2a` gateways.
Build their foundations and tests additively, document the cutover, but leave
the actual swap for a supervised session. When in doubt, pick a SAFE item below
or improve test coverage; never break `main`.

## Remaining (do in order; one increment per commit/merge)

1. ✅ **Cleanup** `internal/ai/providers.go`: removed the now-unused native
   funcs (`generateAtlas`, `generateAnthropic`, `generateAnthropicInternal`,
   `readAnthropicStream`, `generateLocalOpenAI`) + dead imports (−353 lines).
2. ✅ **recall**: `RecallServer.Search` registered via mesh (handler in main,
   wraps `recallSearch` over data + mail); the `recall` tool calls it over RPC.
3. ◑ **apps**: `apps.Server.Build` registered via mesh; the `apps_build` tool
   calls it over RPC. NOTE: `apps_search`/`apps_read` are HTTP-path tools
   (Method/Path, no Go handler) — they belong with the MCP gateway work (#5),
   not a direct mesh.Call reroute. `apps_fork`/`apps_run`/`apps_create`/
   `apps_edit` have handlers and could get service methods later if needed.
4. **Agent pipeline**: route `agent.Query` (the `agent` tool + assistant) through
   a go-micro agent over the registered services, instead of the hand-rolled
   planner in `agent/` + `agent/micro/`. This is the big one — verify carefully.
   - ◑ Foundation done: `mesh.NewAgent(name, prompt, provider, key, services)`
     builds a go-micro agent on the in-process mesh (shares registry/client/
     store). Live-verified: an agent over a registered service calls its method
     and answers from the result (internal/mesh/agent_live_test.go).
   - ◑ Wired (opt-in): `agent/native.go` `queryNative` — when `AGENT_NATIVE` is
     set, the catch-all `agent.Query` uses a go-micro agent (`mesh.NewAgent`,
     deepseek-v4-pro, MaxSteps 6) over the registered services. Account context
     is injected into account-scoped tool calls via a `WrapTool` middleware, so
     recall/trade stay correctly scoped. User context + history + guest service
     filtering preserved. Default OFF = no regression. Live-verified end to end.
   - ✅ **Default on.** The go-micro agent is now the default for the catch-all
     path (it's an agent platform). `AGENT_NATIVE=off` opts out. If no LLM
     provider is configured, or a native run errors, it falls through to the
     hand-rolled pipeline so a query never hard-fails.
   - ✅ Full tool coverage for the native agent: added `search.Server.Fetch`
     (web_fetch) and `apps.Server.Search`/`apps.Server.Read` (apps_search/read).
     The search and apps services are in nativeServices, so the agent
     auto-discovers these methods.
   - TODO: validate answer quality/latency vs the hand-rolled path on real
     traffic and tune the system prompt; then retire the hand-rolled planner +
     (eventually) `agent/micro`.
   - NOTE confirmed go-micro handler rules: the handler type AND its method
     request/response types must be exported, or rpc.Register rejects them.
5. ◑ **MCP gateway** **[NEEDS SUPERVISION for the swap]**: changing `/mcp` is an
   external contract.
   - ✅ Side-by-side (additive, opt-in): `mesh.StartMCPGateway(addr)` runs
     go-micro's `gateway/mcp` on a separate port; main starts it when
     `MCP_GATEWAY_ADDR` is set (default off, real `/mcp` untouched). It
     auto-exposes every registered service as an MCP tool. Verified:
     `/health` → `{"status":"ok","tools":N}`, `/mcp/tools` lists the services'
     methods (schemas from the handler signatures + `@example`), plus go-micro's
     built-in store/broker tools.
   - ✅ **Cutover done (default on, no env var).** `POST /mcp` is served by
     go-micro's `gateway/mcp` via a **manual resolver** of mu's tools
     (`internal/api/mcp_micro.go`): go-micro owns the MCP protocol/transport;
     mu keeps the per-IP guard, wallet metering, auth and dispatch
     (`ExecuteTool`). `GET /mcp` still renders mu's own doc page. No framework
     internals exposed. Every prior behaviour preserved (notifications→204,
     tool errors→isError results, quota→-32000, protocolVersion 2025-03-26,
     serverInfo name=mu, named not-found). Full suite green.
   - Dogfood releases this required: **v6.3.6** (Resolver + mountable JSON-RPC
     `NewHandler`), **v6.3.7** (richer `Call` → `CallResult`/isError + coded
     `RPCError`; notifications), **v6.3.8** (`WithServerInfo`/`WithProtocolVersion`
     + named not-found).
   - ✅ Cleanup done: removed the now-dead `mcpPostHandler`/`handleInitialize`/
     `handleToolsList`/`handleToolsCall` (~192 lines) from internal/api/mcp.go.
6. ⛔ **A2A gateway — deliberately deferred (would regress).** go-micro's
   `gateway/a2a` (`NewAgentHandler(card, invoke)`) is a **single-agent** model:
   one card + one `invoke(text)→string`. mu's `internal/a2a` (382 lines) is
   richer — **multi-skill** routing built from the registered micro-agents plus
   task lifecycle (`message/send`, `tasks/get`, `tasks/cancel`). Cutting `/a2a`
   over to go-micro now would drop skills and task management. Revisit only if
   go-micro's a2a gateway grows multi-skill + task support (an upstream project),
   or if A2A usage justifies it. mu's `/a2a` stays as-is — and it already runs
   on top of the (now go-micro-backed) AI core and services.
7. ◑ **[SAFE]** Register the remaining agent-facing domains as go-micro services.
   - ✅ **mail**: `mail.Server.Search` (rune-safe formatting) registered via
     mesh; added to the native agent's services so it can search mail directly
     (account id injected via the WrapTool middleware). Round-trip test added.
   - TODO: places, reminder, chat lack clean AI-first text accessors (places
     search is HTTP-handler-based; reminder only has ReminderHTML; chat is
     HTTP). Each needs a small text accessor written before a service wrapper —
     lower value (overlap with existing tools), do when convenient.
8. ✅ **Streaming usage**: go-micro v6.3.4 (#3304) surfaces token usage on
   streams (atlascloud/openai request `stream_options.include_usage` and return
   the final usage chunk). `streamViaMicro` now records it — closes the
   usage-accounting regression from routing streams through go-micro.

## Notes / gaps filed or to file in go-micro

- Token usage on streaming responses is not surfaced (mu records 0 for streamed
  calls). Candidate for a v6.3.x fix.
