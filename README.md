# mu

Tools for agents

## Overview

Mu is an MCP server and web app for agents and humans. It provides access to the real world through MCP for the agents 
and let's humans browse or interact with same content in a web app. Access news, web search, mail,
markets, weather, video, places, images, files, calendar, contacts and more.

Use it live at [micro.mu](https://micro.mu), or self-host.

## Usage

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

Create a Personal Access Token at
[/token](https://micro.mu/token) and send it as `Authorization: Bearer`.

```bash
curl -X POST https://micro.mu/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Scope the connection to the services you need:

```
https://micro.mu/mcp?tools=news,web,mail
```

That's what gets listed to your agent. Everything else is still callable.

**Browse the tools:** [micro.mu/tools](https://micro.mu/tools) — every tool,
grouped by service, with what each call costs. 

See [MCP docs](docs/MCP.md) for the protocol details.

## The tools

Grouped the way [/tools](https://micro.mu/tools) and the sidebar group them —
one row per service, named for it.

| Area | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_read` · `apps_run` · `apps_search` · `apps_test` — build and run small web tools |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Chat** | `chat` — one question, one answer |
| **Contacts** | `contacts_add` · `contacts_find` · `contacts_list` · `contacts_delete` — turn a name into an address |
| **Events** | `events_create` · `events_free` · `events_list` — schedule, and find when you are free |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` · `files_delete` — keep a file, get a URL |
| **Images** | `images_generate` · `images_search` |
| **Index** | `index_search` — everything this instance holds for you |
| **Islam** | `islam_today` · `islam_prayer` · `islam_qibla` — daily reminder, prayer times, qibla |
| **Mail** | `mail_inbox` · `mail_send` · `mail_address` — a real SMTP server with DKIM, and an address per agent |
| **Markets** | `markets_list` — stocks, crypto, futures, commodities, currencies |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Places** | `places_search` · `places_nearby` · `places_eta` — points of interest, geocoding, travel time |
| **Search** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Storage** | `db_create` · `db_get` · `db_list` · `db_delete` — per-caller records (`db_create` with an `id` updates) |
| **Stream** | `stream_list` · `stream_post` — this instance's own timeline |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Wallet** | `wallet_balance` — credits, and where to send USDC to top up |
| **Weather** | `weather_forecast` — conditions, forecast, pollen |
| **Platform** | `agent` — ask the whole thing a question and let it compose. Also `quran` · `quran_search` · `hadith`, `save` · `unsave` · `saved_list`, and `flag` · `dismiss` · `block_user` · `unblock_user` |

## Request a tool

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) and say what it should do.

## The app

The server includes a web app with a home screen. Cards render each service at a glance (headlines,
prices, weather, unread mail) and the agent sits inline to act on what you're
looking at. Logged-out visitors get a public version with live data.

An LLM — Claude, Atlas Cloud (DeepSeek), or a local Ollama / OpenAI-compatible
endpoint — calls the services as tools, composes answers, and keeps per-user
memory across sessions.

Sign in with a username and password, a **passkey** (WebAuthn), or **Google**.
For the CLI, generate a Personal Access Token at `/token`.

## CLI

Every tool is a `mu` subcommand. The same binary runs the server (`mu --serve`)
and the CLI.

```bash
mu news_list                            # latest headlines
mu news_search "ai safety"              # search news
mu web_search "claude code"             # search the web
mu agent "what is the btc price?"       # run the full agent
mu weather_forecast --lat 51.5 --lon -0.12
mu wallet                               # your balance
mu help                                 # full tool list
```

The CLI is registry-driven — a tool added to the server automatically becomes a
CLI command.

```bash
mu login                  # opens /token in your browser, paste the PAT back
mu config set token xxx   # or set it directly
export MU_TOKEN=xxx       # or use the environment
```

See [CLI docs](docs/CLI.md).

## Discord & Telegram

Talk to the agent from Discord or Telegram — questions, markets, news, all from
chat. [Join the Discord](https://discord.gg/WeMU5AGxD)

Discord: `/agent`, `/news`, `/markets`, `/weather`, `/mail`, `/social`, `/blog`,
`/video`, `/search`, `/apps`, `/balance`, `/usage`.
Telegram: `/agent`, `/ask`, `/news`, `/markets`, `/weather`, `/usage`.

Setup for both is in [Installation](docs/INSTALLATION.md).

## Self-hosting

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
```

```bash
# Docker
git clone https://github.com/micro/mu && cd mu
docker compose up

# From source
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```

### First-run setup

Open **http://localhost:8080** and Mu walks you through a one-time setup: create
your admin account and pick an AI provider (Claude, Atlas Cloud, or a local
Ollama / OpenAI-compatible endpoint). That's enough to have a working agent.

For terminal setup. Configure the provider headless, then start the server:

```bash
mu setup        # pick a provider, paste a key
mu --serve      # first account you create becomes admin
```

Or set everything by hand:

```bash
export ADMIN=you@example.com          # who's admin (else: first account)
export ATLAS_API_KEY=xxx              # or ANTHROPIC_API_KEY, or OPENAI_BASE_URL
mu --serve
```

Once you're admin, every other key (YouTube, Brave search, weather, mail/DKIM,
Google sign-in…) is configurable from `/admin/env` in the browser.

See [Installation guide](docs/INSTALLATION.md).

### Configuration

Customise feeds, prompts and cards by editing JSON files:

- `service/news/feeds.json` — RSS news feeds
- `service/chat/prompts.json` — chat topics
- `home/cards.json` — home screen cards
- `service/video/channels.json` — YouTube channels
- `service/places/locations.json` — saved locations

See [Environment Variables](docs/ENVIRONMENT_VARIABLES.md) for all options.

## Documentation

Full docs in the [docs](docs/) folder. Start with [MCP](docs/MCP.md) for agents,
[Architecture](docs/ARCHITECTURE.md) for the code. The live tool catalogue is at
[/tools](https://micro.mu/tools).

## License

[AGPL-3.0](LICENSE) — use, modify, distribute. If you run a modified version as a
service, share the source.
