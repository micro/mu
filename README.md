# mu

**Tools for agents.** News, mail, search, weather, markets, video, places,
files, contacts, calendar and your own documents, as tools an agent can use via one MCP
server and token — also includes a web app for humans.

## Hosting

Use it live at [micro.mu](https://micro.mu), or run your own.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu --serve
```

Open **http://localhost:8080**. The first account you create is the admin.

It runs with no configuration. A few things need an API key.


| For | Set | Notes |
|---|---|---|
| AI features | `ANTHROPIC_API_KEY`, `ATLAS_API_KEY`, `OPENROUTER_API_KEY`, or `OPENAI_BASE_URL` | free if you run Ollama locally |
| Web search | `BRAVE_API_KEY` | Brave has a free tier |
| Video | `YOUTUBE_API_KEY` | free quota |

Follow setup in CLI

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

See the [installation guide](docs/INSTALL.md).

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

See [micro.mu/tools](https://micro.mu/tools) for all the tools. See [Help](docs/HELP.md) for the protocol.

## The tools

Here are the tools

| Service | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_read` · `apps_run` · `apps_search` · `apps_test` — build and run small web tools |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Chat** | `chat_rooms` · `chat_messages` · `chat_send` — the live discussion rooms attached to an item, and saying something in one |
| **Contacts** | `contacts_add` · `contacts_find` · `contacts_list` · `contacts_delete` — turn a name into an address |
| **Docs** | `docs_write` · `docs_read` · `docs_list` · `docs_delete` — your own documents: a title and a markdown body, private by default. `docs_write` with an `id` replaces one. For something short to remember, use notes; apps persist through `mu.db`, which is a record store rather than documents |
| **Email** | `email_send` · `email_history` · `email_sender` · `email_verify` — email people outside, from a sending domain of its own. Capped per day, and the history says delivered or bounced rather than only accepted. `email_verify` is the signup code check, under your product's name |
| **Events** | `events_create` · `events_list` · `events_delete` · `events_free` — schedule, cancel, and find when you are free, counting the Google Calendar you already keep |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` · `files_delete` — keep a file, get a URL |
| **Flights** | `flights_overhead` · `flights_track` · `flights_airport` — where aircraft are, live from the positions they broadcast themselves. No schedule behind it, so it says where an aeroplane is and never why it is late |
| **Images** | `images_generate` · `images_search` |
| **Mail** | `mail_inbox` · `mail_send` · `mail_search` · `mail_info` — private messages, and an inbox each of your agents can be reached at. `mail_send` writes as you: a username stays here, a full address leaves so a reply comes back. Write to `you+name@` and that agent answers in the thread |
| **Markets** | `markets_list` — stocks, crypto, futures, commodities, currencies |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Notes** | `notes_add` · `notes_get` · `notes_list` · `notes_delete` — a title and what is under it, kept between conversations and read back into every one |
| **Places** | `places_search` · `places_nearby` · `places_geocode` · `places_address` · `places_elevation` — points of interest, geocoding both directions, height above sea level |
| **Prayer** | `prayer_times` · `prayer_qibla` · `prayer_reflection` · `prayer_verse` · `prayer_saying` · `prayer_search` — Islamic prayer times, qibla, a daily verse and saying, and the sources by reference or by question |
| **Routes** | `routes_eta` · `routes_directions` · `routes_nearest` — travel time with traffic, turn-by-turn, and which of several places is quickest to reach |
| **Search** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **SMS** | `sms_send` · `sms_history` · `sms_number` · `sms_verify` — text somebody and read what they text back, from a real number. Priced per segment, capped per day, and STOP is honoured |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Stream** | `stream_list` · `stream_post` — this instance's own timeline |
| **Tasks** | `tasks_create` · `tasks_list` · `tasks_next` · `tasks_update` · `tasks_delete` — what is to be done, and work you can hand to the agent |
| **User** | `user_saved` · `user_save` · `user_unsave` · `user_hide` · `user_flag` · `user_block` · `user_unblock` — what you do about other people's posts: keep one, stop seeing one, report one, or stop hearing from an account |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Weather** | `weather_forecast` — conditions, forecast, pollen |
| **WhatsApp** | `whatsapp_send` · `whatsapp_history` · `whatsapp_open` — reply to people on WhatsApp. Only within 24 hours of their message, which is WhatsApp's rule and not ours: `whatsapp_open` says who can be written to and until when |

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) to request a tool.

## The app

The server includes a web app. A home screen renders each service at a glance —
headlines, prices, weather, unread mail — and the agent sits inline to act on
what you are looking at. Apps run sandboxed, in an opaque origin, and reach the
platform through a fixed set of operations rather than your session.

Sign in with a username and password, a passkey (WebAuthn), or Google.

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
mu docs list --collection notes         # your own documents
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

Run `mu --help` for the list — it reads the same catalogue the agent does.

## Discord & Telegram

Talk to the agent from Discord or Telegram — questions, markets, news, all from
chat. [Join the Discord](https://discord.gg/WeMU5AGxD)

Every tool is a command on both, in the same two-word shape as the CLI:
`/news list`, `/markets list category:stocks`, `/prayer times`. Discord gets one
slash command per service with the methods as subcommands; on Telegram you type
`/news list`. Both also take `/agent <question>` for anything that needs
composing, and `/usage` for your own stats.

Setup for both is in [Install](docs/INSTALL.md).

## Credits & Payments

A person tops up by card and spends one credit balance. A credit is 1p.

**An agent can pay with USDC over [x402](https://x402.org) and never sign
up.** A priced call with no credentials answers `402 Payment Required` naming
the price and where to send it. The payment is the identity.

`mu` is its own x402 client. Put a funded Base wallet's key in
`~/.mu/keys/wallet.seed`:

```bash
mu x402 call markets_list                 # free — no wallet, no account
mu x402 call web_search query="x402"      # priced — pays the 402, returns it
```

Self-host with neither Stripe nor x402 and nothing is metered: every tool is
free.

## Configuration

Customise feeds, prompts and cards by editing JSON files:

- `service/news/feeds.json` — RSS news feeds
- `service/chat/prompts.json` — chat topics
- `home/cards.json` — home screen cards
- `service/video/channels.json` — YouTube channels
- `service/places/locations.json` — saved locations

See [Install](docs/INSTALL.md) for every setting the code reads.

## Documentation

Three pages, and they are the site's: [About](https://micro.mu/about),
[Help](https://micro.mu/help) for connecting an agent, and
[Install](https://micro.mu/install) for running your own. The live tool
catalogue is at [/tools](https://micro.mu/tools) — it is generated from what
the instance runs, so it cannot go stale.

For the code, [Architecture](docs/ARCHITECTURE.md).

## License

AGPL 3.0
