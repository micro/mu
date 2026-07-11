# mu

**An agent for everyday.**

News, mail, search, weather, markets, video — the everyday internet, handled by one agent you just talk to. Ask Mu for anything you'd normally open ten tabs and five apps to do, and get an answer instead.

No ads. No tracking. No algorithm. Pay for the tools, not with your attention.

## Overview

The big platforms have a service for everything — and they own it, and they monetise your attention to do it. Mu is the alternative: one agent across all the everyday things — news, mail, markets, weather, search, video, blog, social, reminders — each a real service the agent operates on your behalf. And because it's open and self-hostable, you can run the whole stack yourself instead of renting each piece from someone else.

The agent remembers your preferences across sessions, surfaces contextual suggestions based on your data, and learns what you care about over time. It isn't a chatbot bolted onto a website — it's the interface to a stack of services that are yours.

Built in the open, on [Go Micro](https://github.com/micro/go-micro). Self-host the whole thing.

### How it works

Open Mu and you see a prompt: **"What do you need?"** Below it, contextual suggestions based on your current state — unread emails, market movements, latest news. Ask a question or tap a suggestion. The AI calls your services, composes an answer, and shows it inline.

Below the AI, cards give you an at-a-glance overview of everything: news headlines, market prices, blog posts, social threads, video. Cards are configurable — show or hide what you care about.

### The services

Each is a service the agent can call — most registered as [Go Micro](https://github.com/micro/go-micro) services — and reachable directly over REST, MCP, A2A, or the CLI.

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
- **Apps** — Build and use small, useful tools — any app can be pinned as a home card
- **Reminder** — A daily Islamic reminder surfaced as a home card and MCP tool
- **Stream** — Public event feed for agents and tools to subscribe to

Runs as a single Go binary. Self-host your own instance.

## Built on Go Micro

Mu is built on [Go Micro](https://github.com/micro/go-micro) and dogfoods it as a real-world reference app. Each domain — news, markets, weather, mail, search, video, blog, social and more — is registered as a Go Micro service, so the agent can call it and it's reachable over REST, MCP, A2A, and the CLI from one registry.

- **Each capability is a Go Micro service**, registered in-process.
- **The agent calls those services** and composes an answer.
- **One binary.** Everything runs in-process behind an in-memory registry, so self-hosting is a single Go binary — the same services can later be split across processes by swapping the registry.

A service for everything, like the big platforms — except the runtime is open and the instance is yours.

## For developers

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

## Pricing

**Free to use.** Join the [Discord](https://discord.gg/WeMU5AGxD) and start talking to the AI — no account, no payment, no limits.

- **Discord** — free, unlimited
- **Web** — free browsing, 3 free AI queries for guests, credits for more
- **MCP / API** — pay per-request with USDC via [x402](https://x402.org)
- **Self-host** — unlimited, run your own instance with local models

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
