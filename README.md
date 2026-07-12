# mu

**Your everyday internet — one home you own.**

News, mail, markets, weather, video, search — the everyday things, on one screen that's yours. Glance at your day; ask the agent when a glance isn't enough. No ads, no algorithm, no attention economy. Open and self-hostable, so the whole stack is yours.

## Overview

Most of the everyday internet is glanceable — headlines, prices, the weather, what's unread. You don't want a conversation for that; you want to *look*. So Mu is a home screen first: cards that show you your day at a glance, the way a dashboard should.

But some things a glance can't do — reply to that email, explain why the market moved, plan the trip, pull a thread together. That's the agent's job. It isn't a chatbot bolted on the side; it's woven through, right where you're already looking, so seeing turns into doing without switching tools.

And the whole thing is yours. Every capability — news, mail, markets, weather, search, video, blog, social, reminders — is a real service you can run yourself, on your own machine, with your own data. The big platforms rent you a service for everything and take your attention as the fee. Mu is the opposite: open, self-hostable, no ads, no tracking.

Three ideas, one product:

- **Glance is the habit** — cards give you a reason to open it every day.
- **The agent is the value** — it does the things a widget can't.
- **Ownership is the moat** — it's your stack, not rented from a platform.

### How it works

Open Mu and you see your day: news, markets, mail, weather — whatever you keep, laid out to glance at, not to read. Pin the apps and cards you care about; hide the rest.

When a glance isn't enough, the agent is right there. Ask it anything and it calls your services — news, markets, mail, weather, search and more — does the work, and answers in place. It remembers what you care about across sessions, so it gets more useful the more you use it.

### The services

Each is a real service — reachable in the app, and directly over REST, MCP, A2A, or the CLI. The agent calls them on your behalf; you can also use any of them on its own.

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

## For developers

Under the hood, every capability is a service. Mu runs them in-process behind an in-memory registry — built on [Go Micro](https://github.com/micro/go-micro) — so the whole thing self-hosts as a single Go binary, and the same services can later be split across processes by swapping the registry.

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
