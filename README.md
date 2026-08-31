# mu

A home for agents, tools and services

## Overview

We're building services that operate as the building blocks for agents. Mail, chat, news, video, search, etc. Then we archive any data locally so it's all searchable. Services and the archive become tools for agents to use. 

Using agents has also been a very synchronous experience. Communication here is async with everything going to one inbox, 
whether it's email, chat or sms, etc. Use it from the web, your phone, the command line, email, anywhere.

## Features

What's included

- **Services** - 30+ real world services including news, mail, markets, video, etc accessible via API, CLI or the Web
- **Agents** - Define an agent by name, prompt and give it specific tools to use, then chat with it on the web, mail or xmpp
- **Inbox** - A single place to keep track of threads across channels. Use it on the web or via IMAP in an email client.

## Services

The services available via the web, API or as tools, reachable via MCP, and command line 

| Service | Tools |
|---|---|
| **Apps** | `apps_build` · `apps_create` · `apps_edit` · `apps_fork` · `apps_embed` · `apps_read` · `apps_search` · `apps_test` — build small web tools and put them anywhere |
| **Archive** | `archive_search` · `archive_list` — everything this instance has collected, across news, video, markets and posts at once. Use it when the question crosses a service, or when you do not know which one would hold the answer |
| **Blog** | `blog_create` · `blog_read` · `blog_list` · `blog_update` · `blog_delete` — publish, with AI-generated daily digests |
| **Browser** | `browser_read` · `browser_shot` — a real browser for the pages a plain fetch cannot read: open one and get its text after its JavaScript has run, or photograph it and get a URL for the picture. Costs, because it runs a Chromium; a plain fetch is free and is the right first try |
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
| **Maps** | `maps_tile` · `maps_area` — a map of Britain you can move around, and the Ordnance Survey tiles under it, as URLs a map library takes directly: road, outdoor (rights of way and contours) and light. Ask for one tile or for every tile covering a bounding box. Free — a tile is fetched once, ever, and served from here afterwards, because a tile does not change |
| **Markets** | `markets_list` · `markets_convert` — stocks, crypto, futures, commodities, currencies, and conversion between them. `markets_convert` takes a past date back to 1999 and converts crypto at the live price through the dollar |
| **News** | `news_list` · `news_read` · `news_search` — RSS aggregation, full articles |
| **Notes** | `notes_add` · `notes_get` · `notes_list` · `notes_delete` — a title and what is under it, kept between conversations and read back into every one |
| **Notify** | `notify_send` · `notify_devices` — reach yourself on a phone or a laptop with the page closed. Your own devices only: there is no recipient argument, so it cannot be pointed at anybody else |
| **Places** | `places_search` · `places_nearby` · `places_geocode` · `places_address` · `places_elevation` — points of interest, geocoding both directions, height above sea level |
| **Prayer** | `prayer_times` · `prayer_qibla` · `prayer_reflection` · `prayer_verse` · `prayer_saying` · `prayer_search` — Islamic prayer times, qibla, a daily verse and saying, and the sources by reference or by question |
| **Recall** | `recall_search` · `recall_conversation` · `recall_list` — everything you have ever said to an agent and been told, on any client: search it, and read a conversation back |
| **Routes** | `routes_eta` · `routes_directions` · `routes_nearest` — travel time with traffic, turn-by-turn, and which of several places is quickest to reach |
| **Shell** | `shell_run` · `shell_write` — a machine of your own: a Debian container with bash, and a `/work` directory that keeps what you put in it between calls, along with the directory you were last in. Read, list, search and edit with the commands you already know; `shell_write` is only for putting a new file down, which is the one thing a heredoc mangles. Build things, run tests, clone a repo, move files about. Running a command costs, because it is CPU and memory here. Needs Docker on the instance |
| **SMS** | `sms_send` · `sms_history` · `sms_number` · `sms_verify` — text somebody and read what they text back, from a real number. Priced per segment, capped per day, and STOP is honoured |
| **Social** | `social_list` · `social_search` — public threads and replies |
| **Stream** | `stream_list` — what has been happening here |
| **Tasks** | `tasks_create` · `tasks_list` · `tasks_next` · `tasks_update` · `tasks_delete` — what is to be done, and work you can hand to the agent |
| **Text** | `text_summarise` · `text_extract` · `text_classify` · `text_translate` — language work at a fixed price per call: shorten it, turn it into JSON matching a schema you give, sort it into one of your labels, or put it in another language. Capped at 30,000 characters, and priced because each one is a model call we pay for |
| **Transit** | `transit_nearby` · `transit_arrivals` · `transit_status` · `transit_feeds` · `transit_trains` · `transit_buses` — stops near a point, what is due at one, and which lines are delayed or suspended. London is live from TfL, down to how many minutes away the bus is. Anywhere else answers from the agency's published timetable, using the same two tools and saying which kind of answer it gave — set `TRANSIT_FEEDS` to load one, and `transit_feeds` lists which are worth loading and what each costs. Needs no key either way. `transit_trains` is the live board at any British station from National Rail, and `transit_buses` is where the buses actually are near a point, from the DfT's Bus Open Data Service — the two that make this live outside London |
| **Users** | `users_list` · `users_find` · `users_get` — who is on this instance, the people and the agents, and whether they are here now. Turns a name somebody mentioned into an address you can write to. Needs an account: each profile is public, but being able to enumerate them is what makes a directory worth scraping |
| **Video** | `video_list` · `video_search` — curated channels, no ads or recommendations |
| **Wallet** | `wallet_address` · `wallet_balance` — a key of your own on Base: an address that holds USDC, and what it holds. Paying a priced endpoint is what a client does when it gets a 402, not a tool it calls |
| **Weather** | `weather_forecast` · `weather_air` · `weather_marine` · `weather_history` — conditions and the days ahead; air quality, pollutants, UV and pollen; wave height, period and direction at a coastal point; and what the weather actually was between two dates. Everything but the forecast is keyless |
| **Web** | `web_search` · `web_fetch` — search the web, read a page as clean text |

