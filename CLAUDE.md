# Mu

**Tools for agents.** The everyday internet — news, mail, search, weather, markets, video, storage — as tools an agent can call over MCP and REST, paid per request in USDC via x402, with no account in the way.

The claim that matters is *one account instead of a hundred*. An agent that wants news, mail, search, weather, markets, places and somewhere to keep records otherwise needs six or seven providers: six signups, six cards on file, six tokens to rotate. Mu is one balance and one protocol, and for an agent paying per request over x402, no signup at all. **Removing the barrier is the product.** Sometimes that means running the thing ourselves — the SMTP server with DKIM is real, because a sending domain is not something you can casually acquire. Sometimes it means paying a provider so the caller doesn't have to hold that relationship. Both are legitimate; the test is whether the caller is spared an account, not whether we wrote the backend.

This replaces an earlier line — *real tools, not wrappers* — which was wrong in a way that cost us. It made "did we build the backend" the measure, which caps breadth at what one team can operate, and breadth concentrated behind one account is the whole value. Depth still matters where depth is what removes the barrier. It is not the point on its own.

Two doors onto one set of services. An agent calls `/mcp`; a person signs in and gets the home screen — cards per service, agent inline, apps, wallet. Nothing is built twice, and a new service appears in both at once. **Keep the signed-in app intact** — it is not legacy, it is the proof the tools work.

Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable — run an instance and anyone paying to call your tools pays you.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain is a package under `service/`, one directory per service
- **Agents** — `agent/micro/` contains specialised micro-agents per domain, routed by keyword + LLM
- **Channels** — Discord (`client/discord/`), Telegram (`client/telegram/`), WhatsApp (`client/whatsapp/`)
- **Protocols** — MCP server at `/mcp`, A2A at `/a2a`, x402 crypto payments
- **AI** — `internal/ai/` supports Anthropic Claude, Atlas Cloud (DeepSeek), and local models (Ollama)
- **Config** — `internal/settings/` for live-reloadable settings, admin UI at `/admin/env`

## Key Packages

| Package | Purpose |
|---------|---------|
| `agent/` | Main agent pipeline (plan → execute → synthesise) |
| `agent/micro/` | Multi-agent system — registry, router, executor, orchestrator |
| `service/news/` | RSS feed aggregation, sentiment tagging |
| `service/markets/` | Crypto, stocks, futures, commodities, currencies via CoinGecko/Yahoo |
| `service/mail/` | SMTP server, DKIM, inbound filtering |
| `service/blog/` | Microblogging with AI-generated daily digests |
| `internal/ai/` | LLM abstraction — Anthropic, Atlas Cloud, local models |
| `internal/api/` | MCP server, tool registry |
| `internal/app/` | Web UI framework, templates, middleware |
| `internal/auth/` | Account system, sessions, passkeys |
| `internal/memory/` | Per-user persistent memory with scoped namespaces |
| `internal/settings/` | Live-reloadable configuration |
| `home/` | Landing page, assistant, home dashboard, summary |
| `client/discord/` | Discord bot with slash commands, embeds, briefings |
| `client/telegram/` | Telegram bot with commands and groups |
| `client/whatsapp/` | WhatsApp Business API integration |
| `billing/` | What this instance charges for and how it gets paid — prices, quota, Stripe, x402. Under everything; imports no service |
| `service/wallet/` | The caller's side of the money: balance, top up, transfer, the /wallet page and the `wallet_*` tools — a service over `billing/` |
| `service/search/` | Brave provider, readability reader, the /search page (no service of its own) |
| `service/db/` | The caller's own records — named collections that outlive a conversation. Apps keep a separate store each |
| `service/files/` | Per-user file storage — keep a file, get a URL, read it back |
| `service/contacts/` | The caller's address book, so a name resolves to an address (headless) |
| `service/web/` | The open web: search it (`web.Search`), fetch a URL (`web.Fetch`) |
| `service/index/` | Search across the caller's own content (headless) |
| `service/stream/` | The console — this instance's own event timeline |
| `service/chat/` | Live discussion rooms attached to an item |
| `internal/user/` | Profiles, status and presence — the public face of an account |
| `docs/` | Embedded documentation served at /docs. `PRODUCT.md` is the one-pager: what Mu is, who arrives, and what Home is for. Check changes against it |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```

## Conventions

- No external dependencies for crypto (secp256k1, RLP, ECDSA implemented in pure Go in `billing/evm.go`)
- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- All client integrations follow the same pattern: auto-create accounts, conversation history, public/private mode
- The main branch is `main`
- One directory per service under `service/`, named for the service. `internal/service`
  is the runtime core that hosts them, not a service itself. See
  `docs/ARCHITECTURE.md` for what is registered, which are headless, which
  are account-scoped, and which are deliberately not exposed to the agent
- A service is named for a **domain** (a noun), never an action. Tool names are
  derived as `service_method`, so an action-named service leaves its main method
  nothing to be called but the same word — that is how `search.Search` produced
  the tool name `search_search`. Methods returning the current set of something
  are all called `List`. Enforced by `TestNoMethodRepeatsItsService`

## The go-micro relationship

go-micro is the substrate Mu is built on. It has three roles, and they must stay separate.

**One-way dependency.** Mu depends on go-micro. go-micro must never depend on Mu.
go-micro changes when *framework users* need something — never because Mu needs
something. If Mu needs a capability go-micro lacks, it goes in Mu's own code, or
Mu vendors or forks. A PR to `micro/go-micro` whose justification is "Mu needs
this" is the failure mode: reject it and put the code in `micro/mu`.

This is a policy, not an architecture. It is cheap to hold, and it targets a
specific failure already lived through once: in 2019 a business was built that
*depended on* go-micro, so the framework had two masters — a library wants
stability and generality, a runtime wants opinionated constraints — and the
engineering budget went on reconciling them instead of shipping either.

Mu pins a released go-micro version. Forking is permitted and expected.

**Not in the product surface.** A Mu user should never meet the words *go-micro*,
*microservice*, or *framework* in the UI, in onboarding, in marketing copy, or in
anything the agent says about itself. If a user-visible string mentions them,
that is a bug. This covers the agent's system prompts and the public status page,
which are easy to forget.

The word *service* is fine — it is ordinary English, and the internal convention
(service name == route == nav label == tool prefix) is good and invisible to
users. What is banned is implementation vocabulary, not the concept.

The README and developer docs are exempt: they are the funnel, and they are
facts. The distinction is developer-facing (say it) vs customer-facing (don't).

## Pricing

A credit is charged when an operation costs us something to run: a model call,
or a paid third party (Atlas Cloud for inference and images, Brave for web
search, Google for places). Operations that only touch this instance's own
storage are free — see the comment on the cost block in `billing/billing.go`.

Abuse control is `auth.CheckPostRate`, not the credit charge. Keep the two jobs
separate: credits price real cost, rate limits stop bots.
