# Mu — The Micro Network

**Apps without ads, algorithms, or tracking.**

## The Problem

The internet was supposed to connect us and make life better. Instead, every platform monetises your attention. Infinite scroll keeps you hooked. Algorithms decide what you see. Ads follow you everywhere. Your data gets mined and sold.

Want to browse the news? You get profiled. Want to search? You get tracked. Want to watch a video? You sit through ads. Want to build a simple tool? You need an app store, a developer account, and a framework.

## What Mu Does

Mu brings together the daily tools people actually use — news, markets, video, search, chat, email, blogging — in one place, with zero ads and zero tracking. Browsing is free. AI features use credits at 1p each. You get 20 free per day.

It runs as a single binary. Self-host it on your own server or use the hosted version at mu.xyz.

## Services

| Service | What it does |
|---------|-------------|
| **News** | Headlines and articles from RSS feeds, chronological — with AI summaries |
| **Markets** | Live crypto, futures, and commodity prices |
| **Video** | YouTube without ads, algorithms, or shorts |
| **Web** | Search the web without tracking (powered by Brave) |
| **Blog** | Microblogging with daily AI-generated digests |
| **Social** | Topic-based discussions with community notes |
| **Chat** | AI-powered conversation on any topic |
| **Mail** | Private messaging and email |
| **Apps** | Build and use small, useful tools — or ask the agent to build one |
| **Agent** | AI assistant that can search, answer, and build across every service |
| **Wallet** | Pay as you go with credits — 1 credit = 1p |

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

## Apps

Need a tool that doesn't exist? Describe it in plain English and the AI builds it — a working app in seconds. A timer. A calculator. A habit tracker. A unit converter. Whatever you need.

Apps are just HTML. No frameworks. No app store. No tracking. Build them yourself in the split-pane editor with live preview, or let the agent create one for you. Copy them, fork them, self-host them.

The platform ships with 6 built-in apps: Timer, Calculator, Unit Converter, Flashcards, Notes, and Habit Tracker. The app builder includes 8 templates and AI generation from natural language.

Apps can access platform features through a lightweight SDK:
- `mu.ai()` — Ask AI a question
- `mu.store` — Persistent key-value storage
- `mu.fetch()` — Fetch URLs through the proxy
- `mu.run()` — Execute code and return structured results

## The Agent

The agent isn't just a chatbot. It has access to every service on the platform via the Model Context Protocol (MCP). It can:

- Search the news, web, and video
- Check market prices and weather
- Read and write blog posts
- Build a working app from a description and save it — in one step
- Execute code in a sandboxed environment

Ask it anything. "What's in the news about AI?" — it searches. "Build me a tip calculator" — it builds one. "What's the weather in London?" — it checks.

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

Agents can authenticate, manage their own wallet credits, search content, create apps, and execute tools — all programmatically.

## Technology

- **Language:** Go — single binary, no dependencies
- **Storage:** JSON files or SQLite with FTS5 full-text search
- **Federation:** ActivityPub — connect with Mastodon, Threads, etc.
- **PWA:** Installable as a progressive web app
- **License:** AGPL-3.0 — open source

## Pricing

- **Free to browse** — news, blogs, videos, markets, all of it
- **20 free credits per day** — covers search, chat, and AI features
- **Pay as you go** — 1 credit = 1p, top up via card or crypto
- **Self-host for free** — run your own instance

## Get Started

Visit [mu.xyz](https://mu.xyz) or self-host:

```
git clone https://github.com/micro/mu
cd mu && go install
mu --serve
```
