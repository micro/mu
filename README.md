# mu

A personal app platform — blog, chat, news, mail, weather and more. No ads. No tracking. No algorithm.

## Overview

Mu is a self-hosted app platform that brings together the things you do every day on the internet — news, markets, weather, mail, chat — in one place, without ads, algorithms, or tracking.

Pay for the tools, not with your attention.

### What's on the dashboard

- **News** — Headlines from RSS feeds, chronological, with AI summaries
- **Markets** — Live crypto, futures, and commodity prices
- **Weather** — Forecasts and conditions
- **Mail** — Private messaging and email
- **Blog** — Microblogging with daily AI-generated digests
- **Chat** — AI-powered conversation on any topic
- **Video** — YouTube without ads, algorithms, or shorts
- **Web** — Search the web without tracking
- **Agent** — AI assistant that can search, answer, and build across every service
- **Apps** — Build and use small, useful tools

Runs as a single Go binary. Self-host or use [mu.xyz](https://mu.xyz).

## Screenshots

### Home

<img width="3728" height="1765" alt="image" src="https://github.com/user-attachments/assets/75e029f8-5802-49aa-9449-4902be5da805" />

[View more](docs/SCREENSHOTS.md)

## How it works

The home screen shows **cards** — a summary of each service. Each card links to a full page. News card shows headlines, links to `/news`. Markets card shows prices, links to `/markets`. Everything at a glance, details one tap away.

## For developers

Mu exposes a REST API and [MCP](https://modelcontextprotocol.io) server at `/mcp` so AI agents and tools can connect directly.

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp"
    }
  }
}
```

30+ tools — news, search, weather, places, video, email, markets — accessible via MCP. AI agents can pay per-request with USDC through the [x402 protocol](https://x402.org). No API keys. No accounts. Just call and pay.

See [API docs](https://mu.xyz/api) · [MCP docs](docs/MCP.md)

## Pricing

Browsing is included. AI and search features use credits — 1 credit = 1p, pay as you go.

- **Card** — Top up via Stripe. 1 credit = 1p.
- **Crypto** — AI agents pay per-request with USDC via [x402](https://x402.org). No account needed.

See [Wallet & Credits](docs/WALLET_AND_CREDITS.md) for details.

## Self-hosting

```bash
# Install
git clone https://github.com/micro/mu
cd mu && go install

# Configure
export ANTHROPIC_API_KEY=xxx    # AI features (Claude)
export YOUTUBE_API_KEY=xxx      # Video search

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

Full docs at [mu.xyz/docs](https://mu.xyz/docs) or in the [docs](docs/) folder.

## License

[AGPL-3.0](LICENSE) — use, modify, distribute. If you run a modified version as a service, share the source.
