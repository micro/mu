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

## Remaining (do in order; one increment per commit/merge)

1. ✅ **Cleanup** `internal/ai/providers.go`: removed the now-unused native
   funcs (`generateAtlas`, `generateAnthropic`, `generateAnthropicInternal`,
   `readAnthropicStream`, `generateLocalOpenAI`) + dead imports (−353 lines).
2. **recall** tool (main.go, auth-scoped, calls `recallSearch`): move the search
   into a go-micro service (e.g. `corpus`/`recall`) and route the tool through it.
3. **apps**: expose the apps/micro-app capabilities (search/read/build) as a
   go-micro service; route `apps_*` tools through it.
4. **Agent pipeline**: route `agent.Query` (the `agent` tool + assistant) through
   a go-micro agent (`agent.New(agent.Services(...))`) over the registered
   services, instead of the hand-rolled planner in `agent/` + `agent/micro/`.
   This is the big one — verify carefully, keep behavior sane.
5. **MCP gateway**: replace the hand-rolled `/mcp` (`internal/api`) with
   go-micro `gateway/mcp`, tools auto-derived from the registered services.
6. **A2A gateway**: replace `/a2a` with go-micro `gateway/a2a`.
7. **Other domains** worth registering as services if agent-facing: mail,
   places, reminder, chat.
8. **Streaming usage**: go-micro stream responses don't return token usage;
   consider a follow-up go-micro fix so `recordUsage` is accurate for streams.

## Notes / gaps filed or to file in go-micro

- Token usage on streaming responses is not surfaced (mu records 0 for streamed
  calls). Candidate for a v6.3.x fix.
