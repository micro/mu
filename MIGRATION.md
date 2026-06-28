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

## Done

- ✅ `internal/mesh` runtime core (registry/client/broker/store, Register, Call, loopback proxy bypass).
- ✅ AI core: `internal/ai.Ask` and `AskStream` both run through go-micro `ai`
  (Atlas/Anthropic/local), history + per-caller token caps preserved.
- ✅ Services: weather, news, markets, social, video, blog, search, trade.

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
   - TODO **[SUPERVISED]**: enable `AGENT_NATIVE` (via /admin/env), compare
     answers/latency vs the hand-rolled path across query types (weather, news,
     prices, recall, guest), tune the system prompt to parity, then flip the
     default and retire the hand-rolled planner + (eventually) `agent/micro`.
   - NOTE confirmed go-micro handler rules: the handler type AND its method
     request/response types must be exported, or rpc.Register rejects them.
5. **MCP gateway** **[NEEDS SUPERVISION for the swap]**: changing `/mcp` is an
   external contract. Safe now: stand up go-micro `gateway/mcp` on a side path
   and diff its tool list against the current `/mcp`. Swap only when supervised.
6. **A2A gateway** **[NEEDS SUPERVISION for the swap]**: same as #5 for `/a2a`.
7. **[SAFE]** Register the remaining agent-facing domains as go-micro services
   (mail, places, reminder, chat) — additive; consumers/gateway come later.
8. **[SAFE]** Streaming usage: go-micro stream responses don't surface token
   usage, so `recordUsage` records 0 for streamed calls. Fix upstream via the
   dogfood loop (add Usage to the final stream chunk / a Usage() on the stream),
   release, then record it in `streamViaMicro`. Verify before releasing.

## Notes / gaps filed or to file in go-micro

- Token usage on streaming responses is not surfaced (mu records 0 for streamed
  calls). Candidate for a v6.3.x fix.
