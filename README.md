# mu

A personal app server

## Overview

A self-hostable personal app server. One Go binary runs a set of everyday services — news, mail, markets, weather, search, video, blog, social, places, reminders — behind a [Go Micro](https://github.com/micro/go-micro) registry, with an LLM agent that calls them as tools. The same services are reachable as a web app, a REST API, an MCP server, an A2A endpoint, and a CLI.

## Features

- **All services in one process.** Each domain (news, markets, mail, weather, …) is a Go Micro service with typed handlers, registered in-process behind an in-memory registry. One binary, no external infrastructure; the same handlers can later be split across processes by swapping the registry.
- **An agent over those services.** An LLM — Claude, Atlas Cloud (DeepSeek), or a local Ollama / OpenAI-compatible endpoint — calls the services as tools, composes answers, and keeps per-user memory across sessions.
- **A web UI that's a home screen.** Cards render each service at a glance (headlines, prices, weather, unread mail); the agent sits inline to act on what you're looking at. Logged-out visitors get a public version with live public data.
- **Several front doors to the same services.** A REST API, an MCP server at `/mcp`, an A2A endpoint at `/a2a`, and a CLI where every tool is a subcommand. API and MCP callers can pay per request in USDC via [x402](https://x402.org).

### Services

Each is a service, reachable in the web app and directly over REST, MCP, A2A, or the CLI. The agent calls them as tools; each is also usable on its own.

- **Agent** — Ask anything. It calls news, markets, mail, weather, web search and more, then synthesises an answer. Remembers your preferences.
- **News** — Headlines from RSS feeds, chronological, with AI summaries
- **Markets** — Live crypto, futures, commodity, and currency prices
- **Mail** — Private messaging and email
- **Blog** — Microblogging with daily AI-generated digests
- **Chat** — Conversational AI with session history
- **Video** — YouTube without ads, algorithms, or shorts
- **Web** — Search the web without tracking
- **Weather** — Forecasts and conditions
- **Places** — Search places and nearby results with configured providers and open-data fallbacks
- **Apps** — Build and use small, useful tools — pin any app to the top of your home screen
- **Reminder** — A daily Islamic reminder surfaced as a home card and MCP tool
- **Stream** — Public event feed for agents and tools to subscribe to

Runs as a single Go binary. Self-host your own instance.

## For Agents

Because every capability is a service, it's reachable however you like. Mu exposes a REST API and an [MCP](https://modelcontextprotocol.io) server at `/mcp`, so AI agents and tools can connect directly.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

30+ tools — news, search, weather, places, video, email, markets — accessible via MCP. AI agents can pay per-request with USDC through the [x402 protocol](https://x402.org). No API keys. No accounts. Just call and pay. First 10 calls per wallet are free.

See [MCP docs](docs/MCP.md)

## CLI

Every MCP tool is also available as a `mu` subcommand. The same binary runs the server (`mu --serve`) and the CLI.

```bash
mu news                                 # latest news feed
mu news_search "ai safety"              # search news
mu chat "hello"                         # chat with the AI
mu agent "what is the btc price?"       # run the full agent
mu web_search "claude code"             # search the web
mu weather_forecast --lat 51.5 --lon -0.12
mu me                                   # your account
mu help                                 # full tool list
```

The CLI is registry-driven — every tool added to the MCP server automatically becomes a CLI command.

### Authentication

```bash
mu login                  # opens /token in your browser, paste the PAT back
mu config set token xxx   # or set it directly
export MU_TOKEN=xxx       # or use the environment
```

See [CLI docs](docs/CLI.md) for more.

## Discord & Telegram

Talk to the AI agent from Discord or Telegram. Ask questions, check markets, get news — all from chat.

[Join the Discord](https://discord.gg/WeMU5AGxD)

Discord slash commands: `/agent`, `/news`, `/markets`, `/weather`, `/mail`.
Telegram commands: `/ask`, `/news`, `/markets`, `/weather`.

See [Discord docs](docs/DISCORD.md) and [Telegram docs](docs/TELEGRAM.md) for setup.

## Self-hosting

### One-command install

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
```

### Docker

```bash
git clone https://github.com/micro/mu && cd mu
docker compose up
```

### From source

```bash
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```

### First-run setup

Open **http://localhost:8080** and Mu walks you through a one-time setup: create
your admin account and pick an AI provider (Claude, Atlas Cloud, or a local
Ollama / OpenAI-compatible endpoint). That's enough to have a working agent.

Prefer the terminal? Configure the provider headless, then start the server:

```bash
mu setup        # pick a provider, paste a key
mu --serve      # first account you create becomes admin
```

Or set everything by hand if you'd rather:

```bash
export ADMIN=you@example.com          # who's admin (else: first account)
export ATLAS_API_KEY=xxx              # or ANTHROPIC_API_KEY, or OPENAI_BASE_URL
mu --serve
```

Once you're admin, every other key (YouTube, Brave search, weather, mail/DKIM…)
is configurable from `/admin/env` in the browser.

See [Installation guide](docs/INSTALLATION.md) for full setup.

### Configuration

Customise feeds, prompts, and cards by editing JSON files:

- `news/feeds.json` — RSS news feeds
- `chat/prompts.json` — Chat topics
- `home/cards.json` — Home screen cards
- `video/channels.json` — YouTube channels
- `places/locations.json` — Saved locations

See [Environment Variables](docs/ENVIRONMENT_VARIABLES.md) for all options.

## Documentation

Full docs in the [docs](docs/) folder.

## License

[AGPL-3.0](LICENSE) — use, modify, distribute. If you run a modified version as a service, share the source.
