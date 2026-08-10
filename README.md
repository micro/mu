# mu

**Tools for agents.** News, mail, search, weather, markets, video, places,
files, contacts, calendar and a database, as tools an agent can call over MCP
and REST — behind one account instead of one per provider.

The same services render as a web app: a home screen with a card per service
and the agent inline.

Use it live at [micro.mu](https://micro.mu), or run your own.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu --serve
```

Open **http://localhost:8080**. The first account you create is the admin.

It runs with no configuration. On a fresh install, with no keys, these
answer: **news, markets, weather, prayer times, quran, mail, files, contacts,
calendar, tasks, the database, and search over your own content.** Weather
comes from Open-Meteo; places and geocoding from OpenStreetMap. No account, no
card, no signup.

Three things need a key, because there is no free provider worth wiring in:

| For | Set | Notes |
|---|---|---|
| The agent, and anything that composes an answer | `ANTHROPIC_API_KEY`, `ATLAS_API_KEY`, or `OPENAI_BASE_URL` | free if you run Ollama locally |
| Web search | `BRAVE_API_KEY` | Brave has a free tier |
| Video | `YOUTUBE_API_KEY` | free quota |

Without them those tools say so plainly rather than failing oddly. Everything
else keeps working.

```bash
mu setup        # pick an AI provider, paste a key
mu --serve
```

Everything else — mail and DKIM, Google sign-in, Stripe, x402 — is optional,
and configurable from `/admin/env` once you are signed in as admin.

Other ways to run it:

```bash
# Docker
git clone https://github.com/micro/mu && cd mu
docker compose up

# From source
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```

See the [installation guide](docs/INSTALLATION.md).

## Connect an agent

**Cursor, and clients with a config file.** Create a token at
[/token](https://micro.mu/token):

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

**Anything else.** It is JSON-RPC over HTTP POST:

```bash
curl -X POST https://micro.mu/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Scope the connection to the services you need:

```
https://micro.mu/mcp?tools=news,web,mail
```

That is what gets listed to your agent. Everything else is still callable.

[micro.mu/tools](https://micro.mu/tools) lists every tool with what each call
costs. See [MCP docs](docs/MCP.md) for the protocol.

## The tools

Here are the tools

| Service | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_read` · `apps_run` · `apps_search` · `apps_test` — build and run small web tools |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Chat** | `chat_rooms` · `chat_messages` · `chat_send` — the live discussion rooms attached to an item, and saying something in one |
| **Contacts** | `contacts_add` · `contacts_find` · `contacts_list` · `contacts_delete` — turn a name into an address |
| **Database** | `db_create` · `db_get` · `db_list` · `db_delete` — named collections of your own records, private by default, that outlive a conversation (`db_create` with an `id` overwrites). Apps get their own separate store through `mu.db` |
| **Events** | `events_create` · `events_list` · `events_delete` · `events_free` — schedule, cancel, and find when you are free, counting the Google Calendar you already keep |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` · `files_delete` — keep a file, get a URL |
| **Images** | `images_generate` · `images_search` |
| **Index** | `index_search` — everything this instance holds for you |
| **Mail** | `mail_inbox` · `mail_send` · `mail_search` · `mail_address` — a real SMTP server with DKIM, and an address per agent. Write to `you+name@` and that agent answers in the thread |
| **Markets** | `markets_list` — stocks, crypto, futures, commodities, currencies |
| **Memory** | `memory_set` · `memory_list` · `memory_delete` — what an agent keeps about you between conversations |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Places** | `places_search` · `places_nearby` · `places_eta` · `places_geocode` — points of interest, geocoding, travel time |
| **Prayer** | `prayer_times` · `prayer_qibla` · `prayer_verse` · `prayer_saying` · `prayer_reflection` — Islamic prayer times, qibla, and a daily verse, saying and name |
| **Search** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Stream** | `stream_list` · `stream_post` — this instance's own timeline |
| **Tasks** | `tasks_create` · `tasks_list` · `tasks_next` · `tasks_update` · `tasks_delete` — what is to be done, and work you can hand to the agent |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Wallet** | `wallet_balance` · `wallet_check` — credits, which is what calls are charged in |
| **Weather** | `weather_forecast` — conditions, forecast, pollen |
| **Platform** | `agent_ask` · `agent_list` — ask the whole thing a question and let it compose, or ask one of your own agents by name. Also `quran_search`, `content_save` · `content_unsave` · `saved_list`, and `content_flag` · `content_hide` · `block_user` · `unblock_user` |

## Request a tool

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) and say what it should do.

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

## The app

The server includes a web app. A home screen renders each service at a glance —
headlines, prices, weather, unread mail — and the agent sits inline to act on
what you are looking at. Apps run sandboxed, in an opaque origin, and reach the
platform through a fixed set of operations rather than your session.

Sign in with a username and password, a passkey (WebAuthn), or Google.

## Configuration

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
