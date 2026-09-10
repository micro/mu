# Mu

A runtime for agents and services

## Overview

Mu is a runtime for agents and services. It's a full stack solution to the question, how do I run everything myself. More and more 
we're becoming reliant on the ecosystem of hosted things. The question is, how much of the system can you run yourself. The services, 
the tools, the APIs, maybe not the models but everything else. From the personal assistant answering the front door to the smtp server 
handling the inbound mail on the backend. Mu attempts to do it all in a single binary on one machine in one place in one system.

## Features 

It includes:

- **Micro** - your personal assistant and the default agent.
- **Home** - a launch pad for your assistant, apps and daily work.
- **Inbox** - A place to keep track of everything.
- **Clients** - Use Micro via Web, SMS, email, etc.
- **Services** - building blocks for agents.
- **Protocols** - a way to self host SMTP, XMPP, SFTP, SSH.

## How it works

**Mu** is a single binary: the runtime, services, archive, inbox and agent system all in one host. Services operate as building blocks for agents — mail, chat, news, video, search, markets, weather and more. Data gets archived locally so it stays searchable and becomes contextual memory. Services and the archive become tools for Micro and any other agents you create.

**Micro** is the first agent and the one you use for everything. It answers by default and can be reached from the web, email, SMS, WhatsApp or the CLI.

Mu comes with a unified inbox for mail, chat, SMS, WhatsApp, notes, tasks and agent activity, bringing communication and agent work into one place.

## Agents

Mu is the runtime. Micro is the first agent and the one people meet first.

- **Micro** is your personal assistant and the default agent. General purpose, with the services above as its tools, so it can answer from what is true now rather than only from what a model remembers: the news this morning, the price this minute, your own mail.

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
- `/home` — a launch pad for your assistant, apps and daily work.
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
