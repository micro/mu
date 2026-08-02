# mu

Tools for agents

## Overview

Mu gives an agent the everyday internet as tools: news, web search, mail,
markets, weather, video, places, images, files, calendar, contacts. 67 of them, behind one
[MCP](https://modelcontextprotocol.io) endpoint. Connect once and your agent has
all of them, instead of wiring up a server for each.

**Real tools, not wrappers.** Most things offering an agent tools are a thin
layer over somebody else's API. Mu runs the mail server (SMTP with DKIM), the
feed aggregator, the search index, the app sandbox and the wallet it hands you.
`mail_inbox` reads a real inbox. `db_create` writes to real storage. The tools
aren't a catalogue of other people's products — they're this instance's own
capabilities, exposed.

There is also a web app on the same tools, because the operator is a person too.
Use it hosted at [micro.mu](https://micro.mu), or self-host the single Go binary
— same product either way. Open source, AGPL-3.0.

## Point an agent at it

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://micro.mu/mcp"
    }
  }
}
```

That's the whole setup. There is no key in that config because there is nothing
to paste: the first call gets a `401` pointing at the instance's authorization
server, your client walks you through sign-in and keeps the token itself. This is
the [MCP authorization](https://modelcontextprotocol.io/specification/basic/authorization)
spec — Claude Desktop and Cursor both speak it.

For a client that doesn't, create a Personal Access Token at
[/token](https://micro.mu/token) and send it as `Authorization: Bearer`.

```bash
curl -X POST https://micro.mu/mcp -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Reading this instance's own content is included. Calls that cost us money to run
— a model call, a paid third party — draw credits from your balance.

**Browse what's there:** [micro.mu/tools](https://micro.mu/tools) — every tool,
grouped by service, with what each call costs. [MCP docs](docs/MCP.md) for the
protocol detail.

## The tools

| Area | Tools |
|---|---|
| **Web** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Markets** | `markets_list` — crypto, futures, commodities, currencies |
| **Weather** | `weather_forecast` — conditions, forecast, pollen |
| **Places** | `places_search` · `places_nearby` · `places_eta` — points of interest, geocoding, travel time |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Mail** | `mail_inbox` · `mail_send` · `mail_address` — a real SMTP server with DKIM, and an address per agent |
| **Storage** | `db_create` · `db_get` · `db_list` · `db_update` · `db_delete` — per-caller records |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` — keep a file, get a URL |
| **Calendar** | `events_create` · `events_free` · `events_list` — schedule, and find when you are free |
| **Contacts** | `contacts_find` · `contacts_add` · `contacts_list` — turn a name into an address |
| **Search your own** | `index_search` — everything this instance holds for you |
| **Images** | `images_generate` · `images_search` |
| **Writing** | `blog_*` · `social_*` · `stream_*` — publish, read, discuss |
| **Apps** | `apps_build` · `apps_run` · `apps_edit` — build and run small web tools |
| **Faith** | `islam_today` · `islam_prayer` · `islam_qibla` · `quran` · `hadith` |
| **Money** | `wallet_balance` — credits, and where to send USDC to top up |
| **Agent** | `agent` · `chat` — ask the whole thing a question and let it compose |

Every tool is also a `mu` CLI subcommand. Account-scoped tools (mail, storage,
index, images, events, files, contacts) need a signed-in caller; `mail_send` always does, so an
unaccountable caller can't spend a domain's reputation.

## Why the tools are real

Each area above is a **service** — a Go package with typed handlers, registered
in-process behind a [Go Micro](https://github.com/micro/go-micro) registry. One
binary, no external infrastructure. The registry is the single source of truth:
a service declares itself once and every surface derives from that declaration.

```go
var Spec = service.Spec{
	Name:        "web",
	Handler:     new(Server),
	Description: "The open web: search it, read a page from it",
	Page:        "/search",
	Endpoints: map[string]service.Endpoint{
		"Search": {Doc: "Search the web for current information", Cost: wallet.OpWebSearch},
		"Fetch":  {Doc: "Fetch a web page and return readable content", Cost: wallet.OpWebFetch},
	},
}
```

Register that and it becomes available, in the same moment and with no extra
wiring, to:

- **agents**, over MCP at `/mcp` — for Claude Desktop, Cursor, or your own;
- **the built-in agent**, as a tool it can call;
- **custom agents**, in the tool picker at `/agent/new`;
- **the CLI**, where each tool is a subcommand;
- **apps**, through `mu.service(name, method, args)` in the SDK.

So extending Mu is adding an element, not wiring N integrations.

```js
// inside an app — any registered service, no SDK change needed
const { times } = await mu.service('islam', 'prayer', { lat, lon, tz })
const { summary } = await mu.service('weather', 'forecast', { lat, lon })
```

Identity is bound server-side from the call context — a caller never names whose
data it wants, and there is no account field in any request to forge.

## The app

The same tools, for a person. Cards render each service at a glance (headlines,
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

Run your own instance and you are the operator: the tools are yours, and anyone
paying to call them pays you.

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

Prefer the terminal? Configure the provider headless, then start the server:

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
