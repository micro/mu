# Mu

Own your services. The everyday internet — news, mail, search, weather, video, markets — runs on services the big platforms own; Mu is that same stack owned by you, with an AI agent across all of it. Built on go-micro: every capability is a go-micro service, the assistant is a go-micro agent. Single binary, self-hostable.

## Architecture

- **Single Go binary** — `mu --serve` starts the web server, `mu <command>` runs CLI
- **Services** — each domain (news, markets, mail, weather, blog, social, video, trade, search, places, reminder) is a package under the top level
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
| `trade/` | DEX trading via Uniswap, automated strategies |
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
| `docs/` | Embedded documentation served at /docs |

## Development

```bash
go build ./...          # build
go test ./... -short    # test
go vet ./...            # vet
```

## Conventions

- No external dependencies for crypto (secp256k1, RLP, ECDSA implemented in pure Go in `trade/tx.go`)
- Settings via `internal/settings/` — reads env vars first, falls back to stored values
- Background loops use goroutines started in `Load()` or `main.go`
- Agent tools registered in `internal/api/mcp.go` (static) and `main.go` (dynamic with handlers)
- All client integrations follow the same pattern: auto-create accounts, conversation history, public/private mode
- The main branch is `main`
