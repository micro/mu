# mu

Tools for agents

## Overview

Mu is an MCP server and web app for agents and humans. It provides access to the real world through MCP for the agents 
and let's humans browse or interact with same content in a web app. Access news, web search, mail,
markets, weather, video, places, images, files, events, contacts, and a database
your agents can keep records in.

Use it live at [micro.mu](https://micro.mu), or self-host.

## Usage

Two ways in, depending on what your client reads. Both need an account — the
same one you sign into the app with.

**Cursor, and clients with a config file.** Create a token at
[/token](https://micro.mu/token) and add this to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp",
      "headers": {
        "Authorization": "Bearer ${env:MU_TOKEN}"
      }
    }
  }
}
```

**Claude Desktop.** Settings → Connectors → Add custom connector, and paste
`https://micro.mu/mcp`. It registers itself, opens a browser and asks you to
sign in — no token needed. Pasting the URL into `claude_desktop_config.json`
will not work: that file only takes local command-line servers.

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

One row per service, named the way [/tools](https://micro.mu/tools) and the
sidebar name it, alphabetical within each group.

The groups are the point. A flat list of twenty-three services claims they all
matter the same amount, and they do not — the first group is what an agent
should read before it fetches anything, and the rest is fetching.

### What it already knows

Ask these first. Everything below this is a call out to something; this is what
is already true about the person asking.

| Area | Tools |
|---|---|
| **Context** | `context_get` — what the caller watches, live, and what has been remembered about them, in one call. Cheaper than guessing which of the others to try |
| **Memory** | `memory_set` · `memory_list` · `memory_delete` — durable facts about the caller, read back into every question they ask |

### What is yours

Your own things. Private by default, and closed entirely to a caller with no
account.

| Area | Tools |
|---|---|
| **Contacts** | `contacts_add` · `contacts_find` · `contacts_list` · `contacts_delete` — turns a name into an address, which is what "email Sam" needs before any mail can be sent |
| **Database** | `db_create` · `db_get` · `db_list` · `db_delete` — named collections of your own records, private by default, that outlive a conversation (`db_create` with an `id` overwrites). Apps get their own separate store through `mu.db` |
| **Events** | `events_create` · `events_list` · `events_delete` · `events_free` — schedule, cancel, and find when you are free, counting the Google Calendar you already keep |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` · `files_delete` — keep a file, get a URL |
| **Index** | `index_search` — everything this instance holds for you |
| **Mail** | `mail_inbox` · `mail_send` · `mail_search` · `mail_address` — a real SMTP server with DKIM, and an address per agent |
| **Tasks** | `tasks_create` · `tasks_list` · `tasks_next` · `tasks_update` · `tasks_delete` — what is to be done, and work you can hand to the agent |
| **Wallet** | `wallet_balance` · `wallet_check` — credits, which is what calls are charged in |

### The world outside the model

What an agent cannot know and has to go and get.

| Area | Tools |
|---|---|
| **Images** | `images_generate` · `images_search` |
| **Markets** | `markets_list` — stocks, crypto, futures, commodities, currencies |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Places** | `places_search` · `places_nearby` · `places_eta` · `places_geocode` — points of interest, geocoding, travel time |
| **Prayer** | `prayer_times` · `prayer_qibla` · `prayer_verse` · `prayer_saying` · `prayer_reflection` — Islamic prayer times, qibla, and a daily verse, saying and name |
| **Search** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Weather** | `weather_forecast` — conditions, forecast, pollen |

### What runs here

This instance hosts these rather than fetching them, which is the difference
between a tool and a wrapper.

| Area | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_read` · `apps_run` · `apps_search` · `apps_test` — build and run small web tools |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Chat** | `chat_rooms` · `chat_messages` — the live discussion rooms attached to an item |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Stream** | `stream_list` · `stream_post` — this instance's own timeline |
| **Platform** | `agent_ask` — ask the whole thing a question and let it compose. Also `quran_search`, `content_save` · `content_unsave` · `saved_list`, and `content_flag` · `content_hide` · `block_user` · `unblock_user` |

## Request a tool

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) and say what it should do.

## The app

The server includes a web app with a home screen. Cards render each service at a glance (headlines,
prices, weather, unread mail) and the agent sits inline to act on what you're
looking at.

An LLM — Claude, Atlas Cloud (DeepSeek), or a local Ollama / OpenAI-compatible
endpoint — calls the services as tools, composes answers, and keeps per-user
memory across sessions.

Sign in with a username and password, a **passkey** (WebAuthn), or **Google**.
For the CLI, generate a Personal Access Token at `/token`.

## CLI

Every tool is a `mu` subcommand. The same binary runs the server (`mu --serve`)
and the CLI.

```bash
mu news list                            # latest headlines
mu news search "ai safety"              # search news
mu web search "claude code"             # search the web
mu markets list --category stocks       # live prices
mu agent "what is the btc price?"       # run the full agent
mu weather forecast --lat 51.5 --lon -0.12
mu db list --collection notes           # your own records
mu wallet                               # your balance
mu help                                 # full tool list
```

Every tool in the table above is a command: the service, then the method. The
underscore form works too, so `mu news list` and `mu news_list` are the same
call.

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

Every tool is a command on both, in the same two-word shape as the CLI:
`/news list`, `/markets list category:stocks`, `/prayer times`. Discord gets one
slash command per service with the methods as subcommands; on Telegram you type
`/news list`. Both also take `/agent <question>` for anything that needs
composing, and `/usage` for your own stats.

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
