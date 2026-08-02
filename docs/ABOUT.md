# About Mu

**Tools for agents.**

## What Mu is

Mu gives an agent the everyday internet as tools: news, web search, mail,
markets, weather, video, places, images, storage. 59 of them, over
[MCP](https://modelcontextprotocol.io) and REST, paid per request in USDC via
[x402](https://x402.org) — no API key, no account, no signup.

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

Every capability is a service, and a service is reachable however the caller
prefers. Nothing is built twice.

**An agent** calls the MCP endpoint at `/mcp`, or REST, or pays per request over
x402. It gets a tool list and uses what is there.

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
at the moment of need. And with x402 there is no signup between wanting and
paying — the agent settles in stablecoin and retries, sub-second.

## How paying works

1. **No login to call.** Point an agent at an endpoint. Your first 10 calls per
   wallet are free.
2. **When payment is due,** the endpoint answers `HTTP 402` with a price. The
   agent's x402 wallet pays in USDC and retries.
3. **You pay the operator** running the instance, directly, wallet to wallet. No
   middleman.

Prefer prepaid credits and a dashboard? Create an account and use a Personal
Access Token. Both reach the same tools.

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
