# mu

A personal home server

## Overview

A personal home server. One Go binary runs a set of everyday services — news, mail, markets, weather, search, images, video, blog, social, places, reminders — behind a [Go Micro](https://github.com/micro/go-micro) registry, with an LLM agent that calls them as tools. The same services are reachable as a web app, a REST API, an MCP server, an A2A endpoint, and a CLI.

Use it hosted at [micro.mu](https://micro.mu), or self-host the single binary — same product either way. Open source, AGPL-3.0.

## Features

- **All services in one process.** Each domain (news, markets, mail, weather, …) is a Go Micro service with typed handlers, registered in-process behind an in-memory registry. One binary, no external infrastructure; the same handlers can later be split across processes by swapping the registry.
- **An agent over those services.** An LLM — Claude, Atlas Cloud (DeepSeek), or a local Ollama / OpenAI-compatible endpoint — calls the services as tools, composes answers, and keeps per-user memory across sessions.
- **A web UI that's a home screen.** Cards render each service at a glance (headlines, prices, weather, unread mail); the agent sits inline to act on what you're looking at. Logged-out visitors get a public version with live public data.
- **Several front doors to the same services.** A REST API, an MCP server at `/mcp`, an A2A endpoint at `/a2a`, and a CLI where every tool is a subcommand. API and MCP callers can pay per request in USDC via [x402](https://x402.org).

## Services

Each is reachable in the web app and directly over REST, MCP, A2A, or the CLI. The agent calls them as tools; each is also usable on its own.

- **Agent** — Ask anything. It calls news, markets, mail, weather, search and more, then synthesises an answer. Remembers your preferences.
- **Chat** — Conversational AI with session history
- **News** — Headlines from RSS feeds, chronological, with AI summaries
- **Markets** — Live crypto, futures, commodity, and currency prices
- **Weather** — Forecasts and conditions
- **Mail** — Private messaging and email (SMTP server with DKIM, inbound filtering)
- **Social** — Public discussion threads
- **Blog** — Microblogging with daily AI-generated digests
- **Video** — YouTube without ads, algorithms, or shorts
- **Images** — Generate images from a prompt, plus a daily nature / mindful image
- **Search** — Search the web without tracking, with a clean reader view
- **Places** — Search places and nearby results with configured providers and open-data fallbacks
- **Islam** — A daily Islamic reminder (verse, hadith, reflection), also an MCP tool
- **Apps** — Build and use small, useful tools (like **Saved**, a built-in read-later list) — pin any app to the top of your home screen
- **Stream** — Public event feed for agents and tools to subscribe to

## Accounts & sign-in

Sign in to the web app with a username and password, a **passkey** (WebAuthn), or **Google**. Already have an account? Link Google to it from **Account** settings and use Google sign-in from then on. For the API and CLI, generate a Personal Access Token at `/token`.

Passkeys work out of the box. To enable Google sign-in when self-hosting, set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` (and optionally `GOOGLE_REDIRECT_URI`, which defaults to `<your-origin>/oauth2/callback`) from `/admin/env` or the environment.

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

40+ tools — news, search, weather, places, video, email, markets, images — accessible via MCP. AI agents can pay per-request with USDC through the [x402 protocol](https://x402.org). No API keys. No accounts. Just call and pay. First 10 calls per wallet are free.

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

Discord slash commands: `/agent`, `/news`, `/markets`, `/weather`, `/mail`, `/social`, `/blog`, `/video`, `/search`, `/apps`, `/balance`, `/usage`.
Telegram commands: `/agent`, `/ask`, `/news`, `/markets`, `/weather`, `/usage`.

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

Once you're admin, every other key (YouTube, Brave search, weather, mail/DKIM,
Google sign-in…) is configurable from `/admin/env` in the browser.

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
