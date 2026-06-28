# Building mu on go-micro v6

Findings from evaluating whether mu's services and agents should become
[go-micro v6](https://go-micro.dev) services and agents — i.e. dogfooding the
framework to build the product on it.

Status: **research + a runnable spike** (`examples/gomicro-weather/`). No
production code changed; `go-micro.dev/v6` is **not** in mu's main `go.mod` yet.

## The headline

go-micro's `master` is **v6** (`go-micro.dev/v6`, v6.3.0) and it is now an
**agent harness and service framework**, not just microservices plumbing. Over
the past year mu hand-rolled, by necessity, most of what v6 now ships as
primitives. The two have massively converged.

```go
// A service is a struct with methods. Doc-comments + @example become tool schemas.
type Weather struct{}
// Forecast returns a short weather summary for a place.
// @example {"location": "London"}
func (Weather) Forecast(ctx, req *ForecastRequest, rsp *ForecastResponse) error { … }

svc := micro.NewService("weather"); svc.Handle(new(Weather)); svc.Run()
// → exposed as REST + gRPC + MCP + A2A, and usable as an agent tool, automatically

agent := micro.NewAgent("assistant",
    micro.AgentServices("weather"),          // other services become its tools
    micro.AgentProvider("atlascloud"))       // mu's existing provider
```

## mu's hand-rolled subsystems → go-micro v6 primitives

| mu today | go-micro v6 |
|---|---|
| `internal/ai` (Anthropic/Atlas/Ollama, `ai.Ask`) | `ai` package — **Atlas is a first-class provider** (`ai.New("atlascloud", …)`), + Anthropic/OpenAI/Gemini/Groq/Mistral/Together |
| `internal/api/mcp.go` tool registry (`RegisterTool`, `Tool{}`) | methods auto-become MCP/agent tools from doc-comments; `gateway/mcp` |
| `/mcp`, `/a2a`, x402 (`wallet`) | `gateway/mcp`, `gateway/a2a`, built-in **x402 payments** |
| `agent/` + `agent/micro/` (router, executor, orchestrator) | `agent` package + built-in **Plan & Delegate**, guardrails (MaxSteps, LoopLimit, ApproveTool), tool middleware |
| `internal/memory` (scoped namespaces) | `agent` `Memory` (store-backed or in-memory) |
| `internal/settings` live config | `config` |
| background goroutine loops | `flow` — durable, event-triggered workflows |
| per-service HTTP handlers | `web.Service` (HTTP handlers with discovery) |

The value of dogfooding is exactly here: mu is the proof that v6 can build a
real product, and the place its gaps surface.

## What the spike proved (`examples/gomicro-weather/`)

Ran against the real framework, in-process, with mu's Atlas key:

- **Conversion is a method.** mu's `weather.ForecastText` + its hand-registered
  tool becomes one typed `Forecast` method. ✓
- **It's a normal RPC service** — direct typed `client.Call` works. ✓
- **The AI tool is auto-derived** (name + JSON schema) from the method and its
  `@example` comment — zero hand-written registration. This replaces
  `internal/api/mcp.go`. ✓
- **An agent answers end to end** — an Atlas-Cloud agent discovers the tool,
  calls it, and synthesises an answer from the result, with no tool glue. ✓

### Operational notes found while doing it

- **Proxy / address.** go-micro's HTTP transport honours `HTTP(S)_PROXY` and
  advertises a non-loopback IP (it picked the container's default-route IP).
  Under a proxy, in-process loopback RPC is hijacked and fails with
  `502 Bad Gateway` until `NO_PROXY` includes loopback + the advertised IP.
  Relevant to mu's deployment environment; trivial to handle.
- **Model matters for synthesis.** With Atlas, the default `llama-3.3-70b`
  often called the tool but didn't loop back to a final answer;
  `deepseek-v4-pro` reliably grounded answers in the tool result. mu already
  uses deepseek on Atlas, so this is a non-issue in practice.

## The one real impedance mismatch

go-micro is RPC-first (typed `req/rsp` methods); mu is **web-first** (HTML UI,
design system, passkey auth/sessions, single binary). Services don't map onto
`Handle` for free where they're really web handlers. But:

- Services can run **in-process** behind an in-memory registry (the spike does
  this), so adopting go-micro does **not** force mu to physically distribute.
  The monolith stays one binary; internally it becomes a fleet of in-process
  services + agents, with the web UI as a `web.Service` in front.
- `web.Service` + the gateways cover the HTTP/MCP/A2A surface.

So the realistic path is **monolith-of-services**, not a big-bang rewrite.

## "Run somewhere" / the offering

A deployment platform is low-value. The framing that fits: **mu, rebuilt on
go-micro, _is_ the offering** — a personal AI-agent platform where every
capability is a discoverable service/agent (REST + MCP + A2A for free).
go-micro already answers "run somewhere": `micro run` + one-command SSH/systemd
deploy. The thing you run is the mu fleet.

## Recommended path (incremental, in-process)

1. **Keep this spike** as a committed, runnable reference (done — separate
   module, doesn't touch mu's `go.mod`).
2. **Convert one real service in-process** inside the mu binary — weather or
   news — wrapping the existing domain logic (`weather.FetchWeather`), deleting
   that domain's hand-rolled tool/AI glue, proving it against mu's real config
   and auth. This is the first step that adds `go-micro.dev/v6` to mu's
   `go.mod`.
3. **Stand a go-micro agent** over the converted services and route one channel
   (e.g. the web assistant) through it; compare with `agent/micro`.
4. **Expand service by service**, retiring `internal/ai`, `internal/api`,
   `agent/micro`, and the x402 plumbing as the framework subsumes them.
5. **Distribute only if/when scale demands it** — flip in-process services to
   separate processes via the registry; no code change to the handlers.

Each step is reversible and produces evidence before the next. The big-bang
full conversion is **not** recommended.