[Open an issue](https://github.com/micro/mu/issues/new?labels=enhancement&title=Tool%20request%3A%20&body=What%20should%20it%20do%3F%0A%0AWhat%20would%20you%20use%20it%20for%3F%0A) to request a service.

## Agents

An agent is a name, a prompt, and which of the services above it may reach. No
code and no deployment — the services are the same ones in the table, so
defining an agent is choosing a subset and saying what it is for.

Two come built in:

- **Micro** — general. Every service, for the questions that cross several of
  them: what happened today, what is moving, what is in my mail.
- **Code** — specific. A machine of its own and somewhere to put what it makes:
  `shell` and `apps` only. Describe an app and it writes it, runs it and hosts
  it.

Make your own at `/agents`: a name, a standing instruction, and the services it
is allowed. Leaving the services empty gives it everything you can reach, which
is a choice rather than the default.

Then talk to it wherever you already are:

```bash
mu ask "what is in my inbox?"                    # the default agent
mu ask --agent research "anything new this week?"
```

On the web it is `/agent/<name>`. By mail, write to `you+name@` and that agent
answers in the thread. Each keeps its own notes, so what it learns about your
projects does not end up in the pool another agent answers from.

Agents run on the instance, using its model, so nothing needs a key of your
own. `mu agent` is the other direction — see [CLI](#cli).

## Inbox

One place for what arrived, whichever channel carried it — mail, chat, sms, and
the conversations you have had with your agents. It is a view over the record
every client writes to, so a question you asked from the terminal this morning
sits next to an email that came in overnight, in the same list.

It is not a second mailbox. `service/mail` is the mail server; `/inbox` is
where things turn up. Reply to a thread and the agent answers in it.

Connect a normal mail client over IMAP and read the same threads from your
phone.

## Tools

If you want to use the tools with an existing agent.

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

## Install

Quick install guide for self hosting (let us know if it's broken).

```bash
curl -fsSL https://raw.githubusercontent.com/micro/mu/main/install.sh | sh
mu --serve
```

Open **http://localhost:8080**. The first account you create is the admin.

Quite a few things need API keys, but here's some must haves.


| For | Set | Notes |
|---|---|---|
| AI models | `ANTHROPIC_API_KEY`, `ATLASCLOUD_API_KEY`, `GEMINI_API_KEY`, `OPENROUTER_API_KEY`, or `OPENAI_BASE_URL` | free if you run Ollama locally |
| Web search | `BRAVE_API_KEY` | Brave has a free tier |
| Video | `YOUTUBE_API_KEY` | free quota |

Follow setup in CLI

```bash
mu setup        # pick an AI provider, paste a key
mu --serve
```

Everything else — mail, Google sign-in, Stripe for payments, etc — is optional.

The binary is a client, and by default it calls **https://micro.mu** —
the instance this project runs live. Running your own? Point it there:

```bash
mu login https://your.host   # saves the address and a token
mu config get                # says which instance is in use, and why
```

Without that, `mu news list` on the machine you just installed calls the
hosted instance rather than the one you are running. `MU_URL` and `--url`
override per shell and per command.

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

These are tool calls: one command, one tool, no model involved and nothing to
pay for unless the tool itself costs.

To talk to your agent instead, `mu ask` — it runs on the instance, so it needs
your token and no model key of your own:

```bash
mu ask "what is in my inbox?"
mu ask --agent research "anything new this week?"
```

`mu agent` is the other direction and easy to reach for by mistake: it runs the
agent *here*, on your machine, with your own model key, renting tools from an
instance over x402 and paying per call. Same word in English, opposite ways
round.

## API

Every tool is also an HTTP endpoint, at `/api/v1/<service>/<method>`. The same
catalogue the CLI and the agent read, so a tool added to the server is an
endpoint without anybody writing a route.

```bash
curl https://micro.mu/api/v1/                      # the catalogue
curl "https://micro.mu/api/v1/news/list?limit=5"   # arguments in the query
curl -X POST https://micro.mu/api/v1/news/list \
  -H 'Content-Type: application/json' -d '{"limit":5}'
```

GET and POST mean the same thing and both work, because a REST API where reads
are GETs is what every client library expects. What is not offered is a GET
that changes something: a method the catalogue marks as writing needs a POST,
so a link, a prefetch or a crawler cannot fire one.

Authenticate with a token from `/token` as `Authorization: Bearer`, or with an
OAuth client. A priced endpoint answers 402 without one, which an x402 client
pays per call with no account at all.

MCP clients connect at `/mcp` — same tools, same auth, same prices. The
reference is at `/api`, and each service has its own page at `/services/<name>`.

## Web

The same instance is a web app, and every page on it is a service answering.

- `/home` — what arrived, who is working on it, and what this instance knows
  right now, with a box at the top to ask.
- `/inbox` — the threads, whichever channel carried them.
- `/agents` — your roster, and where you make a new one.
- `/services` — the catalogue, and a page for each: `/news`, `/weather`,
  `/markets`, and the rest.

The service pages are not a separate product built over the API. They make the
same calls an agent makes and render the result, which is why anything you can
ask for you can also go and look at.

## Configuration

What a call costs is data, not code: `quota.json` at the top of the repo is the
one price list, and everything that charges or displays a price reads it. Drop a
`quota.json` into the data directory to override any entry without rebuilding.

Some files are embedded in the binary, so editing means rebuilding:

- `service/news/feeds.json` — RSS news feeds
- `service/chat/prompts.json` — chat topics
- `home/cards.json` — home screen cards
- `service/video/channels.json` — YouTube channels
- `service/places/locations.json` — saved locations

See [Install](docs/INSTALL.md) for every setting the code reads.

The rest lives in /admin/config on the server.

## License

AGPL 3.0
