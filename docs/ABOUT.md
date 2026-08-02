# About Mu

**Tools for agents.**

## What Mu is

Mu gives an agent the everyday internet as tools: news, web search, mail,
markets, weather, video, places, images, storage. 59 of them, behind one
[MCP](https://modelcontextprotocol.io) endpoint.

Point an MCP client at `/mcp` and everything is available. Browse what's there
at [/tools](https://micro.mu/tools).

## Real tools, not wrappers

Most things offering an agent tools are a thin layer over somebody else's API —
reselling access, no margin, nothing to defend. Mu runs the things it exposes.

`mail_inbox` reads a real inbox, served by an SMTP server with DKIM this
instance runs. `web_search` queries an index it maintains. `db_create` writes to
real storage. `apps_run` executes in a sandbox it owns.

That is the whole idea. An agent is only as useful as what it can actually
reach, and most of what it can reach today is either a chat box with no hands or
a proxy to an API somebody else controls.

## Two doors, one set of tools

Every capability is a service, and a service is reachable through either door.
Nothing is built twice.

**An agent** calls the MCP endpoint at `/mcp`. It gets a tool list and uses what
is there. That is the only way in — there is no second protocol to choose
between, and nothing to integrate per tool.

**A person** signs in and gets the home screen: cards for each service at a
glance — headlines, prices, weather, unread mail — with the agent inline to act
on what they are looking at. Apps, saved items, wallet, settings, all of it.

Same services underneath. When a new one registers it appears in both places at
once, with no extra wiring — the agent gets a new tool, the app gets a new card.
That is why the tools are worth trusting: they are used by a person every day
before any agent calls them.

## Why an agent, not a developer

The previous generation of this idea sold APIs to developers. A developer has to
find you, choose you, integrate you, and keep choosing you — high friction, high
churn, competing on price with whoever is cheapest.

An agent does not shop. It is handed a tool list and uses what is there. That
inverts the funnel: you are not persuading anyone to integrate, you are present
at the moment of need.

## Connecting

1. **Add the endpoint** to your MCP client: `https://micro.mu/mcp`. Nothing else
   goes in the config.
2. **Sign in when it asks.** The first call returns `401` pointing at this
   instance's authorization server; a client that speaks
   [MCP authorization](https://modelcontextprotocol.io/specification/basic/authorization)
   walks you through sign-in and keeps the token itself. One that doesn't takes a
   Personal Access Token from `/token`.
3. **Call anything.** Reading this instance's own content is included. Calls that
   cost money to run — a model call, a paid third party — draw credits.

It is the same account either way: sign into the app and your agent's calls draw
on the same balance.

## What we don't do

- **No ads** — we don't sell your attention
- **No tracking** — we don't profile you
- **No algorithmic ranking** — chronological, transparent
- **No infinite scroll** — there's always an end
- **No push notifications** — you come when you want

## Technology

One Go binary, built on [Go Micro](https://go-micro.dev). Every capability is a
go-micro service; the assistant is a go-micro agent that calls them; `/mcp` is
served by go-micro's MCP gateway. No external infrastructure.

Supports Anthropic Claude, Atlas Cloud (DeepSeek, Qwen), or local models via any
OpenAI-compatible API (Ollama, vLLM, llama.cpp).

Run an instance and you are the operator: the tools are yours, and anyone paying
to call them pays you. AGPL-3.0.

## Where to go next

- **[Tools](https://micro.mu/tools)** — the catalogue, with prices
- **[MCP](MCP.md)** — protocol detail and playground
- **[CLI](CLI.md)** — every tool as a `mu` subcommand
- **[Installation](INSTALLATION.md)** — run your own
- **[Architecture](ARCHITECTURE.md)** — how the services fit together

---

**Try it** at [micro.mu](https://micro.mu) — or self-host from
[github.com/micro/mu](https://github.com/micro/mu)
