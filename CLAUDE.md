# Mu

A personal home server. News, mail, search, weather, markets, video — the everyday internet, handled by one agent you talk to and run yourself. The big platforms own a service for everything; Mu is the alternative — one home server across all the everyday things, each a real service, and open/self-hostable so you can run the whole stack yourself. Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain (news, markets, mail, weather, blog, social, video, search, places, reminder) is a package under the top level
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
| `news/` | RSS feed aggregation, sentiment tagging |
| `markets/` | Crypto, futures, commodities, currencies via CoinGecko/Yahoo |
| `mail/` | SMTP server, DKIM, inbound filtering |
| `blog/` | Microblogging with AI-generated daily digests |
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
| `wallet/` | Credit system, Stripe, x402 |
| `search/` | Brave web search, readability reader |
| `db/` | Per-user records for services and apps (headless) |
| `web/` | Fetch a URL and return readable content (headless) |
| `index/` | Search across the caller's own content (headless) |
| `docs/` | Embedded documentation served at /docs |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```

## Conventions

- No external dependencies for crypto (secp256k1, RLP, ECDSA implemented in pure Go in `wallet/evm.go`)
- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- All client integrations follow the same pattern: auto-create accounts, conversation history, public/private mode
- The main branch is `main`
- One service per top-level directory, named for the service. See
  `docs/SERVICE_REGISTRY.md` for what is registered, which are headless, which
  are account-scoped, and which are deliberately not exposed to the agent

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
storage are free — see the comment on the cost block in `wallet/wallet.go`.

Abuse control is `auth.CheckPostRate`, not the credit charge. Keep the two jobs
separate: credits price real cost, rate limits stop bots.
