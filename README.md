# Mu

A runtime for agents and services

## Overview

Mu is a runtime for agents and services. It's a full stack solution to the question, how do I run everything myself. More and more 
we're becoming reliant on the ecosystem of hosted things. The question is, how much of the system can you run yourself. The services, 
the tools, the agents, maybe not the models but everything else. From the personal assistant answering the front door to the smtp server 
handling the inbound mail on the backend. Mu attempts to do it all in a single binary on one machine in one place in one system.

## Features 

It includes:

- **Micro** - your personal assistant and the default agent.
- **Home** - your personal dashboard and Feed, with a prompt for quick questions.
- **Inbox** - A place to keep track of everything.
- **Clients** - Use Micro via Web, SMS, email, etc.
- **Services** - building blocks for agents.
- **Protocols** - a way to self host SMTP, XMPP, SFTP, SSH.

## How it works

**Mu** is a single binary: the runtime, services, archive, inbox and agent system all in one host. Services operate as building blocks for agents — mail, chat, news, video, search, markets, weather and more. Data gets archived locally so it stays searchable and becomes contextual memory. Services and the archive become tools for Micro and any other agents you create.

**Micro** is the first agent and the one you use for everything. It answers by default and can be reached from the web, email, SMS, WhatsApp or the CLI.

Mu comes with a unified inbox for mail, chat, SMS, WhatsApp, notes, tasks and agent activity, bringing communication and agent work into one place.

Home keeps the prompt, brief, service shortcuts, Inbox, Todo and Upcoming
in a personal dashboard, with Feed as a separate tab. Assistant opens the
dedicated conversation. Navigation offers Assistant (Ask on mobile), Home,
Inbox, Agents and Services together. Logged-out Home visits return to landing.

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

The CLI calls the same public capabilities as HTTP and MCP:

```bash
mu agent_list
mu work submit --prompt "Research the options and recommend one"
mu work list
mu work get --id WORK_ID
mu inbox list
mu help
```

Use `mu ask` for an interactive conversation. Operation names can be written as
two words or with an underscore, such as `mu work list` or `mu work_list`.
`mu agent` remains the local-agent command; use `mu agent_list` to list remote
agents. Service-tool commands remain available against a separately configured tools
host; they are not the primary host's public API.

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

One public surface for **Agent, Work and Inbox**, available through JSON HTTP
at `/api/v1` and MCP at `/mcp`. Mu runs the agent and manages its tool calls;
your client supplies the goal and reads the outcome.

```bash
curl https://micro.mu/api/v1
curl https://micro.mu/api/v1/agent/ask \
  -H "Authorization: Bearer $MU_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"What needs my attention?"}'
```

The response contains `data.text` and `data.thread`. Pass `thread` back to
continue. For background execution, call `work/submit` and poll `work/get` with
the returned `id`. Inbox lists and reads the same saved conversations as the UI.
All operations use POST bodies; only the catalogue uses GET.

Create a token at `/token`. API scopes are `api:agent`, `api:work`, and
`api:inbox`, with read/write permissions. These are account-wide capabilities,
not stateless application sandboxes. Existing service-scoped tokens cannot
acquire broader agent access. Agent calls use the existing credit balance.

The [live API reference](https://micro.mu/api) describes each operation and its
arguments. MCP exposes the same operations as `agent_ask`, `work_submit`, etc.
Service tools remain internal to agent execution and the sandboxed app bridge.
A separate host configured for x402 retains its existing service contract.

## Web

- `/` - talk to Micro
- `/home` — a launch pad for your assistant, apps and daily work.
- `/inbox` — messages, updates and conversations.
- `/work` — delegated goals, progress and outcomes.
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
