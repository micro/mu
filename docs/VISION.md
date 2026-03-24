# Vision

**Apps without ads, algorithms, or tracking.**

## The Problem

The internet was supposed to connect us and make life better. Instead, every platform monetises your attention. Infinite scroll keeps you hooked. Algorithms decide what you see. Ads follow you everywhere. Your data gets mined and sold.

Want to browse the news? You get profiled. Want to search? You get tracked. Want to watch a video? You sit through ads. Want to build a simple tool? You need an app store, a developer account, and a framework.

The tools people use every day — news, search, email, chat, markets — are scattered across dozens of platforms, each competing for your time and data. There's no single place that brings them together without the noise.

## What Mu Does

Mu brings together the daily tools people actually use in one place, with zero ads and zero tracking. Browsing is free. AI features use credits at 1p each. You get 20 free per day.

It runs as a single Go binary. Self-host it on your own server or use the hosted version.

| Service | What it does |
|---------|-------------|
| **News** | Headlines from RSS feeds, chronological, with AI summaries |
| **Markets** | Live crypto, futures, commodity, and currency prices |
| **Video** | YouTube without ads, algorithms, or shorts |
| **Web** | Search the web without tracking (Brave) |
| **Blog** | Microblogging with daily AI-generated digests |
| **Chat** | AI-powered conversation |
| **Mail** | Private messaging and email |
| **Apps** | Build and use small web tools — or ask the agent to build one |
| **Agent** | AI assistant that searches, answers, and builds across every service |
| **Wallet** | Pay as you go — 1 credit = 1p |

## Design Choices

Every design choice follows from a simple question: does this serve the user or the platform?

**Chronological feeds.** No algorithm decides what you see. News is sorted by time. Blog posts are sorted by time. You choose what to read.

**Finite content.** There is no infinite scroll. You see what's there and move on. The goal is to inform, not to keep you scrolling.

**No ads, no tracking.** Revenue comes from usage credits, not attention. There's no incentive to maximise screen time.

**Pay for what you use.** Browsing is free. Actions that cost infrastructure — search, AI, posting — cost credits. 1 credit = 1p. You get 20 free per day, and credits never expire.

**Single binary.** Mu runs as one Go binary with no external dependencies. Self-host it on your own server or use the hosted version.

**Open source.** AGPL-3.0. Your server, your data, no limits.

## What Makes It Different

| The Internet Today | Mu |
|---|---|
| Infinite scroll | Finite, curated content |
| Algorithmic feeds | Chronological, you choose |
| Ad-supported | No ads |
| Engagement metrics | Intentional use |
| Screen time maximisation | Get in, get out |
| Data mining | No tracking |
| Walled gardens | Self-hostable, open source |

## The Agent

The agent has access to every service on the platform via the Model Context Protocol (MCP). It can search news and the web, check market prices and weather, read and write blog posts, build working apps from a description, and execute code in a sandbox.

This isn't a chatbot bolted onto a website. Every feature is a tool the agent can use, and every tool is available to external AI agents too.

## Apps

Need a tool that doesn't exist? Describe it in plain English and the AI builds it — a working app in seconds. Apps are just HTML. No frameworks. No app store. No tracking. Build them in the split-pane editor with live preview, start from a template, or let the agent create one for you.

The agent can also run JavaScript in a sandbox for calculations and data processing, returning structured results.

## For Developers and AI Agents

Every feature is available via REST API and MCP. Connect Claude Desktop, Cursor, or any MCP-compatible client:

```json
{
  "mcpServers": {
    "mu": {
      "url": "https://mu.xyz/mcp"
    }
  }
}
```

## The Agent Economy

AI agents need access to real-world services — search, weather, places, email, markets. Today, each service requires a separate API key, a separate account, a separate billing relationship. An agent that wants to search the web, check the weather, and send an email needs three different providers, three signups, three payment methods.

Mu collapses this into one endpoint. Every service is available via MCP at `/mcp`. And with the [x402 protocol](https://x402.org), agents pay per-request with USDC stablecoins — no account, no API key, no signup. The agent's wallet is its identity.

This is what API access looks like when you build for machines, not just humans:

| Traditional APIs | Mu + x402 |
|---|---|
| Sign up for each service | No account needed |
| Manage API keys | No API keys |
| Pre-pay or subscribe | Pay per request |
| Different billing per provider | One wallet, one protocol |
| Rate limits tied to API tiers | Pay for what you use |

An autonomous agent can search the web ($0.05), check the weather ($0.01), look up nearby restaurants ($0.05), and send an email ($0.04) — all from a single MCP endpoint, paying on-chain for each call. Zero onboarding. Zero friction.

The [services marketplace](MARKETPLACE.md) extends this further. Third-party developers can register their own MCP services — recipe extraction, flight tracking, translation — and agents discover and pay for them through the same protocol. x402 means providers can also receive payments directly on-chain, no intermediary required.

## Technology

- **Language:** Go — single binary, no dependencies
- **Storage:** JSON files and SQLite with FTS5 full-text search
- **Federation:** ActivityPub — connect with Mastodon, Threads, etc.
- **PWA:** Installable as a progressive web app
- **License:** AGPL-3.0

## Pricing

- **Free to browse** — news, blogs, videos, markets, all of it
- **20 free credits per day** — covers search, chat, and AI features
- **Pay as you go** — 1 credit = 1p, top up via card
- **Pay with crypto** — AI agents pay per-request with USDC via [x402](https://x402.org)
- **Self-host for free** — run your own instance, unlimited

## Get Started

Visit [mu.xyz](https://mu.xyz) or self-host:

```
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```
