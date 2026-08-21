# mu

A personal agent.

## Overview

An agent with an address. Chat to it in a browser or write to it by email, and
it can reach 100+ tools: news, mail, search, weather, markets, video, places,
files, contacts, calendar, documents. Work in progress.

One binary, four ways in:

| | |
|---|---|
| **Web app** | sign in and use it |
| **Email** | write to your address; it answers in the thread |
| **Mail client** | IMAP and SMTP, so the mailbox opens in Thunderbird or your phone |
| **MCP and the CLI** | for programs — the same tools over a protocol or a shell |

`mu agent` is a fifth thing and the one people mix up: the binary acting as a
client of somebody else's instance rather than serving one. See
[Agent](#agent-bring-your-own-model).

**SMTP in, IMAP out, HTTP for the app, MCP for agents, x402 for payments.** No
protocol here is ours. A billion people can already reach an instance from the
mail client they have open, without installing anything or agreeing to
anything, because the address is a real address and the standards behind it are
forty years old.

## Tools

| Service | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_read` · `apps_run` · `apps_search` · `apps_test` — build and run small web tools |
| **Archive** | `archive_search` · `archive_list` — everything this instance has collected, across news, video, markets and posts at once. Use it when the question crosses a service, or when you do not know which one would hold the answer |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Chat** | `chat_rooms` · `chat_messages` · `chat_send` — the live discussion rooms attached to an item, and saying something in one |
| **Contacts** | `contacts_add` · `contacts_find` · `contacts_list` · `contacts_delete` — turn a name into an address |
| **Docs** | `docs_write` · `docs_read` · `docs_list` · `docs_delete` — your own documents: a title and a markdown body, private by default. `docs_write` with an `id` replaces one. For something short to remember, use notes; apps persist through `mu.db`, which is a record store rather than documents |
| **Events** | `events_create` · `events_list` · `events_delete` · `events_free` — schedule, cancel, and find when you are free, counting the Google Calendar you already keep |
| **Files** | `files_put` · `files_get` · `files_list` · `files_share` · `files_delete` — keep a file, get a URL |
| **Flights** | `flights_overhead` · `flights_track` · `flights_airport` — where aircraft are, live from the positions they broadcast themselves. No schedule behind it, so it says where an aeroplane is and never why it is late |
| **Food** | `food_product` · `food_search` · `food_hygiene` — what is in a packet and whether the kitchen is clean. A barcode gives ingredients, allergens, nutrition per 100g and how processed it is, from Open Food Facts; `food_hygiene` gives the Food Standards Agency's inspection rating for any UK business. Where the data is silent it says so, because an absence of allergen information is not an absence of allergens. Needs no key |
| **Hazards** | `hazards_quakes` · `hazards_alerts` · `hazards_floods` — recent earthquakes worldwide from the USGS, with magnitude, place, how long ago and any tsunami warning, and current disasters from GDACS: cyclones, floods, volcanoes and wildfires, green through red. And flood warnings in force in England from the Environment Agency — the one hazard here that is a forecast rather than a record, since a warning says flooding is expected. Pass a lat/lon to ask about somewhere in particular. Needs no key for any of them |
| **Images** | `images_generate` · `images_search` |
| **Mail** | `mail_inbox` · `mail_send` · `mail_search` · `mail_info` — private messages, and an inbox each of your agents can be reached at. `mail_send` writes as you: a username stays here, a full address leaves so a reply comes back. Write to `you+name@` and that agent answers in the thread |
| **Markets** | `markets_list` · `markets_convert` — stocks, crypto, futures, commodities, currencies, and conversion between them. `markets_convert` takes a past date back to 1999 and converts crypto at the live price through the dollar |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Notes** | `notes_add` · `notes_get` · `notes_list` · `notes_delete` — a title and what is under it, kept between conversations and read back into every one |
| **Places** | `places_search` · `places_nearby` · `places_geocode` · `places_address` · `places_elevation` — points of interest, geocoding both directions, height above sea level |
| **Prayer** | `prayer_times` · `prayer_qibla` · `prayer_reflection` · `prayer_verse` · `prayer_saying` · `prayer_search` — Islamic prayer times, qibla, a daily verse and saying, and the sources by reference or by question |
| **Recall** | `recall_search` · `recall_conversation` · `recall_list` — everything you have ever said to an agent and been told, on any client: search it, and read a conversation back |
| **Routes** | `routes_eta` · `routes_directions` · `routes_nearest` — travel time with traffic, turn-by-turn, and which of several places is quickest to reach |
| **Search** | `web_search` · `web_fetch` — search the web, read a page as clean text |
| **SMS** | `sms_send` · `sms_history` · `sms_number` · `sms_verify` — text somebody and read what they text back, from a real number. Priced per segment, capped per day, and STOP is honoured |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Stream** | `stream_list` · `stream_post` — this instance's own timeline |
| **Tasks** | `tasks_create` · `tasks_list` · `tasks_next` · `tasks_update` · `tasks_delete` — what is to be done, and work you can hand to the agent |
| **Text** | `text_summarise` · `text_extract` · `text_classify` · `text_translate` — language work at a fixed price per call: shorten it, turn it into JSON matching a schema you give, sort it into one of your labels, or put it in another language. Capped at 30,000 characters, and priced because each one is a model call we pay for |
| **Tiles** | `tiles_tile` · `tiles_area` — Ordnance Survey map tiles for Britain, as URLs a map library takes directly: road, outdoor (rights of way and contours) and light. Ask for one tile or for every tile covering a bounding box. Free — a tile is fetched once, ever, and served from here afterwards, because a tile does not change |
| **Transit** | `transit_nearby` · `transit_arrivals` · `transit_status` · `transit_feeds` · `transit_trains` · `transit_buses` — stops near a point, what is due at one, and which lines are delayed or suspended. London is live from TfL, down to how many minutes away the bus is. Anywhere else answers from the agency's published timetable, using the same two tools and saying which kind of answer it gave — set `TRANSIT_FEEDS` to load one, and `transit_feeds` lists which are worth loading and what each costs. Needs no key either way. `transit_trains` is the live board at any British station from National Rail, and `transit_buses` is where the buses actually are near a point, from the DfT's Bus Open Data Service — the two that make this live outside London |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Wallet** | `wallet_address` · `wallet_balance` · `wallet_list` · `wallet_pay` — a key of your own on Base: an address that holds USDC, and paying for a tool on another x402 server with it. Capped per call and per day |
| **Weather** | `weather_forecast` · `weather_air` · `weather_marine` · `weather_history` — conditions and the days ahead; air quality, pollutants, UV and pollen; wave height, period and direction at a coastal point; and what the weather actually was between two dates. Everything but the forecast is keyless |

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) to request a tool.

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
and configurable from `/admin/config` once you are signed in as admin.

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

## Connect via MCP

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

See [micro.mu/tools](https://micro.mu/tools) for all the tools.

## App

The server includes a web app. A home screen renders each service at a glance —
headlines, prices, weather, unread mail — and the agent sits inline to act on
what you are looking at. Apps run sandboxed, in an opaque origin, and reach the
platform through a fixed set of operations rather than your session.

Sign in with a username and password, a passkey (WebAuthn), or Google.

## Email

The server is an MTA: it listens for SMTP, delivers outbound to the recipient's
MX signed with DKIM, and filters what arrives. An account is an address.

```
asim@micro.mu             your address; mail here reaches your agent
asim+research@micro.mu    your agent named "research"
agent@micro.mu            the instance's shared agent
```

The part after the `+` picks which agent answers. Replies thread — `In-Reply-To`
and `References` are set from the chain — and everything that arrives is
readable at `/inbox`.

**Nothing arrives from a stranger.** Inbound is refused with a 550 unless it is
a reply to something we sent, we have written to that address before, the
sender's domain is whitelisted, or the sender is verified on an account here.
The policy is at the top of
[`service/mail/inbound_filter.go`](service/mail/inbound_filter.go).

**IMAP and SMTP submission** mean the mailbox opens in Thunderbird, Mail.app or
your phone, and you can reply from there. Username is your account name,
password is an access token from `/token`. Ports, TLS and a worked nginx config
are in [Install](docs/INSTALL.md#reading-your-mail-in-a-mail-client);
[examples/imap-client](examples/imap-client) is a working client.

Self-hosting needs `MAIL_DOMAIN`, an MX record and inbound SMTP. See
[Install](docs/INSTALL.md).

## CLI

Every tool is a `mu` subcommand. The same binary runs the server (`mu --serve`)
and the CLI.

```bash
mu news list                            # latest headlines
mu news search "ai safety"              # search news
mu web search "claude code"             # search the web
mu markets list --category stocks       # live prices
mu weather forecast --lat 51.5 --lon -0.12
mu docs list --collection notes         # your own documents
mu x402                                 # paying per call: config, and your key
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

These are tool calls, not agent runs: one command, one tool, no model involved.
`mu agent` is not among them — it is the mode below, and it needs a model key
and a wallet rather than a token.

## Agent (bring your own model)

`mu agent` is the binary as a *client* of an instance, not a caller of one you
are signed into. It holds your model key and a private key, reads the tool
catalogue from a running instance, and pays per call over x402 — no account on
that instance and no signup.

The distinction that matters: `mu news list` uses your account on an instance,
`mu agent` uses your wallet on somebody else's. They share a binary and nothing
else.

```bash
# 1. A model. The tools are rented; the thinking is yours.
export ANTHROPIC_API_KEY=sk-ant-...     # or OPENROUTER_API_KEY
                                        # or OPENAI_BASE_URL for Ollama etc.

# 2. A wallet. Created for you on first run, or make it yourself:
mu x402 key new                         # prints an address; send USDC on Base
                                        # to it. No ETH — you never pay gas.

# 3. Ask.
mu agent                                # a conversation
mu agent "what happened in markets today?"
```

```
model: anthropic/claude-sonnet-4-6
113 tools from https://micro.mu
wallet: 0x4160a863… (1.27 USDC)

> what are the top news headlines today?
· news_list
…
> of those, which matters most for markets?
… answered with no tool call, and no charge
```

Reading the catalogue is free, so it works before the wallet holds anything —
only priced tools need funds. What a run spent is read back off the chain when
it ends, not totted up from what the agent believes it authorised.

`--server` points it at any x402 instance; a name from `X402_SERVERS` works too.
`--seed` uses a different key.

## Credits & Payments

A person tops up by card and spends one credit balance. A credit is 1p.

A credit is charged when a call costs this instance money: a model call, or a
third party billed per request. Everything else is 0 — the agent itself, your
inbox, your files, and reading or sending mail from a mail client. Prices are
[`quota.json`](quota.json), which is the one place they are set.

The exception is mail addressed outside the instance. It is priced and capped
per day, and not because it costs anything to send: what a loop spends there is
the sending domain's reputation, and no balance repairs that. A price stops
somebody who has to pay; the cap is what stops a loop.

**An agent can pay with USDC over [x402](https://x402.org) and never sign
up.** A priced call with no credentials answers `402 Payment Required` naming
the price and where to send it. The payment is the identity.

To write an agent that pays, see
[examples/x402-agent](examples/x402-agent) — a standalone module that imports
none of this. It uses the [x402
Foundation](https://github.com/x402-foundation/x402) SDK, so the same file pays
any x402 server.

To watch it work, `mu` is its own client too. Put a funded Base wallet's key in
`~/.mu/keys/wallet.seed`:

```bash
mu x402 call web_search query="x402"   # 402 → signs → pays → returns the result
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

## License

AGPL 3.0
