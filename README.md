# mu

Your personal AI — news, mail, markets, weather, search and more, all through one interface. No ads. No tracking. No algorithm.

## Overview

Mu is a personal AI platform. Ask it anything — it checks your mail, looks up prices, searches the web, reads the news, and gives you a personalised answer. Every service is a tool the AI can use on your behalf.

The AI remembers your preferences across sessions, surfaces contextual suggestions based on your data, and learns what you care about over time.

Built in the open. Pay for the tools, not with your attention.

### How it works

Open Mu and you see a prompt: **"What do you need?"** Below it, contextual suggestions based on your current state — unread emails, market movements, latest news. Ask a question or tap a suggestion. The AI checks your services, composes an answer, and shows it inline.

Below the AI, cards give you an at-a-glance overview of everything: news headlines, market prices, blog posts, social threads, video. Cards are configurable — show or hide what you care about.

### What's included

- **AI Agent** — Ask anything. It searches news, checks markets, reads your mail, fetches weather, searches the web, and synthesises an answer. Remembers your preferences.
- **News** — Headlines from RSS feeds, chronological, with AI summaries
- **Markets** — Live crypto, futures, and commodity prices
- **Mail** — Private messaging and email
- **Blog** — Microblogging with daily AI-generated digests
- **Chat** — Conversational AI with session history
- **Video** — YouTube without ads, algorithms, or shorts
- **Web** — Search the web without tracking
- **Weather** — Forecasts and conditions
- **Apps** — Build and use small, useful tools — any app can be pinned as a home card
- **Stream** — Public event feed for agents and tools to subscribe to

Runs as a single Go binary. Self-host your own instance.

## For developers

Mu exposes a REST API and [MCP](https://modelcontextprotocol.io) server at `/mcp` so AI agents and tools can connect directly.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://your-instance.com/mcp"
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

## Pricing

Browsing is included. AI and search features use credits — 1 credit = 1p, pay as you go.

- **Starter** — £5/month, 500 credits
- **Pro** — £10/month, 1,200 credits
- **Pay as you go** — Top up any amount via Stripe
- **Crypto** — AI agents pay per-request with USDC via [x402](https://x402.org). First 10 calls free.
- **Self-host** — Unlimited. Run your own instance with local models.

See [Wallet & Credits](docs/WALLET_AND_CREDITS.md) for details.

## Self-hosting

```bash
# Install
git clone https://github.com/micro/mu
cd mu && go install

# Configure (choose your AI provider)
export ANTHROPIC_API_KEY=xxx              # Anthropic Claude
# or
export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama / local models
export OPENAI_API_KEY=ollama

# Run
mu --serve
```

Go to localhost:8081. See [Installation guide](docs/INSTALLATION.md) for full setup.

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
