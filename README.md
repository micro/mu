# Mu

**Tools for agents.**

Mu is an open-source runtime for agents. It provides services, tools, memory and machine interfaces for agents to use.

**Micro** is the personal assistant that ships with Mu and the default human-facing front door.

## Overview

**Mu** is the definitive unit: the runtime, services, archive, inbox and agent system. Services operate as building blocks for agents — mail, chat, news, video, search, markets, weather and more. Data can be archived locally so it stays searchable and becomes contextual memory. Services and the archive become tools for Micro and any other agents you create.

**Micro** is the first agent and the one people meet first. It answers by default and can be reached from the web, email, SMS, WhatsApp or the CLI.

Using agents has become a fragmented experience. Mu unifies communication into one inbox, whether it's mail, chat, SMS, WhatsApp, notes, tasks or agent activity, while Micro gives that system a single human-facing assistant.

## Features

What's included

- **Micro** - The default personal assistant and the main front door to the system.
- **Clients** - Use Micro via Web, SMS, email, WhatsApp and more. Requires setup.
- **Services** - 30+ services including news, mail, markets and video, accessible via API, CLI or the Web.
- **Agents** - Micro answers by default; define your own by name, prompt and tools, then chat via the web.
- **Inbox** - A single place to keep track of chats, notes, tasks, etc. Assign tasks to agents or reply directly.

## Agents

Mu is the runtime. Micro is the first agent and the one people meet first.

- **Micro** is the default personal assistant. General purpose, with the services above as its tools, so it can answer from what is true now rather than only from what a model remembers: the news this morning, the price this minute, your own mail.
- **Code** is the second, and is not finished. It builds things on a machine of its own — writes the files, runs them, hosts the result — and what it makes outlives the conversation.

Your own agents are the same shape: a name, an instruction, and the tools they may reach. Each has an address, so `agent+yours@` reaches it from anywhere that can send mail, the same way `agent@` reaches Micro.

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

The binary is a client, and by default it calls **https://micro.mu** — the hosted Mu instance where Micro runs publicly. Running your own? Point it there:

```bash
mu login https://your.host   # saves the address and a token
mu config get                # says which instance is in use, and why
```

Without that, `mu news list` on the machine you just installed calls the hosted instance rather than the one you are running. `MU_URL` and `--url` override per shell and per command.

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

Every service is a `mu` subcommand

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

Every tool is a command: the service, then the method. The underscore form works too, so `mu news list` and `mu news_list` are the same call.

To authenticate

```bash
mu login                  # opens /token in your browser, paste the PAT back
mu config set token xxx   # or set it directly
export MU_TOKEN=xxx       # or use the environment
```

Run `mu --help` for the list — it reads the same catalogue the agent does.

To talk to **Micro** instead, use `mu ask` — it runs on the instance, so it needs your token and no model key of your own:

```bash
mu ask "what is in my inbox?"
mu ask --agent research "anything new this week?"
```

`mu agent` is the other direction and easy to reach for by mistake: it runs the agent *here*, on your machine, with your own model key, renting tools from an instance over x402 and paying per call. Same word in English, opposite ways round.

## API

Every service has a HTTP endpoint, at `/api/v1/<service>/<method>`.

```bash
curl https://micro.mu/api/v1/                      # the catalogue
curl "https://micro.mu/api/v1/news/list?limit=5"   # arguments in the query
curl -X POST https://micro.mu/api/v1/news/list \
  -H 'Content-Type: application/json' -d '{"limit":5}'
```

Authenticate with a token from `/token` as `Authorization: Bearer`, or with an OAuth client. A priced endpoint answers 402 without one, which an x402 client pays per call with no account at all.

For Tools via MCP use `/mcp`. See [/tools](https://micro.mu/tools) for more info.

## Web

- `/` - talk to Micro
- `/home` — an overview of everything going on.
- `/inbox` — the place to see chats, mail, tasks, etc.
- `/agents` — your agents, and where you make a new one.
- `/services` — `/news`, `/weather`, `/markets`, etc.

## Configuration

Some files are embedded in the binary, so editing means rebuilding:

- `home/cards.json` — home screen cards
- `service/news/feeds.json` — RSS news feeds
- `service/chat/prompts.json` — chat topics
- `service/video/channels.json` — YouTube channels
- `service/places/locations.json` — saved locations

See [Install](docs/INSTALL.md) for every setting the code reads.

The rest lives in /admin/config on the server.

## License

AGPL 3.0
